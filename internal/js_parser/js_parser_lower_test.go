package js_parser

import (
	"fmt"
	"testing"
)

func TestLowerFunctionArgumentScope(t *testing.T) {
	templates := []string{
		"(x = %s) => {\n};\n",
		"(function(x = %s) {\n});\n",
		"function foo(x = %s) {\n}\n",

		"({[%s]: x}) => {\n};\n",
		"(function({[%s]: x}) {\n});\n",
		"function foo({[%s]: x}) {\n}\n",

		"({x = %s}) => {\n};\n",
		"(function({x = %s}) {\n});\n",
		"function foo({x = %s}) {\n}\n",
	}

	for _, template := range templates {
		test := func(before string, after string) {
			expectPrintedTarget(t, 2015, fmt.Sprintf(template, before), fmt.Sprintf(template, after))
		}

		test("a() ?? b", "((_a) => (_a = a()) != null ? _a : b)()")
		test("a()?.b", "((_a) => (_a = a()) == null ? void 0 : _a.b)()")
		test("a?.b?.()", "((_a) => (_a = a == null ? void 0 : a.b) == null ? void 0 : _a.call(a))()")
		test("a.b.c?.()", "((_a) => ((_b) => (_b = (_a = a.b).c) == null ? void 0 : _b.call(_a))())()")
		test("class { static a }", "((_a) => (_a = class {\n}, __publicField(_a, \"a\"), _a))()")
	}
}

func TestLowerNullishCoalescing(t *testing.T) {
	expectParseError(t, "a ?? b && c", "<stdin>: error: Unexpected \"&&\"\n")
	expectParseError(t, "a ?? b || c", "<stdin>: error: Unexpected \"||\"\n")
	expectParseError(t, "a ?? b && c || d", "<stdin>: error: Unexpected \"&&\"\n")
	expectParseError(t, "a ?? b || c && d", "<stdin>: error: Unexpected \"||\"\n")
	expectParseError(t, "a && b ?? c", "<stdin>: error: Unexpected \"??\"\n")
	expectParseError(t, "a || b ?? c", "<stdin>: error: Unexpected \"??\"\n")
	expectParseError(t, "a && b || c ?? c", "<stdin>: error: Unexpected \"??\"\n")
	expectParseError(t, "a || b && c ?? d", "<stdin>: error: Unexpected \"??\"\n")
	expectPrinted(t, "a ?? b, b && c", "a ?? b, b && c;\n")
	expectPrinted(t, "a ?? b, b || c", "a ?? b, b || c;\n")
	expectPrinted(t, "a && b, b ?? c", "a && b, b ?? c;\n")
	expectPrinted(t, "a || b, b ?? c", "a || b, b ?? c;\n")

	expectPrintedTarget(t, 2020, "a ?? b", "a ?? b;\n")
	expectPrintedTarget(t, 2019, "a ?? b", "a != null ? a : b;\n")
	expectPrintedTarget(t, 2019, "a() ?? b()", "var _a;\n(_a = a()) != null ? _a : b();\n")
	expectPrintedTarget(t, 2019, "function foo() { if (x) { a() ?? b() ?? c() } }",
		"function foo() {\n  var _a, _b;\n  if (x) {\n    (_b = (_a = a()) != null ? _a : b()) != null ? _b : c();\n  }\n}\n")
	expectPrintedTarget(t, 2019, "() => a ?? b", "() => a != null ? a : b;\n")
	expectPrintedTarget(t, 2019, "() => a() ?? b()", "() => {\n  var _a;\n  return (_a = a()) != null ? _a : b();\n};\n")
}

func TestLowerNullishCoalescingAssign(t *testing.T) {
	expectPrinted(t, "a ??= b", "a ??= b;\n")

	expectPrintedTarget(t, 2019, "a ??= b", "a != null ? a : a = b;\n")
	expectPrintedTarget(t, 2019, "a.b ??= c", "var _a;\n(_a = a.b) != null ? _a : a.b = c;\n")
	expectPrintedTarget(t, 2019, "a().b ??= c", "var _a, _b;\n(_b = (_a = a()).b) != null ? _b : _a.b = c;\n")
	expectPrintedTarget(t, 2019, "a[b] ??= c", "var _a;\n(_a = a[b]) != null ? _a : a[b] = c;\n")
	expectPrintedTarget(t, 2019, "a()[b()] ??= c", "var _a, _b, _c;\n(_c = (_a = a())[_b = b()]) != null ? _c : _a[_b] = c;\n")

	expectPrintedTarget(t, 2020, "a ??= b", "a ?? (a = b);\n")
	expectPrintedTarget(t, 2020, "a.b ??= c", "a.b ?? (a.b = c);\n")
	expectPrintedTarget(t, 2020, "a().b ??= c", "var _a;\n(_a = a()).b ?? (_a.b = c);\n")
	expectPrintedTarget(t, 2020, "a[b] ??= c", "a[b] ?? (a[b] = c);\n")
	expectPrintedTarget(t, 2020, "a()[b()] ??= c", "var _a, _b;\n(_a = a())[_b = b()] ?? (_a[_b] = c);\n")
}

func TestLowerLogicalAssign(t *testing.T) {
	expectPrinted(t, "a &&= b", "a &&= b;\n")
	expectPrinted(t, "a ||= b", "a ||= b;\n")

	expectPrintedTarget(t, 2020, "a &&= b", "a && (a = b);\n")
	expectPrintedTarget(t, 2020, "a.b &&= c", "a.b && (a.b = c);\n")
	expectPrintedTarget(t, 2020, "a().b &&= c", "var _a;\n(_a = a()).b && (_a.b = c);\n")
	expectPrintedTarget(t, 2020, "a[b] &&= c", "a[b] && (a[b] = c);\n")
	expectPrintedTarget(t, 2020, "a()[b()] &&= c", "var _a, _b;\n(_a = a())[_b = b()] && (_a[_b] = c);\n")

	expectPrintedTarget(t, 2020, "a ||= b", "a || (a = b);\n")
	expectPrintedTarget(t, 2020, "a.b ||= c", "a.b || (a.b = c);\n")
	expectPrintedTarget(t, 2020, "a().b ||= c", "var _a;\n(_a = a()).b || (_a.b = c);\n")
	expectPrintedTarget(t, 2020, "a[b] ||= c", "a[b] || (a[b] = c);\n")
	expectPrintedTarget(t, 2020, "a()[b()] ||= c", "var _a, _b;\n(_a = a())[_b = b()] || (_a[_b] = c);\n")
}

func TestLowerAsyncFunctions(t *testing.T) {
	// Lowered non-arrow functions with argument evaluations should merely use
	// "arguments" rather than allocating a new array when forwarding arguments
	expectPrintedTarget(t, 2015, "async function foo(a, b = couldThrowErrors()) {console.log(a, b);}", `function foo(_0) {
  return __async(this, arguments, function* (a, b = couldThrowErrors()) {
    console.log(a, b);
  });
}
`)
	// Skip forwarding altogether when parameter evaluation obviously cannot throw
	expectPrintedTarget(t, 2015, "async (a, b = 123) => {console.log(a, b);}", `(a, b = 123) => __async(this, null, function* () {
  console.log(a, b);
});
`)
}

func TestLowerClassSideEffectOrder(t *testing.T) {
	// The order of computed property side effects must not change
	expectPrintedTarget(t, 2015, `class Foo {
	[a()]() {}
	[b()];
	[c()] = 1;
	[d()]() {}
	static [e()];
	static [f()] = 1;
	static [g()]() {}
	[h()];
}
`, `var _a, _b, _c, _d, _e;
class Foo {
  constructor() {
    __publicField(this, _a);
    __publicField(this, _b, 1);
    __publicField(this, _e);
  }
  [a()]() {
  }
  [(_a = b(), _b = c(), d())]() {
  }
  static [(_c = e(), _d = f(), g())]() {
  }
}
_e = h();
__publicField(Foo, _c);
__publicField(Foo, _d, 1);
`)
}

func TestLowerClassInstance(t *testing.T) {
	expectPrintedTarget(t, 2015, "class Foo {}", "class Foo {\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { foo }", "class Foo {\n  constructor() {\n    __publicField(this, \"foo\");\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { foo = null }", "class Foo {\n  constructor() {\n    __publicField(this, \"foo\", null);\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { 123 }", "class Foo {\n  constructor() {\n    __publicField(this, 123);\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { 123 = null }", "class Foo {\n  constructor() {\n    __publicField(this, 123, null);\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { [foo] }", "var _a;\nclass Foo {\n  constructor() {\n    __publicField(this, _a);\n  }\n}\n_a = foo;\n")
	expectPrintedTarget(t, 2015, "class Foo { [foo] = null }", "var _a;\nclass Foo {\n  constructor() {\n    __publicField(this, _a, null);\n  }\n}\n_a = foo;\n")

	expectPrintedTarget(t, 2015, "(class {})", "(class {\n});\n")
	expectPrintedTarget(t, 2015, "(class { foo })", "(class {\n  constructor() {\n    __publicField(this, \"foo\");\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { foo = null })", "(class {\n  constructor() {\n    __publicField(this, \"foo\", null);\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { 123 })", "(class {\n  constructor() {\n    __publicField(this, 123);\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { 123 = null })", "(class {\n  constructor() {\n    __publicField(this, 123, null);\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { [foo] })", "var _a, _b;\n_b = class {\n  constructor() {\n    __publicField(this, _a);\n  }\n}, _a = foo, _b;\n")
	expectPrintedTarget(t, 2015, "(class { [foo] = null })", "var _a, _b;\n_b = class {\n  constructor() {\n    __publicField(this, _a, null);\n  }\n}, _a = foo, _b;\n")

	expectPrintedTarget(t, 2015, "class Foo extends Bar {}", `class Foo extends Bar {
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} constructor() { super() } }", `class Foo extends Bar {
  bar() {
  }
  constructor() {
    super();
  }
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} foo }", `class Foo extends Bar {
  constructor() {
    super(...arguments);
    __publicField(this, "foo");
  }
  bar() {
  }
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} foo; constructor() { super() } }", `class Foo extends Bar {
  constructor() {
    super();
    __publicField(this, "foo");
  }
  bar() {
  }
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} foo; constructor({ ...args }) { super() } }", `class Foo extends Bar {
  constructor(_a) {
    var args = __rest(_a, []);
    super();
    __publicField(this, "foo");
  }
  bar() {
  }
}
`)
}

func TestLowerClassStatic(t *testing.T) {
	expectPrintedTarget(t, 2015, "class Foo { static foo }", "class Foo {\n}\n__publicField(Foo, \"foo\");\n")
	expectPrintedTarget(t, 2015, "class Foo { static foo = null }", "class Foo {\n}\n__publicField(Foo, \"foo\", null);\n")
	expectPrintedTarget(t, 2015, "class Foo { static foo(a, b) {} }", "class Foo {\n  static foo(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static set foo(a) {} }", "class Foo {\n  static set foo(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static 123 }", "class Foo {\n}\n__publicField(Foo, 123);\n")
	expectPrintedTarget(t, 2015, "class Foo { static 123 = null }", "class Foo {\n}\n__publicField(Foo, 123, null);\n")
	expectPrintedTarget(t, 2015, "class Foo { static 123(a, b) {} }", "class Foo {\n  static 123(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static get 123() {} }", "class Foo {\n  static get 123() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static set 123(a) {} }", "class Foo {\n  static set 123(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static [foo] }", "var _a;\nclass Foo {\n}\n_a = foo;\n__publicField(Foo, _a);\n")
	expectPrintedTarget(t, 2015, "class Foo { static [foo] = null }", "var _a;\nclass Foo {\n}\n_a = foo;\n__publicField(Foo, _a, null);\n")
	expectPrintedTarget(t, 2015, "class Foo { static [foo](a, b) {} }", "class Foo {\n  static [foo](a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static get [foo]() {} }", "class Foo {\n  static get [foo]() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static set [foo](a) {} }", "class Foo {\n  static set [foo](a) {\n  }\n}\n")

	expectPrintedTarget(t, 2015, "export default class Foo { static foo }", "export default class Foo {\n}\n__publicField(Foo, \"foo\");\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static foo = null }", "export default class Foo {\n}\n__publicField(Foo, \"foo\", null);\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static foo(a, b) {} }", "export default class Foo {\n  static foo(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static get foo() {} }", "export default class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static set foo(a) {} }", "export default class Foo {\n  static set foo(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static 123 }", "export default class Foo {\n}\n__publicField(Foo, 123);\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static 123 = null }", "export default class Foo {\n}\n__publicField(Foo, 123, null);\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static 123(a, b) {} }", "export default class Foo {\n  static 123(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static get 123() {} }", "export default class Foo {\n  static get 123() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static set 123(a) {} }", "export default class Foo {\n  static set 123(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static [foo] }", "var _a;\nexport default class Foo {\n}\n_a = foo;\n__publicField(Foo, _a);\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static [foo] = null }", "var _a;\nexport default class Foo {\n}\n_a = foo;\n__publicField(Foo, _a, null);\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static [foo](a, b) {} }", "export default class Foo {\n  static [foo](a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static get [foo]() {} }", "export default class Foo {\n  static get [foo]() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static set [foo](a) {} }", "export default class Foo {\n  static set [foo](a) {\n  }\n}\n")

	expectPrintedTarget(t, 2015, "export default class { static foo }",
		"export default class stdin_default {\n}\n__publicField(stdin_default, \"foo\");\n")
	expectPrintedTarget(t, 2015, "export default class { static foo = null }",
		"export default class stdin_default {\n}\n__publicField(stdin_default, \"foo\", null);\n")
	expectPrintedTarget(t, 2015, "export default class { static foo(a, b) {} }", "export default class {\n  static foo(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static get foo() {} }", "export default class {\n  static get foo() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static set foo(a) {} }", "export default class {\n  static set foo(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static 123 }",
		"export default class stdin_default {\n}\n__publicField(stdin_default, 123);\n")
	expectPrintedTarget(t, 2015, "export default class { static 123 = null }",
		"export default class stdin_default {\n}\n__publicField(stdin_default, 123, null);\n")
	expectPrintedTarget(t, 2015, "export default class { static 123(a, b) {} }", "export default class {\n  static 123(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static get 123() {} }", "export default class {\n  static get 123() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static set 123(a) {} }", "export default class {\n  static set 123(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static [foo] }",
		"var _a;\nexport default class stdin_default {\n}\n_a = foo;\n__publicField(stdin_default, _a);\n")
	expectPrintedTarget(t, 2015, "export default class { static [foo] = null }",
		"var _a;\nexport default class stdin_default {\n}\n_a = foo;\n__publicField(stdin_default, _a, null);\n")
	expectPrintedTarget(t, 2015, "export default class { static [foo](a, b) {} }", "export default class {\n  static [foo](a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static get [foo]() {} }", "export default class {\n  static get [foo]() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static set [foo](a) {} }", "export default class {\n  static set [foo](a) {\n  }\n}\n")

	expectPrintedTarget(t, 2015, "(class Foo { static foo })", "var _a;\n_a = class {\n}, __publicField(_a, \"foo\"), _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static foo = null })", "var _a;\n_a = class {\n}, __publicField(_a, \"foo\", null), _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static foo(a, b) {} })", "(class Foo {\n  static foo(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static get foo() {} })", "(class Foo {\n  static get foo() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static set foo(a) {} })", "(class Foo {\n  static set foo(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static 123 })", "var _a;\n_a = class {\n}, __publicField(_a, 123), _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static 123 = null })", "var _a;\n_a = class {\n}, __publicField(_a, 123, null), _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static 123(a, b) {} })", "(class Foo {\n  static 123(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static get 123() {} })", "(class Foo {\n  static get 123() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static set 123(a) {} })", "(class Foo {\n  static set 123(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static [foo] })", "var _a, _b;\n_b = class {\n}, _a = foo, __publicField(_b, _a), _b;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static [foo] = null })", "var _a, _b;\n_b = class {\n}, _a = foo, __publicField(_b, _a, null), _b;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static [foo](a, b) {} })", "(class Foo {\n  static [foo](a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static get [foo]() {} })", "(class Foo {\n  static get [foo]() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static set [foo](a) {} })", "(class Foo {\n  static set [foo](a) {\n  }\n});\n")

	expectPrintedTarget(t, 2015, "(class { static foo })", "var _a;\n_a = class {\n}, __publicField(_a, \"foo\"), _a;\n")
	expectPrintedTarget(t, 2015, "(class { static foo = null })", "var _a;\n_a = class {\n}, __publicField(_a, \"foo\", null), _a;\n")
	expectPrintedTarget(t, 2015, "(class { static foo(a, b) {} })", "(class {\n  static foo(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static get foo() {} })", "(class {\n  static get foo() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static set foo(a) {} })", "(class {\n  static set foo(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static 123 })", "var _a;\n_a = class {\n}, __publicField(_a, 123), _a;\n")
	expectPrintedTarget(t, 2015, "(class { static 123 = null })", "var _a;\n_a = class {\n}, __publicField(_a, 123, null), _a;\n")
	expectPrintedTarget(t, 2015, "(class { static 123(a, b) {} })", "(class {\n  static 123(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static get 123() {} })", "(class {\n  static get 123() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static set 123(a) {} })", "(class {\n  static set 123(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static [foo] })", "var _a, _b;\n_b = class {\n}, _a = foo, __publicField(_b, _a), _b;\n")
	expectPrintedTarget(t, 2015, "(class { static [foo] = null })", "var _a, _b;\n_b = class {\n}, _a = foo, __publicField(_b, _a, null), _b;\n")
	expectPrintedTarget(t, 2015, "(class { static [foo](a, b) {} })", "(class {\n  static [foo](a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static get [foo]() {} })", "(class {\n  static get [foo]() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static set [foo](a) {} })", "(class {\n  static set [foo](a) {\n  }\n});\n")

	expectPrintedTarget(t, 2015, "(class {})", "(class {\n});\n")
	expectPrintedTarget(t, 2015, "class Foo {}", "class Foo {\n}\n")
	expectPrintedTarget(t, 2015, "(class Foo {})", "(class Foo {\n});\n")

	// Static field with initializers that access the class expression name must
	// still work when they are pulled outside of the class body
	expectPrintedTarget(t, 2015, `
		let Bar = class Foo {
			static foo = 123
			static bar = Foo.foo
		}
	`, `var _a;
let Bar = (_a = class {
}, __publicField(_a, "foo", 123), __publicField(_a, "bar", _a.foo), _a);
`)
}

func TestLowerOptionalChain(t *testing.T) {
	expectPrintedTarget(t, 2019, "a?.b.c", "a == null ? void 0 : a.b.c;\n")
	expectPrintedTarget(t, 2019, "(a?.b).c", "(a == null ? void 0 : a.b).c;\n")
	expectPrintedTarget(t, 2019, "a.b?.c", "var _a;\n(_a = a.b) == null ? void 0 : _a.c;\n")
	expectPrintedTarget(t, 2019, "this?.x", "this == null ? void 0 : this.x;\n")

	expectPrintedTarget(t, 2019, "a?.[b][c]", "a == null ? void 0 : a[b][c];\n")
	expectPrintedTarget(t, 2019, "(a?.[b])[c]", "(a == null ? void 0 : a[b])[c];\n")
	expectPrintedTarget(t, 2019, "a[b]?.[c]", "var _a;\n(_a = a[b]) == null ? void 0 : _a[c];\n")
	expectPrintedTarget(t, 2019, "this?.[x]", "this == null ? void 0 : this[x];\n")

	expectPrintedTarget(t, 2019, "a?.(b)(c)", "a == null ? void 0 : a(b)(c);\n")
	expectPrintedTarget(t, 2019, "(a?.(b))(c)", "(a == null ? void 0 : a(b))(c);\n")
	expectPrintedTarget(t, 2019, "a(b)?.(c)", "var _a;\n(_a = a(b)) == null ? void 0 : _a(c);\n")
	expectPrintedTarget(t, 2019, "this?.(x)", "this == null ? void 0 : this(x);\n")

	expectPrintedTarget(t, 2019, "delete a?.b.c", "a == null ? true : delete a.b.c;\n")
	expectPrintedTarget(t, 2019, "delete a?.[b][c]", "a == null ? true : delete a[b][c];\n")
	expectPrintedTarget(t, 2019, "delete a?.(b)(c)", "a == null ? true : delete a(b)(c);\n")

	expectPrintedTarget(t, 2019, "delete (a?.b).c", "delete (a == null ? void 0 : a.b).c;\n")
	expectPrintedTarget(t, 2019, "delete (a?.[b])[c]", "delete (a == null ? void 0 : a[b])[c];\n")
	expectPrintedTarget(t, 2019, "delete (a?.(b))(c)", "delete (a == null ? void 0 : a(b))(c);\n")

	expectPrintedTarget(t, 2019, "(delete a?.b).c", "(a == null ? true : delete a.b).c;\n")
	expectPrintedTarget(t, 2019, "(delete a?.[b])[c]", "(a == null ? true : delete a[b])[c];\n")
	expectPrintedTarget(t, 2019, "(delete a?.(b))(c)", "(a == null ? true : delete a(b))(c);\n")

	expectPrintedTarget(t, 2019, "null?.x", "void 0;\n")
	expectPrintedTarget(t, 2019, "null?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2019, "null?.(x)", "void 0;\n")

	expectPrintedTarget(t, 2019, "delete null?.x", "true;\n")
	expectPrintedTarget(t, 2019, "delete null?.[x]", "true;\n")
	expectPrintedTarget(t, 2019, "delete null?.(x)", "true;\n")

	expectPrintedTarget(t, 2019, "undefined?.x", "void 0;\n")
	expectPrintedTarget(t, 2019, "undefined?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2019, "undefined?.(x)", "void 0;\n")

	expectPrintedTarget(t, 2019, "delete undefined?.x", "true;\n")
	expectPrintedTarget(t, 2019, "delete undefined?.[x]", "true;\n")
	expectPrintedTarget(t, 2019, "delete undefined?.(x)", "true;\n")

	expectPrintedTarget(t, 2020, "x?.y", "x?.y;\n")
	expectPrintedTarget(t, 2020, "x?.[y]", "x?.[y];\n")
	expectPrintedTarget(t, 2020, "x?.(y)", "x?.(y);\n")

	expectPrintedTarget(t, 2020, "null?.x", "void 0;\n")
	expectPrintedTarget(t, 2020, "null?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2020, "null?.(x)", "void 0;\n")

	expectPrintedTarget(t, 2020, "undefined?.x", "void 0;\n")
	expectPrintedTarget(t, 2020, "undefined?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2020, "undefined?.(x)", "void 0;\n")

	// Check multiple levels of nesting
	expectPrintedTarget(t, 2019, "a?.b?.c?.d", `var _a, _b;
(_b = (_a = a == null ? void 0 : a.b) == null ? void 0 : _a.c) == null ? void 0 : _b.d;
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.[c]?.[d]", `var _a, _b;
(_b = (_a = a == null ? void 0 : a[b]) == null ? void 0 : _a[c]) == null ? void 0 : _b[d];
`)
	expectPrintedTarget(t, 2019, "a?.(b)?.(c)?.(d)", `var _a, _b;
(_b = (_a = a == null ? void 0 : a(b)) == null ? void 0 : _a(c)) == null ? void 0 : _b(d);
`)

	// Check the need to use ".call()"
	expectPrintedTarget(t, 2019, "a.b?.(c)", `var _a;
(_a = a.b) == null ? void 0 : _a.call(a, c);
`)
	expectPrintedTarget(t, 2019, "a[b]?.(c)", `var _a;
(_a = a[b]) == null ? void 0 : _a.call(a, c);
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c)", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c);
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c).d", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c).d;
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c).d()", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c).d();
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c)['d']", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c)["d"];
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c)['d']()", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c)["d"]();
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c).d['e'](f)['g'].h(i)", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c).d["e"](f)["g"].h(i);
`)
	expectPrintedTarget(t, 2019, "123?.[b]?.(c)", `var _a;
(_a = 123 == null ? void 0 : 123[b]) == null ? void 0 : _a.call(123, c);
`)
	expectPrintedTarget(t, 2019, "a?.[b][c]?.(d)", `var _a, _b;
(_b = a == null ? void 0 : (_a = a[b])[c]) == null ? void 0 : _b.call(_a, d);
`)
	expectPrintedTarget(t, 2019, "a[b][c]?.(d)", `var _a, _b;
(_b = (_a = a[b])[c]) == null ? void 0 : _b.call(_a, d);
`)

	// Check that direct eval status is propagated through optional chaining
	expectPrintedTarget(t, 2019, "eval?.(x)", "eval == null ? void 0 : eval(x);\n")
	expectPrintedMangleTarget(t, 2019, "(1 ? eval : 0)?.(x)", "eval == null || (0, eval)(x);\n")

	// Check super property access
	expectPrintedTarget(t, 2019, "class Foo extends Bar { foo() { super.bar?.() } }", `class Foo extends Bar {
  foo() {
    var _a;
    (_a = super.bar) == null ? void 0 : _a.call(this);
  }
}
`)
	expectPrintedTarget(t, 2019, "class Foo extends Bar { foo() { super['bar']?.() } }", `class Foo extends Bar {
  foo() {
    var _a;
    (_a = super["bar"]) == null ? void 0 : _a.call(this);
  }
}
`)
}

func TestLowerOptionalCatchBinding(t *testing.T) {
	expectPrintedTarget(t, 2019, "try {} catch {}", "try {\n} catch {\n}\n")
	expectPrintedTarget(t, 2018, "try {} catch {}", "try {\n} catch (e) {\n}\n")
}

func TestLowerExportStarAs(t *testing.T) {
	expectPrintedTarget(t, 2020, "export * as ns from 'path'", "export * as ns from \"path\";\n")
	expectPrintedTarget(t, 2019, "export * as ns from 'path'", "import * as ns from \"path\";\nexport {ns};\n")
}
