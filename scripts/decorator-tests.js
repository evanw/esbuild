var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __typeError = (msg) => {
  throw TypeError(msg);
};
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });
var __decoratorStrings = ["class", "method", "getter", "setter", "accessor", "field", "value", "get", "set"];
var __expectFn = (fn) => fn !== void 0 && typeof fn !== "function" ? __typeError("Function expected") : fn;
var __decoratorContext = (kind, name, done, fns) => ({ kind: __decoratorStrings[kind], name, addInitializer: (fn) => done._ ? __typeError("Already initialized") : fns.push(__expectFn(fn || null)) });
var __runInitializers = (array, flags, self, value) => {
  for (var i = 0, fns = array[flags >> 1], n = fns && fns.length; i < n; i++) flags & 1 ? fns[i].call(self) : value = fns[i].call(self, value);
  return value;
};
var __decorateElement = (array, flags, name, decorators, target, extra) => {
  var fn, it, done, ctx, access, k = flags & 7, s = !!(flags & 8), p = !!(flags & 16);
  var j = k > 3 ? array.length + 1 : k ? s ? 1 : 2 : 0, key = __decoratorStrings[k + 5];
  var initializers = k > 3 && (array[j - 1] = []), extraInitializers = array[j] || (array[j] = []);
  var desc = k && (!p && !s && (target = target.prototype), k < 5 && (k > 3 || !p) && __getOwnPropDesc(k < 4 ? target : { get [name]() {
    return __privateGet(this, extra);
  }, set [name](x) {
    return __privateSet(this, extra, x);
  } }, name));
  k ? p && k < 4 && __name(extra, (k > 2 ? "set " : k > 1 ? "get " : "") + name) : __name(target, name);
  for (var i = decorators.length - 1; i >= 0; i--) {
    ctx = __decoratorContext(k, name, done = {}, extraInitializers);
    if (k) {
      ctx.static = s, ctx.private = p, access = ctx.access = { has: p ? (x) => __privateIn(target, x) : (x) => name in x };
      if (k ^ 3) access.get = p ? (x) => (k ^ 1 ? __privateGet : __privateMethod)(x, target, k ^ 4 ? extra : desc.get) : (x) => x[name];
      if (k > 2) access.set = p ? (x, y) => __privateSet(x, target, y, k ^ 4 ? extra : desc.set) : (x, y) => x[name] = y;
    }
    it = (0, decorators[i])(k ? k < 4 ? p ? extra : desc[key] : k > 4 ? void 0 : { get: desc.get, set: desc.set } : target, ctx), done._ = 1;
    if (k ^ 4 || it === void 0) __expectFn(it) && (k > 4 ? initializers.unshift(it) : k ? p ? extra = it : desc[key] = it : target = it);
    else if (typeof it !== "object" || it === null) __typeError("Object expected");
    else __expectFn(fn = it.get) && (desc.get = fn), __expectFn(fn = it.set) && (desc.set = fn), __expectFn(fn = it.init) && initializers.unshift(fn);
  }
  return desc && __defProp(target, name, desc), p ? k ^ 4 ? extra : desc : target;
};
var __publicField = (obj, key, value) => __defNormalProp(obj, typeof key !== "symbol" ? key + "" : key, value);
var __accessCheck = (obj, member, msg) => member.has(obj) || __typeError("Cannot " + msg);
var __privateIn = (member, obj) => Object(obj) !== obj ? __typeError('Cannot use the "in" operator on this value') : member.has(obj);
var __privateGet = (obj, member, getter) => (__accessCheck(obj, member, "read from private field"), getter ? getter.call(obj) : member.get(obj));
var __privateAdd = (obj, member, value) => member.has(obj) ? __typeError("Cannot add the same private member more than once") : member instanceof WeakSet ? member.add(obj) : member.set(obj, value);
var __privateSet = (obj, member, value, setter) => (__accessCheck(obj, member, "write to private field"), setter ? setter.call(obj, value) : member.set(obj, value), value);
var __privateMethod = (obj, member, method) => (__accessCheck(obj, member, "access private method"), method);
const tests = {
  // Class decorators
  "Class decorators: Basic statement": () => {
    var _Foo_decorators, _init;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    _init = [, , ,];
    _Foo_decorators = [dec];
    class Foo2 {
    }
    Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
    __runInitializers(_init, 1, Foo2);
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Anonymous": () => {
    var _class_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    const Foo2 = /* @__PURE__ */ ((x) => x)((_init = [, , ,], _class_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "", _class_decorators, _a), __runInitializers(_init, 1, _a), _a));
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Property value": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    const obj = {
      Foo: (_init = [, , ,], _Foo_decorators = [dec], _a = class {
      }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a)
    };
    assertEq(() => obj.Foo, old);
  },
  "Class decorators: Basic expression: Variable initializer": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    const Foo2 = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a);
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Array binding": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    const [Foo2 = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a)] = [];
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Object binding": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    const { Foo: Foo2 = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a) } = {};
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Assignment initializer": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    let Foo2;
    Foo2 = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a);
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Assignment array binding": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    let Foo2;
    [Foo2 = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a)] = [];
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Assignment object binding": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    let Foo2;
    ({ Foo: Foo2 = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
    }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a) } = {});
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Instance field initializer": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    class Class {
      Foo = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
      }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a);
    }
    const Foo2 = new Class().Foo;
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Static field initializer": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    class Class {
      static Foo = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
      }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a);
    }
    assertEq(() => Class.Foo, old);
  },
  "Class decorators: Basic expression: Instance auto-accessor initializer": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    class Class {
      #Foo = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
      }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a);
      get Foo() {
        return this.#Foo;
      }
      set Foo(_) {
        this.#Foo = _;
      }
    }
    const Foo2 = new Class().Foo;
    assertEq(() => Foo2, old);
  },
  "Class decorators: Basic expression: Static auto-accessor initializer": () => {
    var _Foo_decorators, _init, _a;
    let old;
    const dec = (cls, ctx) => {
      assertEq(() => typeof cls, "function");
      assertEq(() => cls.name, "Foo");
      assertEq(() => ctx.kind, "class");
      assertEq(() => ctx.name, "Foo");
      assertEq(() => "static" in ctx, false);
      assertEq(() => "private" in ctx, false);
      assertEq(() => "access" in ctx, false);
      old = cls;
    };
    class Class {
      static #Foo = (_init = [, , ,], _Foo_decorators = [dec], _a = class {
      }, _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 1, _a), _a);
      static get Foo() {
        return this.#Foo;
      }
      static set Foo(_) {
        this.#Foo = _;
      }
    }
    assertEq(() => Class.Foo, old);
  },
  "Class decorators: Order": () => {
    var _Foo_decorators, _init;
    const log = [];
    let Bar;
    let Baz;
    const dec1 = (cls, ctx) => {
      log.push(2);
      Bar = function() {
        log.push(4);
        return new cls();
      };
      return Bar;
    };
    const dec2 = (cls, ctx) => {
      log.push(1);
      Baz = function() {
        log.push(5);
        return new cls();
      };
      return Baz;
    };
    log.push(0);
    _init = [, , ,];
    _Foo_decorators = [dec1, dec2];
    class Foo2 {
      constructor() {
        log.push(6);
      }
    }
    Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
    __runInitializers(_init, 1, Foo2);
    log.push(3);
    new Foo2();
    log.push(7);
    assertEq(() => Foo2, Bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Class decorators: Return null": () => {
    assertThrows(() => {
      var _Foo_decorators, _init;
      const dec = (cls, ctx) => {
        return null;
      };
      _init = [, , ,];
      _Foo_decorators = [dec];
      class Foo2 {
      }
      Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
      __runInitializers(_init, 1, Foo2);
    }, TypeError);
  },
  "Class decorators: Return object": () => {
    assertThrows(() => {
      var _Foo_decorators, _init;
      const dec = (cls, ctx) => {
        return {};
      };
      _init = [, , ,];
      _Foo_decorators = [dec];
      class Foo2 {
      }
      Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
      __runInitializers(_init, 1, Foo2);
    }, TypeError);
  },
  "Class decorators: Extra initializer": () => {
    var _Foo_decorators, _init;
    let oldAddInitializer;
    let got;
    const dec = (cls, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _Foo_decorators = [dec, dec];
    class Foo2 {
    }
    Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
    __runInitializers(_init, 1, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  // Method decorators
  "Method decorators: Basic (instance method)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const old = {};
    const dec = (key, name) => (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, name);
      assertEq(() => ctx.kind, "method");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => "set" in ctx.access, false);
      old[key] = fn;
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      foo() {
      }
      [(_foo_dec = [dec("foo", "foo")], _b = (_bar_dec = [dec(bar, "[bar]")], bar))]() {
      }
      [_a = (_baz_dec = [dec(baz, "")], baz)]() {
      }
    }
    __decorateElement(_init, 1, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 1, _b, _bar_dec, Foo2);
    __decorateElement(_init, 1, _a, _baz_dec, Foo2);
    assertEq(() => Foo2.prototype.foo, old["foo"]);
    assertEq(() => Foo2.prototype[bar], old[bar]);
    assertEq(() => Foo2.prototype[baz], old[baz]);
  },
  "Method decorators: Basic (static method)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const old = {};
    const dec = (key, name) => (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, name);
      assertEq(() => ctx.kind, "method");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => "set" in ctx.access, false);
      old[key] = fn;
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    class Foo2 {
      static foo() {
      }
      static [(_foo_dec = [dec("foo", "foo")], _b = (_bar_dec = [dec(bar, "[bar]")], bar))]() {
      }
      static [_a = (_baz_dec = [dec(baz, "")], baz)]() {
      }
    }
    __decorateElement(_init, 9, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 9, _b, _bar_dec, Foo2);
    __decorateElement(_init, 9, _a, _baz_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => Foo2.foo, old["foo"]);
    assertEq(() => Foo2[bar], old[bar]);
    assertEq(() => Foo2[baz], old[baz]);
  },
  "Method decorators: Basic (private instance method)": () => {
    var _foo_dec, _init, _Foo_instances, foo_fn;
    let old;
    let lateAsserts;
    const dec = (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, "#foo");
      assertEq(() => ctx.kind, "method");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(new Foo2()), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(new Foo2()), $foo);
        assertEq(() => "set" in ctx.access, false);
      };
      old = fn;
    };
    let $foo;
    _init = [, , ,];
    _foo_dec = [dec];
    const _Foo = class _Foo {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    };
    _Foo_instances = new WeakSet();
    foo_fn = function() {
    };
    foo_fn = __decorateElement(_init, 17, "#foo", _foo_dec, _Foo_instances, foo_fn);
    $foo = __privateMethod(new _Foo(), _Foo_instances, foo_fn);
    let Foo2 = _Foo;
    assertEq(() => $foo, old);
    lateAsserts();
  },
  "Method decorators: Basic (private static method)": () => {
    var _foo_dec, _init, _Foo_static, foo_fn;
    let old;
    let lateAsserts;
    const dec = (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, "#foo");
      assertEq(() => ctx.kind, "method");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(Foo2), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(Foo2), $foo);
        assertEq(() => "set" in ctx.access, false);
      };
      old = fn;
    };
    let $foo;
    _init = [, , ,];
    _foo_dec = [dec];
    const _Foo = class _Foo {
    };
    _Foo_static = new WeakSet();
    foo_fn = function() {
    };
    foo_fn = __decorateElement(_init, 25, "#foo", _foo_dec, _Foo_static, foo_fn);
    __privateAdd(_Foo, _Foo_static);
    __runInitializers(_init, 3, _Foo);
    $foo = __privateMethod(_Foo, _Foo_static, foo_fn);
    let Foo2 = _Foo;
    assertEq(() => $foo, old);
    lateAsserts();
  },
  "Method decorators: Shim (instance method)": () => {
    var _foo_dec, _init;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "bar", 123);
      }
      foo() {
        return this.bar;
      }
    }
    __decorateElement(_init, 1, "foo", _foo_dec, Foo2);
    assertEq(() => Foo2.prototype.foo, bar);
    assertEq(() => new Foo2().foo(), 124);
  },
  "Method decorators: Shim (static method)": () => {
    var _foo_dec, _init;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      static foo() {
        return this.bar;
      }
    }
    __decorateElement(_init, 9, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 123);
    assertEq(() => Foo2.foo, bar);
    assertEq(() => Foo2.foo(), 124);
  },
  "Method decorators: Shim (private instance method)": () => {
    var _foo_dec, _init, _Foo_instances, foo_fn;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    let $foo;
    _init = [, , ,];
    _foo_dec = [dec];
    const _Foo = class _Foo {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
        __publicField(this, "bar", 123);
      }
    };
    _Foo_instances = new WeakSet();
    foo_fn = function() {
      return this.bar;
    };
    foo_fn = __decorateElement(_init, 17, "#foo", _foo_dec, _Foo_instances, foo_fn);
    $foo = __privateMethod(new _Foo(), _Foo_instances, foo_fn);
    let Foo2 = _Foo;
    assertEq(() => $foo, bar);
    assertEq(() => bar.call(new Foo2()), 124);
  },
  "Method decorators: Shim (private static method)": () => {
    var _foo_dec, _init, _Foo_static, foo_fn;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    let $foo;
    _init = [, , ,];
    _foo_dec = [dec];
    const _Foo = class _Foo {
    };
    _Foo_static = new WeakSet();
    foo_fn = function() {
      return this.bar;
    };
    foo_fn = __decorateElement(_init, 25, "#foo", _foo_dec, _Foo_static, foo_fn);
    __privateAdd(_Foo, _Foo_static);
    __runInitializers(_init, 3, _Foo);
    __publicField(_Foo, "bar", 123);
    $foo = __privateMethod(_Foo, _Foo_static, foo_fn);
    let Foo2 = _Foo;
    assertEq(() => $foo, bar);
    assertEq(() => bar.call(Foo2), 124);
  },
  "Method decorators: Order (instance method)": () => {
    var _foo_dec, _init;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      foo() {
        return log.push(6);
      }
    }
    __decorateElement(_init, 1, "foo", _foo_dec, Foo2);
    log.push(3);
    new Foo2().foo();
    log.push(7);
    assertEq(() => Foo2.prototype.foo, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Method decorators: Order (static method)": () => {
    var _foo_dec, _init;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      static foo() {
        return log.push(6);
      }
    }
    __decorateElement(_init, 9, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    log.push(3);
    Foo2.foo();
    log.push(7);
    assertEq(() => Foo2.foo, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Method decorators: Order (private instance method)": () => {
    var _foo_dec, _init, _Foo_instances, foo_fn;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    let $foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    const _Foo = class _Foo {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    };
    _Foo_instances = new WeakSet();
    foo_fn = function() {
      return log.push(6);
    };
    foo_fn = __decorateElement(_init, 17, "#foo", _foo_dec, _Foo_instances, foo_fn);
    $foo = __privateMethod(new _Foo(), _Foo_instances, foo_fn);
    let Foo2 = _Foo;
    log.push(3);
    $foo.call(new Foo2());
    log.push(7);
    assertEq(() => $foo, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Method decorators: Order (private static method)": () => {
    var _foo_dec, _init, _Foo_static, foo_fn;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    let $foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    const _Foo = class _Foo {
    };
    _Foo_static = new WeakSet();
    foo_fn = function() {
      return log.push(6);
    };
    foo_fn = __decorateElement(_init, 25, "#foo", _foo_dec, _Foo_static, foo_fn);
    __privateAdd(_Foo, _Foo_static);
    __runInitializers(_init, 3, _Foo);
    $foo = __privateMethod(_Foo, _Foo_static, foo_fn);
    let Foo2 = _Foo;
    log.push(3);
    $foo.call(Foo2);
    log.push(7);
    assertEq(() => $foo, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Method decorators: Return null (instance method)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
        }
        foo() {
        }
      }
      __decorateElement(_init, 1, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Method decorators: Return null (static method)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        static foo() {
        }
      }
      __decorateElement(_init, 9, "foo", _foo_dec, Foo2);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Method decorators: Return null (private instance method)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_instances, foo_fn;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
          __privateAdd(this, _Foo_instances);
        }
      }
      _Foo_instances = new WeakSet();
      foo_fn = function() {
      };
      foo_fn = __decorateElement(_init, 17, "#foo", _foo_dec, _Foo_instances, foo_fn);
    }, TypeError);
  },
  "Method decorators: Return null (private static method)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_static, foo_fn;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _Foo_static = new WeakSet();
      foo_fn = function() {
      };
      foo_fn = __decorateElement(_init, 25, "#foo", _foo_dec, _Foo_static, foo_fn);
      __privateAdd(Foo2, _Foo_static);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Method decorators: Return object (instance method)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
        }
        foo() {
        }
      }
      __decorateElement(_init, 1, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Method decorators: Return object (static method)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        static foo() {
        }
      }
      __decorateElement(_init, 9, "foo", _foo_dec, Foo2);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Method decorators: Return object (private instance method)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_instances, foo_fn;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
          __privateAdd(this, _Foo_instances);
        }
      }
      _Foo_instances = new WeakSet();
      foo_fn = function() {
      };
      foo_fn = __decorateElement(_init, 17, "#foo", _foo_dec, _Foo_instances, foo_fn);
    }, TypeError);
  },
  "Method decorators: Return object (private static method)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_static, foo_fn;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _Foo_static = new WeakSet();
      foo_fn = function() {
      };
      foo_fn = __decorateElement(_init, 25, "#foo", _foo_dec, _Foo_static, foo_fn);
      __privateAdd(Foo2, _Foo_static);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Method decorators: Extra initializer (instance method)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      foo() {
      }
    }
    __decorateElement(_init, 1, "foo", _foo_dec, Foo2);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Method decorators: Extra initializer (static method)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      static foo() {
      }
    }
    __decorateElement(_init, 9, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  "Method decorators: Extra initializer (private instance method)": () => {
    var _foo_dec, _init, _Foo_instances, foo_fn;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    }
    _Foo_instances = new WeakSet();
    foo_fn = function() {
    };
    foo_fn = __decorateElement(_init, 17, "#foo", _foo_dec, _Foo_instances, foo_fn);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Method decorators: Extra initializer (private static method)": () => {
    var _foo_dec, _init, _Foo_static, foo_fn;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_fn = function() {
    };
    foo_fn = __decorateElement(_init, 25, "#foo", _foo_dec, _Foo_static, foo_fn);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  // Field decorators
  "Field decorators: Basic (instance field)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const dec = (key) => (value, ctx) => {
      assertEq(() => value, void 0);
      assertEq(() => ctx.kind, "field");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => {
        const obj = {};
        ctx.access.set(obj, 321);
        return obj[key];
      }, 321);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    _foo_dec = [dec("foo")], _b = (_bar_dec = [dec(bar)], bar), _a = (_baz_dec = [dec(baz)], baz);
    class Foo2 {
      constructor() {
        __publicField(this, "foo", __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
        __publicField(this, _b, __runInitializers(_init, 10, this, 123)), __runInitializers(_init, 13, this);
        __publicField(this, _a, __runInitializers(_init, 14, this, 123)), __runInitializers(_init, 17, this);
      }
    }
    __decorateElement(_init, 5, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 5, _b, _bar_dec, Foo2);
    __decorateElement(_init, 5, _a, _baz_dec, Foo2);
    assertEq(() => new Foo2().foo, 123);
    assertEq(() => new Foo2()[bar], 123);
    assertEq(() => new Foo2()[baz], 123);
  },
  "Field decorators: Basic (static field)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const dec = (key) => (value, ctx) => {
      assertEq(() => value, void 0);
      assertEq(() => ctx.kind, "field");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => {
        const obj = {};
        ctx.access.set(obj, 321);
        return obj[key];
      }, 321);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    _foo_dec = [dec("foo")], _b = (_bar_dec = [dec(bar)], bar), _a = (_baz_dec = [dec(baz)], baz);
    class Foo2 {
    }
    __decorateElement(_init, 13, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 13, _b, _bar_dec, Foo2);
    __decorateElement(_init, 13, _a, _baz_dec, Foo2);
    __publicField(Foo2, "foo", __runInitializers(_init, 6, Foo2, 123)), __runInitializers(_init, 9, Foo2);
    __publicField(Foo2, _b, __runInitializers(_init, 10, Foo2, 123)), __runInitializers(_init, 13, Foo2);
    __publicField(Foo2, _a, __runInitializers(_init, 14, Foo2, 123)), __runInitializers(_init, 17, Foo2);
    assertEq(() => Foo2.foo, 123);
    assertEq(() => Foo2[bar], 123);
    assertEq(() => Foo2[baz], 123);
  },
  "Field decorators: Basic (private instance field)": () => {
    var _foo_dec, _init, _foo;
    let lateAsserts;
    const dec = (value, ctx) => {
      assertEq(() => value, void 0);
      assertEq(() => ctx.kind, "field");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(new Foo2()), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(new Foo2()), 123);
        assertEq(() => {
          const obj = new Foo2();
          ctx.access.set(obj, 321);
          return get$foo(obj);
        }, 321);
      };
    };
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    __decorateElement(_init, 21, "#foo", _foo_dec, _foo);
    get$foo = (x) => __privateGet(x, _foo);
    assertEq(() => get$foo(new Foo2()), 123);
    lateAsserts();
  },
  "Field decorators: Basic (private static field)": () => {
    var _foo_dec, _init, _foo;
    let lateAsserts;
    const dec = (value, ctx) => {
      assertEq(() => value, void 0);
      assertEq(() => ctx.kind, "field");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(Foo2), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(Foo2), 123);
        assertEq(() => {
          ctx.access.set(Foo2, 321);
          return get$foo(Foo2);
        }, 321);
      };
    };
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
    }
    _foo = new WeakMap();
    __decorateElement(_init, 29, "#foo", _foo_dec, _foo);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2, 123)), __runInitializers(_init, 9, Foo2);
    get$foo = (x) => __privateGet(x, _foo);
    assertEq(() => get$foo(Foo2), 123);
    lateAsserts();
  },
  "Field decorators: Shim (instance field)": () => {
    var _bar_dec, _foo_dec, _init;
    let log = [];
    const dec = (value, ctx) => {
      return function(x) {
        assertEq(() => this instanceof Foo2, true);
        return log.push("foo" in this, "bar" in this, x);
      };
    };
    _init = [, , ,];
    _foo_dec = [dec], _bar_dec = [dec];
    class Foo2 {
      constructor() {
        __publicField(this, "foo", __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
        __publicField(this, "bar", __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
      }
    }
    __decorateElement(_init, 5, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 5, "bar", _bar_dec, Foo2);
    assertEq(() => log + "", "");
    var obj = new Foo2();
    assertEq(() => obj.foo, 3);
    assertEq(() => obj.bar, 6);
    assertEq(() => log + "", "false,false,123,true,false,");
  },
  "Field decorators: Shim (static field)": () => {
    var _bar_dec, _foo_dec, _init;
    let foo;
    let log = [];
    const dec = (value, ctx) => {
      return function(x) {
        assertEq(() => this, foo);
        return log.push("foo" in this, "bar" in this, x);
      };
    };
    assertEq(() => log + "", "");
    _init = [, , ,];
    _foo_dec = [dec], _bar_dec = [dec];
    const _Foo = class _Foo {
    };
    __decorateElement(_init, 13, "foo", _foo_dec, _Foo);
    __decorateElement(_init, 13, "bar", _bar_dec, _Foo);
    foo = _Foo;
    __publicField(_Foo, "foo", __runInitializers(_init, 6, _Foo, 123)), __runInitializers(_init, 9, _Foo);
    __publicField(_Foo, "bar", __runInitializers(_init, 10, _Foo)), __runInitializers(_init, 13, _Foo);
    let Foo2 = _Foo;
    assertEq(() => Foo2.foo, 3);
    assertEq(() => Foo2.bar, 6);
    assertEq(() => log + "", "false,false,123,true,false,");
  },
  "Field decorators: Shim (private instance field)": () => {
    var _bar_dec, _foo_dec, _init, _foo, _bar;
    let log = [];
    const dec = (value, ctx) => {
      return function(x) {
        assertEq(() => this instanceof Foo2, true);
        return log.push(has$foo(this), has$bar(this), x);
      };
    };
    let has$foo;
    let has$bar;
    let get$foo;
    let get$bar;
    _init = [, , ,];
    _foo_dec = [dec], _bar_dec = [dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
        __privateAdd(this, _bar, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
      }
    }
    _foo = new WeakMap();
    _bar = new WeakMap();
    __decorateElement(_init, 21, "#foo", _foo_dec, _foo);
    __decorateElement(_init, 21, "#bar", _bar_dec, _bar);
    has$foo = (x) => __privateIn(_foo, x);
    has$bar = (x) => __privateIn(_bar, x);
    get$foo = (x) => __privateGet(x, _foo);
    get$bar = (x) => __privateGet(x, _bar);
    assertEq(() => log + "", "");
    var obj = new Foo2();
    assertEq(() => get$foo(obj), 3);
    assertEq(() => get$bar(obj), 6);
    assertEq(() => log + "", "false,false,123,true,false,");
  },
  "Field decorators: Shim (private static field)": () => {
    var _bar_dec, _foo_dec, _init, _foo, _bar;
    let foo;
    let log = [];
    const dec = (value, ctx) => {
      return function(x) {
        assertEq(() => this, foo);
        return log.push(has$foo(this), has$bar(this), x);
      };
    };
    assertEq(() => log + "", "");
    let has$foo;
    let has$bar;
    let get$foo;
    let get$bar;
    _init = [, , ,];
    _foo_dec = [dec], _bar_dec = [dec];
    const _Foo = class _Foo {
    };
    _foo = new WeakMap();
    _bar = new WeakMap();
    __decorateElement(_init, 29, "#foo", _foo_dec, _foo);
    __decorateElement(_init, 29, "#bar", _bar_dec, _bar);
    foo = _Foo;
    has$foo = (x) => __privateIn(_foo, x);
    has$bar = (x) => __privateIn(_bar, x);
    get$foo = (x) => __privateGet(x, _foo);
    get$bar = (x) => __privateGet(x, _bar);
    __privateAdd(_Foo, _foo, __runInitializers(_init, 6, _Foo, 123)), __runInitializers(_init, 9, _Foo);
    __privateAdd(_Foo, _bar, __runInitializers(_init, 10, _Foo)), __runInitializers(_init, 13, _Foo);
    let Foo2 = _Foo;
    assertEq(() => get$foo(Foo2), 3);
    assertEq(() => get$bar(Foo2), 6);
    assertEq(() => log + "", "false,false,123,true,false,");
  },
  "Field decorators: Order (instance field)": () => {
    var _foo_dec, _init;
    const log = [];
    const dec1 = (value, ctx) => {
      log.push(2);
      return () => log.push(4);
    };
    const dec2 = (value, ctx) => {
      log.push(1);
      return () => log.push(5);
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __publicField(this, "foo", __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
      }
    }
    __decorateElement(_init, 5, "foo", _foo_dec, Foo2);
    log.push(3);
    var obj = new Foo2();
    log.push(6);
    assertEq(() => obj.foo, 6);
    assertEq(() => log + "", "0,1,2,3,4,5,6");
  },
  "Field decorators: Order (static field)": () => {
    var _foo_dec, _init;
    const log = [];
    const dec1 = (value, ctx) => {
      log.push(2);
      return () => log.push(3);
    };
    const dec2 = (value, ctx) => {
      log.push(1);
      return () => log.push(4);
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
    }
    __decorateElement(_init, 13, "foo", _foo_dec, Foo2);
    __publicField(Foo2, "foo", __runInitializers(_init, 6, Foo2, 123)), __runInitializers(_init, 9, Foo2);
    log.push(5);
    assertEq(() => Foo2.foo, 5);
    assertEq(() => log + "", "0,1,2,3,4,5");
  },
  "Field decorators: Order (private instance field)": () => {
    var _foo_dec, _init, _foo;
    const log = [];
    const dec1 = (value, ctx) => {
      log.push(2);
      return () => log.push(4);
    };
    const dec2 = (value, ctx) => {
      log.push(1);
      return () => log.push(5);
    };
    log.push(0);
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    __decorateElement(_init, 21, "#foo", _foo_dec, _foo);
    get$foo = (x) => __privateGet(x, _foo);
    log.push(3);
    var obj = new Foo2();
    log.push(6);
    assertEq(() => get$foo(obj), 6);
    assertEq(() => log + "", "0,1,2,3,4,5,6");
  },
  "Field decorators: Order (private static field)": () => {
    var _foo_dec, _init, _foo;
    const log = [];
    const dec1 = (value, ctx) => {
      log.push(2);
      return () => log.push(3);
    };
    const dec2 = (value, ctx) => {
      log.push(1);
      return () => log.push(4);
    };
    log.push(0);
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
    }
    _foo = new WeakMap();
    __decorateElement(_init, 29, "#foo", _foo_dec, _foo);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2, 123)), __runInitializers(_init, 9, Foo2);
    get$foo = (x) => __privateGet(x, _foo);
    log.push(5);
    assertEq(() => get$foo(Foo2), 5);
    assertEq(() => log + "", "0,1,2,3,4,5");
  },
  "Field decorators: Return null (instance field)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (value, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __publicField(this, "foo", __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
        }
      }
      __decorateElement(_init, 5, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Field decorators: Return null (static field)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (value, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      __decorateElement(_init, 13, "foo", _foo_dec, Foo2);
      __publicField(Foo2, "foo", __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    }, TypeError);
  },
  "Field decorators: Return null (private instance field)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo;
      const dec = (value, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
        }
      }
      _foo = new WeakMap();
      __decorateElement(_init, 21, "#foo", _foo_dec, _foo);
    }, TypeError);
  },
  "Field decorators: Return null (private static field)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo;
      const dec = (value, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _foo = new WeakMap();
      __decorateElement(_init, 29, "#foo", _foo_dec, _foo);
      __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    }, TypeError);
  },
  "Field decorators: Return object (instance field)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (value, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __publicField(this, "foo", __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
        }
      }
      __decorateElement(_init, 5, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Field decorators: Return object (static field)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (value, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      __decorateElement(_init, 13, "foo", _foo_dec, Foo2);
      __publicField(Foo2, "foo", __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    }, TypeError);
  },
  "Field decorators: Return object (private instance field)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo;
      const dec = (value, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
        }
      }
      _foo = new WeakMap();
      __decorateElement(_init, 21, "#foo", _foo_dec, _foo);
    }, TypeError);
  },
  "Field decorators: Return object (private static field)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo;
      const dec = (value, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _foo = new WeakMap();
      __decorateElement(_init, 29, "#foo", _foo_dec, _foo);
      __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    }, TypeError);
  },
  "Field decorators: Extra initializer (instance field)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (value, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __publicField(this, "foo", __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
      }
    }
    __decorateElement(_init, 5, "foo", _foo_dec, Foo2);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Field decorators: Extra initializer (static field)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (value, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    __decorateElement(_init, 13, "foo", _foo_dec, Foo2);
    __publicField(Foo2, "foo", __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  "Field decorators: Extra initializer (private instance field)": () => {
    var _foo_dec, _init, _foo;
    let oldAddInitializer;
    let got;
    const dec = (value, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    __decorateElement(_init, 21, "#foo", _foo_dec, _foo);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Field decorators: Extra initializer (private static field)": () => {
    var _foo_dec, _init, _foo;
    let oldAddInitializer;
    let got;
    const dec = (value, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    _foo = new WeakMap();
    __decorateElement(_init, 29, "#foo", _foo_dec, _foo);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  // Getter decorators
  "Getter decorators: Basic (instance getter)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const dec = (key, name) => (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, name);
      assertEq(() => ctx.kind, "getter");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => "set" in ctx.access, false);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "bar", 123);
      }
      get foo() {
        return this.bar;
      }
      get [(_foo_dec = [dec("foo", "get foo")], _b = (_bar_dec = [dec(bar, "get [bar]")], bar))]() {
        return this.bar;
      }
      get [_a = (_baz_dec = [dec(baz, "get ")], baz)]() {
        return this.bar;
      }
    }
    __decorateElement(_init, 2, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 2, _b, _bar_dec, Foo2);
    __decorateElement(_init, 2, _a, _baz_dec, Foo2);
    assertEq(() => new Foo2().foo, 123);
    assertEq(() => new Foo2()[bar], 123);
    assertEq(() => new Foo2()[baz], 123);
  },
  "Getter decorators: Basic (static getter)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const dec = (key, name) => (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, name);
      assertEq(() => ctx.kind, "getter");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => "set" in ctx.access, false);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    class Foo2 {
      static get foo() {
        return this.bar;
      }
      static get [(_foo_dec = [dec("foo", "get foo")], _b = (_bar_dec = [dec(bar, "get [bar]")], bar))]() {
        return this.bar;
      }
      static get [_a = (_baz_dec = [dec(baz, "get ")], baz)]() {
        return this.bar;
      }
    }
    __decorateElement(_init, 10, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 10, _b, _bar_dec, Foo2);
    __decorateElement(_init, 10, _a, _baz_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 123);
    assertEq(() => Foo2.foo, 123);
    assertEq(() => Foo2[bar], 123);
    assertEq(() => Foo2[baz], 123);
  },
  "Getter decorators: Basic (private instance getter)": () => {
    var _foo_dec, _bar, _init, _Foo_instances, foo_get;
    let lateAsserts;
    const dec = (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, "get #foo");
      assertEq(() => ctx.kind, "getter");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(new Foo2()), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(new Foo2()), 123);
        assertEq(() => "set" in ctx.access, false);
      };
    };
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
        __privateAdd(this, _bar, 123);
      }
    }
    _bar = new WeakMap();
    _Foo_instances = new WeakSet();
    foo_get = function() {
      return __privateGet(this, _bar);
    };
    foo_get = __decorateElement(_init, 18, "#foo", _foo_dec, _Foo_instances, foo_get);
    get$foo = (x) => __privateGet(x, _Foo_instances, foo_get);
    assertEq(() => get$foo(new Foo2()), 123);
    lateAsserts();
  },
  "Getter decorators: Basic (private static getter)": () => {
    var _foo_dec, _bar, _init, _Foo_static, foo_get;
    let lateAsserts;
    const dec = (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, "get #foo");
      assertEq(() => ctx.kind, "getter");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(Foo2), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(Foo2), 123);
        assertEq(() => "set" in ctx.access, false);
      };
    };
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
    }
    _bar = new WeakMap();
    _Foo_static = new WeakSet();
    foo_get = function() {
      return __privateGet(this, _bar);
    };
    foo_get = __decorateElement(_init, 26, "#foo", _foo_dec, _Foo_static, foo_get);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    __privateAdd(Foo2, _bar, 123);
    get$foo = (x) => __privateGet(x, _Foo_static, foo_get);
    assertEq(() => get$foo(Foo2), 123);
    lateAsserts();
  },
  "Getter decorators: Shim (instance getter)": () => {
    var _foo_dec, _init;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "bar", 123);
      }
      get foo() {
        return this.bar;
      }
    }
    __decorateElement(_init, 2, "foo", _foo_dec, Foo2);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2.prototype, "foo").get, bar);
    assertEq(() => new Foo2().foo, 124);
  },
  "Getter decorators: Shim (static getter)": () => {
    var _foo_dec, _init;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      static get foo() {
        return this.bar;
      }
    }
    __decorateElement(_init, 10, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 123);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2, "foo").get, bar);
    assertEq(() => Foo2.foo, 124);
  },
  "Getter decorators: Shim (private instance getter)": () => {
    var _foo_dec, _bar, _init, _Foo_instances, foo_get;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
        __privateAdd(this, _bar, 123);
      }
    }
    _bar = new WeakMap();
    _Foo_instances = new WeakSet();
    foo_get = function() {
      return __privateGet(this, _bar);
    };
    foo_get = __decorateElement(_init, 18, "#foo", _foo_dec, _Foo_instances, foo_get);
    get$foo = (x) => __privateGet(x, _Foo_instances, foo_get);
    assertEq(() => get$foo(new Foo2()), 124);
  },
  "Getter decorators: Shim (private static getter)": () => {
    var _foo_dec, _bar, _init, _Foo_static, foo_get;
    let bar;
    const dec = (fn, ctx) => {
      bar = function() {
        return fn.call(this) + 1;
      };
      return bar;
    };
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
    }
    _bar = new WeakMap();
    _Foo_static = new WeakSet();
    foo_get = function() {
      return __privateGet(this, _bar);
    };
    foo_get = __decorateElement(_init, 26, "#foo", _foo_dec, _Foo_static, foo_get);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    __privateAdd(Foo2, _bar, 123);
    get$foo = (x) => __privateGet(x, _Foo_static, foo_get);
    assertEq(() => get$foo(Foo2), 124);
  },
  "Getter decorators: Order (instance getter)": () => {
    var _foo_dec, _init;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      get foo() {
        return log.push(6);
      }
    }
    __decorateElement(_init, 2, "foo", _foo_dec, Foo2);
    log.push(3);
    new Foo2().foo;
    log.push(7);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2.prototype, "foo").get, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Getter decorators: Order (static getter)": () => {
    var _foo_dec, _init;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      static get foo() {
        return log.push(6);
      }
    }
    __decorateElement(_init, 10, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    log.push(3);
    Foo2.foo;
    log.push(7);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2, "foo").get, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Getter decorators: Order (private instance getter)": () => {
    var _foo_dec, _init, _Foo_instances, foo_get;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    }
    _Foo_instances = new WeakSet();
    foo_get = function() {
      return log.push(6);
    };
    foo_get = __decorateElement(_init, 18, "#foo", _foo_dec, _Foo_instances, foo_get);
    get$foo = (x) => __privateGet(x, _Foo_instances, foo_get);
    log.push(3);
    assertEq(() => get$foo(new Foo2()), 7);
    log.push(7);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Getter decorators: Order (private static getter)": () => {
    var _foo_dec, _init, _Foo_static, foo_get;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function() {
        log.push(4);
        return fn.call(this);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function() {
        log.push(5);
        return fn.call(this);
      };
      return baz;
    };
    log.push(0);
    let get$foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_get = function() {
      return log.push(6);
    };
    foo_get = __decorateElement(_init, 26, "#foo", _foo_dec, _Foo_static, foo_get);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    get$foo = (x) => __privateGet(x, _Foo_static, foo_get);
    log.push(3);
    assertEq(() => get$foo(Foo2), 7);
    log.push(7);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Getter decorators: Return null (instance getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
        }
        get foo() {
          return;
        }
      }
      __decorateElement(_init, 2, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Getter decorators: Return null (static getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        static get foo() {
          return;
        }
      }
      __decorateElement(_init, 10, "foo", _foo_dec, Foo2);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Getter decorators: Return null (private instance getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_instances, foo_get;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
          __privateAdd(this, _Foo_instances);
        }
      }
      _Foo_instances = new WeakSet();
      foo_get = function() {
        return;
      };
      foo_get = __decorateElement(_init, 18, "#foo", _foo_dec, _Foo_instances, foo_get);
    }, TypeError);
  },
  "Getter decorators: Return null (private static getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_static, foo_get;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _Foo_static = new WeakSet();
      foo_get = function() {
        return;
      };
      foo_get = __decorateElement(_init, 26, "#foo", _foo_dec, _Foo_static, foo_get);
      __privateAdd(Foo2, _Foo_static);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Getter decorators: Return object (instance getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
        }
        get foo() {
          return;
        }
      }
      __decorateElement(_init, 2, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Getter decorators: Return object (static getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        static get foo() {
          return;
        }
      }
      __decorateElement(_init, 10, "foo", _foo_dec, Foo2);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Getter decorators: Return object (private instance getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_instances, foo_get;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
          __privateAdd(this, _Foo_instances);
        }
      }
      _Foo_instances = new WeakSet();
      foo_get = function() {
        return;
      };
      foo_get = __decorateElement(_init, 18, "#foo", _foo_dec, _Foo_instances, foo_get);
    }, TypeError);
  },
  "Getter decorators: Return object (private static getter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_static, foo_get;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _Foo_static = new WeakSet();
      foo_get = function() {
        return;
      };
      foo_get = __decorateElement(_init, 26, "#foo", _foo_dec, _Foo_static, foo_get);
      __privateAdd(Foo2, _Foo_static);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Getter decorators: Extra initializer (instance getter)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      get foo() {
        return;
      }
    }
    __decorateElement(_init, 2, "foo", _foo_dec, Foo2);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Getter decorators: Extra initializer (static getter)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      static get foo() {
        return;
      }
    }
    __decorateElement(_init, 10, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  "Getter decorators: Extra initializer (private instance getter)": () => {
    var _foo_dec, _init, _Foo_instances, foo_get;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    }
    _Foo_instances = new WeakSet();
    foo_get = function() {
      return;
    };
    foo_get = __decorateElement(_init, 18, "#foo", _foo_dec, _Foo_instances, foo_get);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Getter decorators: Extra initializer (private static getter)": () => {
    var _foo_dec, _init, _Foo_static, foo_get;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_get = function() {
      return;
    };
    foo_get = __decorateElement(_init, 26, "#foo", _foo_dec, _Foo_static, foo_get);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  // Setter decorators
  "Setter decorators: Basic (instance setter)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const dec = (key, name) => (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, name);
      assertEq(() => ctx.kind, "setter");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => "get" in ctx.access, false);
      const obj2 = {};
      ctx.access.set(obj2, 123);
      assertEq(() => obj2[key], 123);
      assertEq(() => "bar" in obj2, false);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "bar", 0);
      }
      set foo(x) {
        this.bar = x;
      }
      set [(_foo_dec = [dec("foo", "set foo")], _b = (_bar_dec = [dec(bar, "set [bar]")], bar))](x) {
        this.bar = x;
      }
      set [_a = (_baz_dec = [dec(baz, "set ")], baz)](x) {
        this.bar = x;
      }
    }
    __decorateElement(_init, 3, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 3, _b, _bar_dec, Foo2);
    __decorateElement(_init, 3, _a, _baz_dec, Foo2);
    var obj = new Foo2();
    obj.foo = 321;
    assertEq(() => obj.bar, 321);
    obj[bar] = 4321;
    assertEq(() => obj.bar, 4321);
    obj[baz] = 54321;
    assertEq(() => obj.bar, 54321);
  },
  "Setter decorators: Basic (static setter)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init;
    const dec = (key, name) => (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, name);
      assertEq(() => ctx.kind, "setter");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => "get" in ctx.access, false);
      const obj = {};
      ctx.access.set(obj, 123);
      assertEq(() => obj[key], 123);
      assertEq(() => "bar" in obj, false);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    class Foo2 {
      static set foo(x) {
        this.bar = x;
      }
      static set [(_foo_dec = [dec("foo", "set foo")], _b = (_bar_dec = [dec(bar, "set [bar]")], bar))](x) {
        this.bar = x;
      }
      static set [_a = (_baz_dec = [dec(baz, "set ")], baz)](x) {
        this.bar = x;
      }
    }
    __decorateElement(_init, 11, "foo", _foo_dec, Foo2);
    __decorateElement(_init, 11, _b, _bar_dec, Foo2);
    __decorateElement(_init, 11, _a, _baz_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 0);
    Foo2.foo = 321;
    assertEq(() => Foo2.bar, 321);
    Foo2[bar] = 4321;
    assertEq(() => Foo2.bar, 4321);
    Foo2[baz] = 54321;
    assertEq(() => Foo2.bar, 54321);
  },
  "Setter decorators: Basic (private instance setter)": () => {
    var _foo_dec, _init, _Foo_instances, foo_set;
    let lateAsserts;
    const dec = (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, "set #foo");
      assertEq(() => ctx.kind, "setter");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(new Foo2()), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => "get" in ctx.access, false);
        assertEq(() => {
          const obj2 = new Foo2();
          ctx.access.set(obj2, 123);
          return obj2.bar;
        }, 123);
      };
    };
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
        __publicField(this, "bar", 0);
      }
    }
    _Foo_instances = new WeakSet();
    foo_set = function(x) {
      this.bar = x;
    };
    foo_set = __decorateElement(_init, 19, "#foo", _foo_dec, _Foo_instances, foo_set);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_instances, y, foo_set);
    };
    lateAsserts();
    var obj = new Foo2();
    assertEq(() => set$foo(obj, 321), void 0);
    assertEq(() => obj.bar, 321);
  },
  "Setter decorators: Basic (private static setter)": () => {
    var _foo_dec, _init, _Foo_static, foo_set;
    let lateAsserts;
    const dec = (fn, ctx) => {
      assertEq(() => typeof fn, "function");
      assertEq(() => fn.name, "set #foo");
      assertEq(() => ctx.kind, "setter");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(Foo2), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => "get" in ctx.access, false);
        assertEq(() => {
          ctx.access.set(Foo2, 123);
          return Foo2.bar;
        }, 123);
      };
    };
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_set = function(x) {
      this.bar = x;
    };
    foo_set = __decorateElement(_init, 27, "#foo", _foo_dec, _Foo_static, foo_set);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 0);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_static, y, foo_set);
    };
    lateAsserts();
    assertEq(() => set$foo(Foo2, 321), void 0);
    assertEq(() => Foo2.bar, 321);
  },
  "Setter decorators: Shim (instance setter)": () => {
    var _foo_dec, _init;
    let bar;
    const dec = (fn, ctx) => {
      bar = function(x) {
        fn.call(this, x + 1);
      };
      return bar;
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "bar", 123);
      }
      set foo(x) {
        this.bar = x;
      }
    }
    __decorateElement(_init, 3, "foo", _foo_dec, Foo2);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2.prototype, "foo").set, bar);
    var obj = new Foo2();
    obj.foo = 321;
    assertEq(() => obj.bar, 322);
  },
  "Setter decorators: Shim (static setter)": () => {
    var _foo_dec, _init;
    let bar;
    const dec = (fn, ctx) => {
      bar = function(x) {
        fn.call(this, x + 1);
      };
      return bar;
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      static set foo(x) {
        this.bar = x;
      }
    }
    __decorateElement(_init, 11, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 123);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2, "foo").set, bar);
    Foo2.foo = 321;
    assertEq(() => Foo2.bar, 322);
  },
  "Setter decorators: Shim (private instance setter)": () => {
    var _foo_dec, _init, _Foo_instances, foo_set;
    let bar;
    const dec = (fn, ctx) => {
      bar = function(x) {
        fn.call(this, x + 1);
      };
      return bar;
    };
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
        __publicField(this, "bar", 123);
      }
    }
    _Foo_instances = new WeakSet();
    foo_set = function(x) {
      this.bar = x;
    };
    foo_set = __decorateElement(_init, 19, "#foo", _foo_dec, _Foo_instances, foo_set);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_instances, y, foo_set);
    };
    var obj = new Foo2();
    assertEq(() => set$foo(obj, 321), void 0);
    assertEq(() => obj.bar, 322);
  },
  "Setter decorators: Shim (private static setter)": () => {
    var _foo_dec, _init, _Foo_static, foo_set;
    let bar;
    const dec = (fn, ctx) => {
      bar = function(x) {
        fn.call(this, x + 1);
      };
      return bar;
    };
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_set = function(x) {
      this.bar = x;
    };
    foo_set = __decorateElement(_init, 27, "#foo", _foo_dec, _Foo_static, foo_set);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, "bar", 123);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_static, y, foo_set);
    };
    assertEq(() => set$foo(Foo2, 321), void 0);
    assertEq(() => Foo2.bar, 322);
  },
  "Setter decorators: Order (instance setter)": () => {
    var _foo_dec, _init;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function(x) {
        log.push(4);
        fn.call(this, x);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function(x) {
        log.push(5);
        fn.call(this, x);
      };
      return baz;
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      set foo(x) {
        log.push(6);
      }
    }
    __decorateElement(_init, 3, "foo", _foo_dec, Foo2);
    log.push(3);
    new Foo2().foo = 123;
    log.push(7);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2.prototype, "foo").set, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Setter decorators: Order (static setter)": () => {
    var _foo_dec, _init;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function(x) {
        log.push(4);
        fn.call(this, x);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function(x) {
        log.push(5);
        fn.call(this, x);
      };
      return baz;
    };
    log.push(0);
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      static set foo(x) {
        log.push(6);
      }
    }
    __decorateElement(_init, 11, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    log.push(3);
    Foo2.foo = 123;
    log.push(7);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2, "foo").set, bar);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Setter decorators: Order (private instance setter)": () => {
    var _foo_dec, _init, _Foo_instances, foo_set;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function(x) {
        log.push(4);
        fn.call(this, x);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function(x) {
        log.push(5);
        fn.call(this, x);
      };
      return baz;
    };
    log.push(0);
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    }
    _Foo_instances = new WeakSet();
    foo_set = function(x) {
      log.push(6);
    };
    foo_set = __decorateElement(_init, 19, "#foo", _foo_dec, _Foo_instances, foo_set);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_instances, y, foo_set);
    };
    log.push(3);
    assertEq(() => set$foo(new Foo2(), 123), void 0);
    log.push(7);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Setter decorators: Order (private static setter)": () => {
    var _foo_dec, _init, _Foo_static, foo_set;
    const log = [];
    let bar;
    let baz;
    const dec1 = (fn, ctx) => {
      log.push(2);
      bar = function(x) {
        log.push(4);
        fn.call(this, x);
      };
      return bar;
    };
    const dec2 = (fn, ctx) => {
      log.push(1);
      baz = function(x) {
        log.push(5);
        fn.call(this, x);
      };
      return baz;
    };
    log.push(0);
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec1, dec2];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_set = function(x) {
      log.push(6);
    };
    foo_set = __decorateElement(_init, 27, "#foo", _foo_dec, _Foo_static, foo_set);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_static, y, foo_set);
    };
    log.push(3);
    assertEq(() => set$foo(Foo2, 123), void 0);
    log.push(7);
    assertEq(() => log + "", "0,1,2,3,4,5,6,7");
  },
  "Setter decorators: Return null (instance setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
        }
        set foo(x) {
        }
      }
      __decorateElement(_init, 3, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Setter decorators: Return null (static setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        static set foo(x) {
        }
      }
      __decorateElement(_init, 11, "foo", _foo_dec, Foo2);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Setter decorators: Return null (private instance setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_instances, foo_set;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
          __privateAdd(this, _Foo_instances);
        }
      }
      _Foo_instances = new WeakSet();
      foo_set = function(x) {
      };
      foo_set = __decorateElement(_init, 19, "#foo", _foo_dec, _Foo_instances, foo_set);
    }, TypeError);
  },
  "Setter decorators: Return null (private static setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_static, foo_set;
      const dec = (fn, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _Foo_static = new WeakSet();
      foo_set = function(x) {
      };
      foo_set = __decorateElement(_init, 27, "#foo", _foo_dec, _Foo_static, foo_set);
      __privateAdd(Foo2, _Foo_static);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Setter decorators: Return object (instance setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
        }
        set foo(x) {
        }
      }
      __decorateElement(_init, 3, "foo", _foo_dec, Foo2);
    }, TypeError);
  },
  "Setter decorators: Return object (static setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        static set foo(x) {
        }
      }
      __decorateElement(_init, 11, "foo", _foo_dec, Foo2);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Setter decorators: Return object (private instance setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_instances, foo_set;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __runInitializers(_init, 5, this);
          __privateAdd(this, _Foo_instances);
        }
      }
      _Foo_instances = new WeakSet();
      foo_set = function(x) {
      };
      foo_set = __decorateElement(_init, 19, "#foo", _foo_dec, _Foo_instances, foo_set);
    }, TypeError);
  },
  "Setter decorators: Return object (private static setter)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _Foo_static, foo_set;
      const dec = (fn, ctx) => {
        return {};
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _Foo_static = new WeakSet();
      foo_set = function(x) {
      };
      foo_set = __decorateElement(_init, 27, "#foo", _foo_dec, _Foo_static, foo_set);
      __privateAdd(Foo2, _Foo_static);
      __runInitializers(_init, 3, Foo2);
    }, TypeError);
  },
  "Setter decorators: Extra initializer (instance setter)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
      }
      set foo(x) {
      }
    }
    __decorateElement(_init, 3, "foo", _foo_dec, Foo2);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Setter decorators: Extra initializer (static setter)": () => {
    var _foo_dec, _init;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      static set foo(x) {
      }
    }
    __decorateElement(_init, 11, "foo", _foo_dec, Foo2);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  "Setter decorators: Extra initializer (private instance setter)": () => {
    var _foo_dec, _init, _Foo_instances, foo_set;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
      }
    }
    _Foo_instances = new WeakSet();
    foo_set = function(x) {
    };
    foo_set = __decorateElement(_init, 19, "#foo", _foo_dec, _Foo_instances, foo_set);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Setter decorators: Extra initializer (private static setter)": () => {
    var _foo_dec, _init, _Foo_static, foo_set;
    let oldAddInitializer;
    let got;
    const dec = (fn, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    _Foo_static = new WeakSet();
    foo_set = function(x) {
    };
    foo_set = __decorateElement(_init, 27, "#foo", _foo_dec, _Foo_static, foo_set);
    __privateAdd(Foo2, _Foo_static);
    __runInitializers(_init, 3, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  // Auto-accessor decorators
  "Auto-accessor decorators: Basic (instance auto-accessor)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init, _foo, __b, __a;
    const dec = (key, getName, setName) => (target, ctx) => {
      assertEq(() => typeof target.get, "function");
      assertEq(() => typeof target.set, "function");
      assertEq(() => target.get.name, getName);
      assertEq(() => target.set.name, setName);
      assertEq(() => ctx.kind, "accessor");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => {
        const obj2 = {};
        ctx.access.set(obj2, 123);
        return obj2[key];
      }, 123);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    _foo_dec = [dec("foo", "get foo", "set foo")], _b = (_bar_dec = [dec(bar, "get [bar]", "set [bar]")], bar), _a = (_baz_dec = [dec(baz, "get ", "set ")], baz);
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 0)), __runInitializers(_init, 9, this);
        __privateAdd(this, __b, __runInitializers(_init, 10, this, 0)), __runInitializers(_init, 13, this);
        __privateAdd(this, __a, __runInitializers(_init, 14, this, 0)), __runInitializers(_init, 17, this);
      }
    }
    _foo = new WeakMap();
    __b = new WeakMap();
    __a = new WeakMap();
    __decorateElement(_init, 4, "foo", _foo_dec, Foo2, _foo);
    __decorateElement(_init, 4, _b, _bar_dec, Foo2, __b);
    __decorateElement(_init, 4, _a, _baz_dec, Foo2, __a);
    var obj = new Foo2();
    obj.foo = 321;
    assertEq(() => obj.foo, 321);
    obj[bar] = 4321;
    assertEq(() => obj[bar], 4321);
    obj[baz] = 54321;
    assertEq(() => obj[baz], 54321);
  },
  "Auto-accessor decorators: Basic (static auto-accessor)": () => {
    var _baz_dec, _a, _bar_dec, _b, _foo_dec, _init, _foo, __b, __a;
    const dec = (key, getName, setName) => (target, ctx) => {
      assertEq(() => typeof target.get, "function");
      assertEq(() => typeof target.set, "function");
      assertEq(() => target.get.name, getName);
      assertEq(() => target.set.name, setName);
      assertEq(() => ctx.kind, "accessor");
      assertEq(() => ctx.name, key);
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, false);
      assertEq(() => ctx.access.has({ [key]: false }), true);
      assertEq(() => ctx.access.has({ bar: true }), false);
      assertEq(() => ctx.access.get({ [key]: 123 }), 123);
      assertEq(() => {
        const obj = {};
        ctx.access.set(obj, 123);
        return obj[key];
      }, 123);
    };
    const bar = Symbol("bar");
    const baz = Symbol();
    _init = [, , ,];
    _foo_dec = [dec("foo", "get foo", "set foo")], _b = (_bar_dec = [dec(bar, "get [bar]", "set [bar]")], bar), _a = (_baz_dec = [dec(baz, "get ", "set ")], baz);
    class Foo2 {
    }
    _foo = new WeakMap();
    __b = new WeakMap();
    __a = new WeakMap();
    __decorateElement(_init, 12, "foo", _foo_dec, Foo2, _foo);
    __decorateElement(_init, 12, _b, _bar_dec, Foo2, __b);
    __decorateElement(_init, 12, _a, _baz_dec, Foo2, __a);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2, 0)), __runInitializers(_init, 9, Foo2);
    __privateAdd(Foo2, __b, __runInitializers(_init, 10, Foo2, 0)), __runInitializers(_init, 13, Foo2);
    __privateAdd(Foo2, __a, __runInitializers(_init, 14, Foo2, 0)), __runInitializers(_init, 17, Foo2);
    Foo2.foo = 321;
    assertEq(() => Foo2.foo, 321);
    Foo2[bar] = 4321;
    assertEq(() => Foo2[bar], 4321);
    Foo2[baz] = 54321;
    assertEq(() => Foo2[baz], 54321);
  },
  "Auto-accessor decorators: Basic (private instance auto-accessor)": () => {
    var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_instances;
    let lateAsserts;
    const dec = (target, ctx) => {
      assertEq(() => typeof target.get, "function");
      assertEq(() => typeof target.set, "function");
      assertEq(() => target.get.name, "get #foo");
      assertEq(() => target.set.name, "set #foo");
      assertEq(() => ctx.kind, "accessor");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, false);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(new Foo2()), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(new Foo2()), 0);
        assertEq(() => {
          const obj2 = new Foo2();
          ctx.access.set(obj2, 123);
          return get$foo(obj2);
        }, 123);
      };
    };
    let get$foo;
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _Foo_instances);
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 0)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    _Foo_instances = new WeakSet();
    _a = __decorateElement(_init, 20, "#foo", _foo_dec, _Foo_instances, _foo), foo_get = _a.get, foo_set = _a.set;
    get$foo = (x) => __privateGet(x, _Foo_instances, foo_get);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_instances, y, foo_set);
    };
    lateAsserts();
    var obj = new Foo2();
    assertEq(() => set$foo(obj, 321), void 0);
    assertEq(() => get$foo(obj), 321);
  },
  "Auto-accessor decorators: Basic (private static auto-accessor)": () => {
    var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_static;
    let lateAsserts;
    const dec = (target, ctx) => {
      assertEq(() => typeof target.get, "function");
      assertEq(() => typeof target.set, "function");
      assertEq(() => target.get.name, "get #foo");
      assertEq(() => target.set.name, "set #foo");
      assertEq(() => ctx.kind, "accessor");
      assertEq(() => ctx.name, "#foo");
      assertEq(() => ctx.static, true);
      assertEq(() => ctx.private, true);
      lateAsserts = () => {
        assertEq(() => ctx.access.has(Foo2), true);
        assertEq(() => ctx.access.has({}), false);
        assertEq(() => ctx.access.get(Foo2), 0);
        assertEq(() => {
          ctx.access.set(Foo2, 123);
          return get$foo(Foo2);
        }, 123);
      };
    };
    let get$foo;
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
    }
    _foo = new WeakMap();
    _Foo_static = new WeakSet();
    _a = __decorateElement(_init, 28, "#foo", _foo_dec, _Foo_static, _foo), foo_get = _a.get, foo_set = _a.set;
    __privateAdd(Foo2, _Foo_static);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2, 0)), __runInitializers(_init, 9, Foo2);
    get$foo = (x) => __privateGet(x, _Foo_static, foo_get);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_static, y, foo_set);
    };
    lateAsserts();
    assertEq(() => set$foo(Foo2, 321), void 0);
    assertEq(() => get$foo(Foo2), 321);
  },
  "Auto-accessor decorators: Shim (instance auto-accessor)": () => {
    var _foo_dec, _init, _foo;
    let get;
    let set;
    const dec = (target, ctx) => {
      function init(x) {
        assertEq(() => this instanceof Foo2, true);
        return x + 1;
      }
      get = function() {
        return target.get.call(this) * 10;
      };
      set = function(x) {
        target.set.call(this, x * 2);
      };
      return { get, set, init };
    };
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    __decorateElement(_init, 4, "foo", _foo_dec, Foo2, _foo);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2.prototype, "foo").get, get);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2.prototype, "foo").set, set);
    var obj = new Foo2();
    assertEq(() => obj.foo, (123 + 1) * 10);
    obj.foo = 321;
    assertEq(() => obj.foo, 321 * 2 * 10);
  },
  "Auto-accessor decorators: Shim (static auto-accessor)": () => {
    var _foo_dec, _init, _foo;
    let foo;
    let get;
    let set;
    const dec = (target, ctx) => {
      function init(x) {
        assertEq(() => this, foo);
        return x + 1;
      }
      get = function() {
        return target.get.call(this) * 10;
      };
      set = function(x) {
        target.set.call(this, x * 2);
      };
      return { get, set, init };
    };
    _init = [, , ,];
    _foo_dec = [dec];
    const _Foo = class _Foo {
    };
    _foo = new WeakMap();
    __decorateElement(_init, 12, "foo", _foo_dec, _Foo, _foo);
    foo = _Foo;
    __privateAdd(_Foo, _foo, __runInitializers(_init, 6, _Foo, 123)), __runInitializers(_init, 9, _Foo);
    let Foo2 = _Foo;
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2, "foo").get, get);
    assertEq(() => Object.getOwnPropertyDescriptor(Foo2, "foo").set, set);
    assertEq(() => Foo2.foo, (123 + 1) * 10);
    Foo2.foo = 321;
    assertEq(() => Foo2.foo, 321 * 2 * 10);
  },
  "Auto-accessor decorators: Shim (private instance auto-accessor)": () => {
    var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_instances;
    let get;
    let set;
    const dec = (target, ctx) => {
      function init(x) {
        assertEq(() => this instanceof Foo2, true);
        return x + 1;
      }
      get = function() {
        return target.get.call(this) * 10;
      };
      set = function(x) {
        target.set.call(this, x * 2);
      };
      return { get, set, init };
    };
    let get$foo;
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _Foo_instances);
        __privateAdd(this, _foo, __runInitializers(_init, 6, this, 123)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    _Foo_instances = new WeakSet();
    _a = __decorateElement(_init, 20, "#foo", _foo_dec, _Foo_instances, _foo), foo_get = _a.get, foo_set = _a.set;
    get$foo = (x) => __privateGet(x, _Foo_instances, foo_get);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_instances, y, foo_set);
    };
    var obj = new Foo2();
    assertEq(() => get$foo(obj), (123 + 1) * 10);
    assertEq(() => set$foo(obj, 321), void 0);
    assertEq(() => get$foo(obj), 321 * 2 * 10);
  },
  "Auto-accessor decorators: Shim (private static auto-accessor)": () => {
    var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_static;
    let foo;
    let get;
    let set;
    const dec = (target, ctx) => {
      function init(x) {
        assertEq(() => this, foo);
        return x + 1;
      }
      get = function() {
        return target.get.call(this) * 10;
      };
      set = function(x) {
        target.set.call(this, x * 2);
      };
      return { get, set, init };
    };
    let get$foo;
    let set$foo;
    _init = [, , ,];
    _foo_dec = [dec];
    const _Foo = class _Foo {
    };
    _foo = new WeakMap();
    _Foo_static = new WeakSet();
    _a = __decorateElement(_init, 28, "#foo", _foo_dec, _Foo_static, _foo), foo_get = _a.get, foo_set = _a.set;
    __privateAdd(_Foo, _Foo_static);
    foo = _Foo;
    get$foo = (x) => __privateGet(x, _Foo_static, foo_get);
    set$foo = (x, y) => {
      __privateSet(x, _Foo_static, y, foo_set);
    };
    __privateAdd(_Foo, _foo, __runInitializers(_init, 6, _Foo, 123)), __runInitializers(_init, 9, _Foo);
    let Foo2 = _Foo;
    assertEq(() => get$foo(Foo2), (123 + 1) * 10);
    assertEq(() => set$foo(Foo2, 321), void 0);
    assertEq(() => get$foo(Foo2), 321 * 2 * 10);
  },
  "Auto-accessor decorators: Return null (instance auto-accessor)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo;
      const dec = (target, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
        }
      }
      _foo = new WeakMap();
      __decorateElement(_init, 4, "foo", _foo_dec, Foo2, _foo);
    }, TypeError);
  },
  "Auto-accessor decorators: Return null (static auto-accessor)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo;
      const dec = (target, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _foo = new WeakMap();
      __decorateElement(_init, 12, "foo", _foo_dec, Foo2, _foo);
      __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    }, TypeError);
  },
  "Auto-accessor decorators: Return null (private instance auto-accessor)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_instances;
      const dec = (target, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
        constructor() {
          __privateAdd(this, _Foo_instances);
          __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
        }
      }
      _foo = new WeakMap();
      _Foo_instances = new WeakSet();
      _a = __decorateElement(_init, 20, "#foo", _foo_dec, _Foo_instances, _foo), foo_get = _a.get, foo_set = _a.set;
    }, TypeError);
  },
  "Auto-accessor decorators: Return null (private static auto-accessor)": () => {
    assertThrows(() => {
      var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_static;
      const dec = (target, ctx) => {
        return null;
      };
      _init = [, , ,];
      _foo_dec = [dec];
      class Foo2 {
      }
      _foo = new WeakMap();
      _Foo_static = new WeakSet();
      _a = __decorateElement(_init, 28, "#foo", _foo_dec, _Foo_static, _foo), foo_get = _a.get, foo_set = _a.set;
      __privateAdd(Foo2, _Foo_static);
      __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    }, TypeError);
  },
  "Auto-accessor decorators: Extra initializer (instance auto-accessor)": () => {
    var _foo_dec, _init, _foo;
    let oldAddInitializer;
    let got;
    const dec = (target, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    __decorateElement(_init, 4, "foo", _foo_dec, Foo2, _foo);
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Auto-accessor decorators: Extra initializer (static auto-accessor)": () => {
    var _foo_dec, _init, _foo;
    let oldAddInitializer;
    let got;
    const dec = (target, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    _foo = new WeakMap();
    __decorateElement(_init, 12, "foo", _foo_dec, Foo2, _foo);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  "Auto-accessor decorators: Extra initializer (private instance auto-accessor)": () => {
    var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_instances;
    let oldAddInitializer;
    let got;
    const dec = (target, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
      constructor() {
        __privateAdd(this, _Foo_instances);
        __privateAdd(this, _foo, __runInitializers(_init, 6, this)), __runInitializers(_init, 9, this);
      }
    }
    _foo = new WeakMap();
    _Foo_instances = new WeakSet();
    _a = __decorateElement(_init, 20, "#foo", _foo_dec, _Foo_instances, _foo), foo_get = _a.get, foo_set = _a.set;
    assertEq(() => got, void 0);
    const instance = new Foo2();
    assertEq(() => got.this, instance);
    assertEq(() => got.args.length, 0);
  },
  "Auto-accessor decorators: Extra initializer (private static auto-accessor)": () => {
    var _foo_dec, _init, _foo, _a, foo_get, foo_set, _Foo_static;
    let oldAddInitializer;
    let got;
    const dec = (target, ctx) => {
      ctx.addInitializer(function(...args) {
        got = { this: this, args };
      });
      if (oldAddInitializer) assertThrows(() => oldAddInitializer(() => {
      }), TypeError);
      assertThrows(() => ctx.addInitializer({}), TypeError);
      oldAddInitializer = ctx.addInitializer;
    };
    _init = [, , ,];
    _foo_dec = [dec, dec];
    class Foo2 {
    }
    _foo = new WeakMap();
    _Foo_static = new WeakSet();
    _a = __decorateElement(_init, 28, "#foo", _foo_dec, _Foo_static, _foo), foo_get = _a.get, foo_set = _a.set;
    __privateAdd(Foo2, _Foo_static);
    __privateAdd(Foo2, _foo, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    assertEq(() => got.this, Foo2);
    assertEq(() => got.args.length, 0);
  },
  // Decorator list evaluation
  "Decorator list evaluation: Computed names (class statement)": () => {
    var _dec, _a, _dec2, _b, _dec3, _c, _dec4, _d, _dec5, _e, _dec6, _f, _dec7, _g, _dec8, _h, _dec9, _i, _dec10, _j, _Foo_decorators, _init, __b, __a;
    const log = [];
    const foo = (n) => {
      log.push(n);
      return () => {
      };
    };
    const computed = {
      get method() {
        log.push(log.length);
        return Symbol("method");
      },
      get field() {
        log.push(log.length);
        return Symbol("field");
      },
      get getter() {
        log.push(log.length);
        return Symbol("getter");
      },
      get setter() {
        log.push(log.length);
        return Symbol("setter");
      },
      get accessor() {
        log.push(log.length);
        return Symbol("accessor");
      }
    };
    _init = [, , ,];
    _Foo_decorators = [foo(0)];
    class Foo2 extends (foo(1), Object) {
      constructor() {
        super(...arguments);
        __runInitializers(_init, 5, this);
        __publicField(this, _h, __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, __b, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
      }
      [_j = (_dec10 = [foo(2)], computed.method)]() {
      }
      static [_i = (_dec9 = [foo(4)], computed.method)]() {
      }
      get [(_h = (_dec8 = [foo(6)], computed.field), _g = (_dec7 = [foo(8)], computed.field), _f = (_dec6 = [foo(10)], computed.getter))]() {
        return;
      }
      static get [_e = (_dec5 = [foo(12)], computed.getter)]() {
        return;
      }
      set [_d = (_dec4 = [foo(14)], computed.setter)](x) {
      }
      static set [(_c = (_dec3 = [foo(16)], computed.setter), _b = (_dec2 = [foo(18)], computed.accessor), _a = (_dec = [foo(20)], computed.accessor), _c)](x) {
      }
    }
    __b = new WeakMap();
    __a = new WeakMap();
    __decorateElement(_init, 9, _i, _dec9, Foo2);
    __decorateElement(_init, 10, _e, _dec5, Foo2);
    __decorateElement(_init, 11, _c, _dec3, Foo2);
    __decorateElement(_init, 12, _a, _dec, Foo2, __a);
    __decorateElement(_init, 1, _j, _dec10, Foo2);
    __decorateElement(_init, 2, _f, _dec6, Foo2);
    __decorateElement(_init, 3, _d, _dec4, Foo2);
    __decorateElement(_init, 4, _b, _dec2, Foo2, __b);
    __decorateElement(_init, 13, _g, _dec7, Foo2);
    __decorateElement(_init, 5, _h, _dec8, Foo2);
    Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
    __runInitializers(_init, 3, Foo2);
    __publicField(Foo2, _g, __runInitializers(_init, 14, Foo2)), __runInitializers(_init, 17, Foo2);
    __privateAdd(Foo2, __a, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    __runInitializers(_init, 1, Foo2);
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21");
  },
  "Decorator list evaluation: Computed names (class expression)": () => {
    var _dec, _a, _dec2, _b, _dec3, _c, _dec4, _d, _dec5, _e, _dec6, _f, _dec7, _g, _dec8, _h, _dec9, _i, _dec10, _j, _class_decorators, _init, _k, __b, __a;
    const log = [];
    const foo = (n) => {
      log.push(n);
      return () => {
      };
    };
    const computed = {
      get method() {
        log.push(log.length);
        return Symbol("method");
      },
      get field() {
        log.push(log.length);
        return Symbol("field");
      },
      get getter() {
        log.push(log.length);
        return Symbol("getter");
      },
      get setter() {
        log.push(log.length);
        return Symbol("setter");
      },
      get accessor() {
        log.push(log.length);
        return Symbol("accessor");
      }
    };
    _init = [, , ,], _class_decorators = [foo(0)], _k = class extends (foo(1), Object) {
      constructor() {
        super(...arguments);
        __runInitializers(_init, 5, this);
        __publicField(this, _h, __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, __b, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
      }
      [_j = (_dec10 = [foo(2)], computed.method)]() {
      }
      static [_i = (_dec9 = [foo(4)], computed.method)]() {
      }
      get [(_h = (_dec8 = [foo(6)], computed.field), _g = (_dec7 = [foo(8)], computed.field), _f = (_dec6 = [foo(10)], computed.getter))]() {
        return;
      }
      static get [_e = (_dec5 = [foo(12)], computed.getter)]() {
        return;
      }
      set [_d = (_dec4 = [foo(14)], computed.setter)](x) {
      }
      static set [(_c = (_dec3 = [foo(16)], computed.setter), _b = (_dec2 = [foo(18)], computed.accessor), _a = (_dec = [foo(20)], computed.accessor), _c)](x) {
      }
    }, __b = new WeakMap(), __a = new WeakMap(), __decorateElement(_init, 9, _i, _dec9, _k), __decorateElement(_init, 10, _e, _dec5, _k), __decorateElement(_init, 11, _c, _dec3, _k), __decorateElement(_init, 12, _a, _dec, _k, __a), __decorateElement(_init, 1, _j, _dec10, _k), __decorateElement(_init, 2, _f, _dec6, _k), __decorateElement(_init, 3, _d, _dec4, _k), __decorateElement(_init, 4, _b, _dec2, _k, __b), __decorateElement(_init, 13, _g, _dec7, _k), __decorateElement(_init, 5, _h, _dec8, _k), _k = __decorateElement(_init, 0, "", _class_decorators, _k), __runInitializers(_init, 3, _k), __publicField(_k, _g, __runInitializers(_init, 14, _k)), __runInitializers(_init, 17, _k), __privateAdd(_k, __a, __runInitializers(_init, 6, _k)), __runInitializers(_init, 9, _k), __runInitializers(_init, 1, _k), _k;
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21");
  },
  'Decorator list evaluation: "this" (class statement)': () => {
    const log = [];
    const dummy = () => {
    };
    const ctx = {
      foo(n) {
        log.push(n);
      }
    };
    function wrapper() {
      var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _Foo_decorators, _init, _accessor, _accessor2;
      _init = [, , ,];
      _Foo_decorators = [(assertEq(() => this.foo(0), void 0), dummy)];
      class Foo2 extends (_a = (assertEq(() => this.foo(1), void 0), Object), _method_dec2 = [(assertEq(() => this.foo(2), void 0), dummy)], _method_dec = [(assertEq(() => this.foo(3), void 0), dummy)], _field_dec2 = [(assertEq(() => this.foo(4), void 0), dummy)], _field_dec = [(assertEq(() => this.foo(5), void 0), dummy)], _getter_dec2 = [(assertEq(() => this.foo(6), void 0), dummy)], _getter_dec = [(assertEq(() => this.foo(7), void 0), dummy)], _setter_dec2 = [(assertEq(() => this.foo(8), void 0), dummy)], _setter_dec = [(assertEq(() => this.foo(9), void 0), dummy)], _accessor_dec2 = [(assertEq(() => this.foo(10), void 0), dummy)], _accessor_dec = [(assertEq(() => this.foo(11), void 0), dummy)], _a) {
        constructor() {
          super(...arguments);
          __runInitializers(_init, 5, this);
          __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
          __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        }
        method() {
        }
        static method() {
        }
        get getter() {
          return;
        }
        static get getter() {
          return;
        }
        set setter(x) {
        }
        static set setter(x) {
        }
      }
      _accessor = new WeakMap();
      _accessor2 = new WeakMap();
      __decorateElement(_init, 9, "method", _method_dec, Foo2);
      __decorateElement(_init, 10, "getter", _getter_dec, Foo2);
      __decorateElement(_init, 11, "setter", _setter_dec, Foo2);
      __decorateElement(_init, 12, "accessor", _accessor_dec, Foo2, _accessor2);
      __decorateElement(_init, 1, "method", _method_dec2, Foo2);
      __decorateElement(_init, 2, "getter", _getter_dec2, Foo2);
      __decorateElement(_init, 3, "setter", _setter_dec2, Foo2);
      __decorateElement(_init, 4, "accessor", _accessor_dec2, Foo2, _accessor);
      __decorateElement(_init, 13, "field", _field_dec, Foo2);
      __decorateElement(_init, 5, "field", _field_dec2, Foo2);
      Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
      __runInitializers(_init, 3, Foo2);
      __publicField(Foo2, "field", __runInitializers(_init, 14, Foo2)), __runInitializers(_init, 17, Foo2);
      __privateAdd(Foo2, _accessor2, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
      __runInitializers(_init, 1, Foo2);
    }
    wrapper.call(ctx);
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11");
  },
  'Decorator list evaluation: "this" (class expression)': () => {
    const log = [];
    const dummy = () => {
    };
    const ctx = {
      foo(n) {
        log.push(n);
      }
    };
    function wrapper() {
      var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _class_decorators, _init, _b, _accessor, _accessor2;
      _init = [, , ,], _class_decorators = [(assertEq(() => this.foo(0), void 0), dummy)], _b = class extends (_a = (assertEq(() => this.foo(1), void 0), Object), _method_dec2 = [(assertEq(() => this.foo(2), void 0), dummy)], _method_dec = [(assertEq(() => this.foo(3), void 0), dummy)], _field_dec2 = [(assertEq(() => this.foo(4), void 0), dummy)], _field_dec = [(assertEq(() => this.foo(5), void 0), dummy)], _getter_dec2 = [(assertEq(() => this.foo(6), void 0), dummy)], _getter_dec = [(assertEq(() => this.foo(7), void 0), dummy)], _setter_dec2 = [(assertEq(() => this.foo(8), void 0), dummy)], _setter_dec = [(assertEq(() => this.foo(9), void 0), dummy)], _accessor_dec2 = [(assertEq(() => this.foo(10), void 0), dummy)], _accessor_dec = [(assertEq(() => this.foo(11), void 0), dummy)], _a) {
        constructor() {
          super(...arguments);
          __runInitializers(_init, 5, this);
          __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
          __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        }
        method() {
        }
        static method() {
        }
        get getter() {
          return;
        }
        static get getter() {
          return;
        }
        set setter(x) {
        }
        static set setter(x) {
        }
      }, _accessor = new WeakMap(), _accessor2 = new WeakMap(), __decorateElement(_init, 9, "method", _method_dec, _b), __decorateElement(_init, 10, "getter", _getter_dec, _b), __decorateElement(_init, 11, "setter", _setter_dec, _b), __decorateElement(_init, 12, "accessor", _accessor_dec, _b, _accessor2), __decorateElement(_init, 1, "method", _method_dec2, _b), __decorateElement(_init, 2, "getter", _getter_dec2, _b), __decorateElement(_init, 3, "setter", _setter_dec2, _b), __decorateElement(_init, 4, "accessor", _accessor_dec2, _b, _accessor), __decorateElement(_init, 13, "field", _field_dec, _b), __decorateElement(_init, 5, "field", _field_dec2, _b), _b = __decorateElement(_init, 0, "", _class_decorators, _b), __runInitializers(_init, 3, _b), __publicField(_b, "field", __runInitializers(_init, 14, _b)), __runInitializers(_init, 17, _b), __privateAdd(_b, _accessor2, __runInitializers(_init, 6, _b)), __runInitializers(_init, 9, _b), __runInitializers(_init, 1, _b), _b;
    }
    wrapper.call(ctx);
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11");
  },
  'Decorator list evaluation: "await" (class statement)': async () => {
    const log = [];
    const dummy = () => {
    };
    async function wrapper() {
      var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _Foo_decorators, _init, _accessor, _accessor2;
      _init = [, , ,];
      _Foo_decorators = [(log.push(await Promise.resolve(0)), dummy)];
      class Foo2 extends (_a = (log.push(await Promise.resolve(1)), Object), _method_dec2 = [(log.push(await Promise.resolve(2)), dummy)], _method_dec = [(log.push(await Promise.resolve(3)), dummy)], _field_dec2 = [(log.push(await Promise.resolve(4)), dummy)], _field_dec = [(log.push(await Promise.resolve(5)), dummy)], _getter_dec2 = [(log.push(await Promise.resolve(6)), dummy)], _getter_dec = [(log.push(await Promise.resolve(7)), dummy)], _setter_dec2 = [(log.push(await Promise.resolve(8)), dummy)], _setter_dec = [(log.push(await Promise.resolve(9)), dummy)], _accessor_dec2 = [(log.push(await Promise.resolve(10)), dummy)], _accessor_dec = [(log.push(await Promise.resolve(11)), dummy)], _a) {
        constructor() {
          super(...arguments);
          __runInitializers(_init, 5, this);
          __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
          __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        }
        method() {
        }
        static method() {
        }
        get getter() {
          return;
        }
        static get getter() {
          return;
        }
        set setter(x) {
        }
        static set setter(x) {
        }
      }
      _accessor = new WeakMap();
      _accessor2 = new WeakMap();
      __decorateElement(_init, 9, "method", _method_dec, Foo2);
      __decorateElement(_init, 10, "getter", _getter_dec, Foo2);
      __decorateElement(_init, 11, "setter", _setter_dec, Foo2);
      __decorateElement(_init, 12, "accessor", _accessor_dec, Foo2, _accessor2);
      __decorateElement(_init, 1, "method", _method_dec2, Foo2);
      __decorateElement(_init, 2, "getter", _getter_dec2, Foo2);
      __decorateElement(_init, 3, "setter", _setter_dec2, Foo2);
      __decorateElement(_init, 4, "accessor", _accessor_dec2, Foo2, _accessor);
      __decorateElement(_init, 13, "field", _field_dec, Foo2);
      __decorateElement(_init, 5, "field", _field_dec2, Foo2);
      Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
      __runInitializers(_init, 3, Foo2);
      __publicField(Foo2, "field", __runInitializers(_init, 14, Foo2)), __runInitializers(_init, 17, Foo2);
      __privateAdd(Foo2, _accessor2, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
      __runInitializers(_init, 1, Foo2);
    }
    await wrapper();
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11");
  },
  'Decorator list evaluation: "await" (class expression)': async () => {
    const log = [];
    const dummy = () => {
    };
    async function wrapper() {
      var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _class_decorators, _init, _b, _accessor, _accessor2;
      _init = [, , ,], _class_decorators = [(log.push(await Promise.resolve(0)), dummy)], _b = class extends (_a = (log.push(await Promise.resolve(1)), Object), _method_dec2 = [(log.push(await Promise.resolve(2)), dummy)], _method_dec = [(log.push(await Promise.resolve(3)), dummy)], _field_dec2 = [(log.push(await Promise.resolve(4)), dummy)], _field_dec = [(log.push(await Promise.resolve(5)), dummy)], _getter_dec2 = [(log.push(await Promise.resolve(6)), dummy)], _getter_dec = [(log.push(await Promise.resolve(7)), dummy)], _setter_dec2 = [(log.push(await Promise.resolve(8)), dummy)], _setter_dec = [(log.push(await Promise.resolve(9)), dummy)], _accessor_dec2 = [(log.push(await Promise.resolve(10)), dummy)], _accessor_dec = [(log.push(await Promise.resolve(11)), dummy)], _a) {
        constructor() {
          super(...arguments);
          __runInitializers(_init, 5, this);
          __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
          __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        }
        method() {
        }
        static method() {
        }
        get getter() {
          return;
        }
        static get getter() {
          return;
        }
        set setter(x) {
        }
        static set setter(x) {
        }
      }, _accessor = new WeakMap(), _accessor2 = new WeakMap(), __decorateElement(_init, 9, "method", _method_dec, _b), __decorateElement(_init, 10, "getter", _getter_dec, _b), __decorateElement(_init, 11, "setter", _setter_dec, _b), __decorateElement(_init, 12, "accessor", _accessor_dec, _b, _accessor2), __decorateElement(_init, 1, "method", _method_dec2, _b), __decorateElement(_init, 2, "getter", _getter_dec2, _b), __decorateElement(_init, 3, "setter", _setter_dec2, _b), __decorateElement(_init, 4, "accessor", _accessor_dec2, _b, _accessor), __decorateElement(_init, 13, "field", _field_dec, _b), __decorateElement(_init, 5, "field", _field_dec2, _b), _b = __decorateElement(_init, 0, "", _class_decorators, _b), __runInitializers(_init, 3, _b), __publicField(_b, "field", __runInitializers(_init, 14, _b)), __runInitializers(_init, 17, _b), __privateAdd(_b, _accessor2, __runInitializers(_init, 6, _b)), __runInitializers(_init, 9, _b), __runInitializers(_init, 1, _b), _b;
    }
    await wrapper();
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11");
  },
  "Decorator list evaluation: Outer private name (class statement)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _Foo_decorators, _init, _accessor, _accessor2;
    const log = [];
    class Dummy {
      static #foo(n) {
        log.push(n);
        return () => {
        };
      }
      static {
        const dummy = this;
        _init = [, , ,];
        _Foo_decorators = [dummy.#foo(0)];
        class Foo2 extends (_a = (dummy.#foo(1), Object), _method_dec2 = [dummy.#foo(2)], _method_dec = [dummy.#foo(3)], _field_dec2 = [dummy.#foo(4)], _field_dec = [dummy.#foo(5)], _getter_dec2 = [dummy.#foo(6)], _getter_dec = [dummy.#foo(7)], _setter_dec2 = [dummy.#foo(8)], _setter_dec = [dummy.#foo(9)], _accessor_dec2 = [dummy.#foo(10)], _accessor_dec = [dummy.#foo(11)], _a) {
          constructor() {
            super(...arguments);
            __runInitializers(_init, 5, this);
            __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
            __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
          }
          method() {
          }
          static method() {
          }
          get getter() {
            return;
          }
          static get getter() {
            return;
          }
          set setter(x) {
          }
          static set setter(x) {
          }
        }
        _accessor = new WeakMap();
        _accessor2 = new WeakMap();
        __decorateElement(_init, 9, "method", _method_dec, Foo2);
        __decorateElement(_init, 10, "getter", _getter_dec, Foo2);
        __decorateElement(_init, 11, "setter", _setter_dec, Foo2);
        __decorateElement(_init, 12, "accessor", _accessor_dec, Foo2, _accessor2);
        __decorateElement(_init, 1, "method", _method_dec2, Foo2);
        __decorateElement(_init, 2, "getter", _getter_dec2, Foo2);
        __decorateElement(_init, 3, "setter", _setter_dec2, Foo2);
        __decorateElement(_init, 4, "accessor", _accessor_dec2, Foo2, _accessor);
        __decorateElement(_init, 13, "field", _field_dec, Foo2);
        __decorateElement(_init, 5, "field", _field_dec2, Foo2);
        Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
        __runInitializers(_init, 3, Foo2);
        __publicField(Foo2, "field", __runInitializers(_init, 14, Foo2)), __runInitializers(_init, 17, Foo2);
        __privateAdd(Foo2, _accessor2, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
        __runInitializers(_init, 1, Foo2);
      }
    }
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11");
  },
  "Decorator list evaluation: Outer private name (class expression)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _class_decorators, _init, _b, _accessor, _accessor2;
    const log = [];
    class Dummy {
      static #foo(n) {
        log.push(n);
        return () => {
        };
      }
      static {
        const dummy = this;
        _init = [, , ,], _class_decorators = [dummy.#foo(0)], _b = class extends (_a = (dummy.#foo(1), Object), _method_dec2 = [dummy.#foo(2)], _method_dec = [dummy.#foo(3)], _field_dec2 = [dummy.#foo(4)], _field_dec = [dummy.#foo(5)], _getter_dec2 = [dummy.#foo(6)], _getter_dec = [dummy.#foo(7)], _setter_dec2 = [dummy.#foo(8)], _setter_dec = [dummy.#foo(9)], _accessor_dec2 = [dummy.#foo(10)], _accessor_dec = [dummy.#foo(11)], _a) {
          constructor() {
            super(...arguments);
            __runInitializers(_init, 5, this);
            __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
            __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
          }
          method() {
          }
          static method() {
          }
          get getter() {
            return;
          }
          static get getter() {
            return;
          }
          set setter(x) {
          }
          static set setter(x) {
          }
        }, _accessor = new WeakMap(), _accessor2 = new WeakMap(), __decorateElement(_init, 9, "method", _method_dec, _b), __decorateElement(_init, 10, "getter", _getter_dec, _b), __decorateElement(_init, 11, "setter", _setter_dec, _b), __decorateElement(_init, 12, "accessor", _accessor_dec, _b, _accessor2), __decorateElement(_init, 1, "method", _method_dec2, _b), __decorateElement(_init, 2, "getter", _getter_dec2, _b), __decorateElement(_init, 3, "setter", _setter_dec2, _b), __decorateElement(_init, 4, "accessor", _accessor_dec2, _b, _accessor), __decorateElement(_init, 13, "field", _field_dec, _b), __decorateElement(_init, 5, "field", _field_dec2, _b), _b = __decorateElement(_init, 0, "", _class_decorators, _b), __runInitializers(_init, 3, _b), __publicField(_b, "field", __runInitializers(_init, 14, _b)), __runInitializers(_init, 17, _b), __privateAdd(_b, _accessor2, __runInitializers(_init, 6, _b)), __runInitializers(_init, 9, _b), __runInitializers(_init, 1, _b), _b;
      }
    }
    assertEq(() => "" + log, "0,1,2,3,4,5,6,7,8,9,10,11");
  },
  "Decorator list evaluation: Inner private name (class statement)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _Foo_decorators, _foo, _init, _accessor, _accessor2;
    const fns = [];
    const capture = (fn) => {
      fns.push(fn);
      return () => {
      };
    };
    class Dummy {
      static #foo = NaN;
      static {
        _init = [, , ,];
        _Foo_decorators = [capture(() => new Foo2().#foo + 0)], _method_dec2 = [capture(() => __privateGet(new _Foo(), _foo) + 1)], _method_dec = [capture(() => __privateGet(new _Foo(), _foo) + 2)], _field_dec2 = [capture(() => __privateGet(new _Foo(), _foo) + 3)], _field_dec = [capture(() => __privateGet(new _Foo(), _foo) + 4)], _getter_dec2 = [capture(() => __privateGet(new _Foo(), _foo) + 5)], _getter_dec = [capture(() => __privateGet(new _Foo(), _foo) + 6)], _setter_dec2 = [capture(() => __privateGet(new _Foo(), _foo) + 7)], _setter_dec = [capture(() => __privateGet(new _Foo(), _foo) + 8)], _accessor_dec2 = [capture(() => __privateGet(new _Foo(), _foo) + 9)], _accessor_dec = [capture(() => __privateGet(new _Foo(), _foo) + 10)];
        let _Foo = class _Foo {
          constructor() {
            __runInitializers(_init, 5, this);
            __privateAdd(this, _foo, 10);
            __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
            __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
          }
          method() {
          }
          static method() {
          }
          get getter() {
            return;
          }
          static get getter() {
            return;
          }
          set setter(x) {
          }
          static set setter(x) {
          }
        };
        _foo = new WeakMap();
        _accessor = new WeakMap();
        _accessor2 = new WeakMap();
        __decorateElement(_init, 9, "method", _method_dec, _Foo);
        __decorateElement(_init, 10, "getter", _getter_dec, _Foo);
        __decorateElement(_init, 11, "setter", _setter_dec, _Foo);
        __decorateElement(_init, 12, "accessor", _accessor_dec, _Foo, _accessor2);
        __decorateElement(_init, 1, "method", _method_dec2, _Foo);
        __decorateElement(_init, 2, "getter", _getter_dec2, _Foo);
        __decorateElement(_init, 3, "setter", _setter_dec2, _Foo);
        __decorateElement(_init, 4, "accessor", _accessor_dec2, _Foo, _accessor);
        __decorateElement(_init, 13, "field", _field_dec, _Foo);
        __decorateElement(_init, 5, "field", _field_dec2, _Foo);
        _Foo = __decorateElement(_init, 0, "Foo", _Foo_decorators, _Foo);
        __runInitializers(_init, 3, _Foo);
        __publicField(_Foo, "field", __runInitializers(_init, 14, _Foo)), __runInitializers(_init, 17, _Foo);
        __privateAdd(_Foo, _accessor2, __runInitializers(_init, 6, _Foo)), __runInitializers(_init, 9, _Foo);
        __runInitializers(_init, 1, _Foo);
        let Foo2 = _Foo;
      }
    }
    const firstFn = fns.shift();
    assertEq(() => {
      try {
        firstFn();
        throw new Error("Expected a TypeError to be thrown");
      } catch (err) {
        if (err instanceof TypeError) return true;
        throw err;
      }
    }, true);
    const log = [];
    for (const fn of fns) log.push(fn());
    assertEq(() => "" + log, "11,12,13,14,15,16,17,18,19,20");
  },
  "Decorator list evaluation: Inner private name (class expression)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _Foo_decorators, _foo, _init, _a, _accessor, _accessor2;
    const fns = [];
    const capture = (fn) => {
      fns.push(fn);
      return () => {
      };
    };
    class Outer {
      static #foo = 0;
      static {
        _init = [, , ,], _Foo_decorators = [capture(() => Outer.#foo + 0)], _method_dec2 = [capture(() => __privateGet(new _a(), _foo) + 1)], _method_dec = [capture(() => __privateGet(new _a(), _foo) + 2)], _field_dec2 = [capture(() => __privateGet(new _a(), _foo) + 3)], _field_dec = [capture(() => __privateGet(new _a(), _foo) + 4)], _getter_dec2 = [capture(() => __privateGet(new _a(), _foo) + 5)], _getter_dec = [capture(() => __privateGet(new _a(), _foo) + 6)], _setter_dec2 = [capture(() => __privateGet(new _a(), _foo) + 7)], _setter_dec = [capture(() => __privateGet(new _a(), _foo) + 8)], _accessor_dec2 = [capture(() => __privateGet(new _a(), _foo) + 9)], _accessor_dec = [capture(() => __privateGet(new _a(), _foo) + 10)], _a = class {
          constructor() {
            __runInitializers(_init, 5, this);
            __privateAdd(this, _foo, 10);
            __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
            __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
          }
          method() {
          }
          static method() {
          }
          get getter() {
            return;
          }
          static get getter() {
            return;
          }
          set setter(x) {
          }
          static set setter(x) {
          }
        }, _foo = new WeakMap(), _accessor = new WeakMap(), _accessor2 = new WeakMap(), __decorateElement(_init, 9, "method", _method_dec, _a), __decorateElement(_init, 10, "getter", _getter_dec, _a), __decorateElement(_init, 11, "setter", _setter_dec, _a), __decorateElement(_init, 12, "accessor", _accessor_dec, _a, _accessor2), __decorateElement(_init, 1, "method", _method_dec2, _a), __decorateElement(_init, 2, "getter", _getter_dec2, _a), __decorateElement(_init, 3, "setter", _setter_dec2, _a), __decorateElement(_init, 4, "accessor", _accessor_dec2, _a, _accessor), __decorateElement(_init, 13, "field", _field_dec, _a), __decorateElement(_init, 5, "field", _field_dec2, _a), _a = __decorateElement(_init, 0, "Foo", _Foo_decorators, _a), __runInitializers(_init, 3, _a), __publicField(_a, "field", __runInitializers(_init, 14, _a)), __runInitializers(_init, 17, _a), __privateAdd(_a, _accessor2, __runInitializers(_init, 6, _a)), __runInitializers(_init, 9, _a), __runInitializers(_init, 1, _a), _a;
      }
    }
    const log = [];
    for (const fn of fns) log.push(fn());
    assertEq(() => "" + log, "0,11,12,13,14,15,16,17,18,19,20");
  },
  "Decorator list evaluation: Class binding (class statement)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _Foo_decorators, _init, _accessor, _accessor2;
    const fns = [];
    const capture = (fn) => {
      fns.push(fn);
      assertThrows(() => fn(), ReferenceError);
      return () => {
      };
    };
    _init = [, , ,];
    _Foo_decorators = [capture(() => Foo2)], _method_dec2 = [capture(() => _Foo)], _method_dec = [capture(() => _Foo)], _field_dec2 = [capture(() => _Foo)], _field_dec = [capture(() => _Foo)], _getter_dec2 = [capture(() => _Foo)], _getter_dec = [capture(() => _Foo)], _setter_dec2 = [capture(() => _Foo)], _setter_dec = [capture(() => _Foo)], _accessor_dec2 = [capture(() => _Foo)], _accessor_dec = [capture(() => _Foo)];
    let _Foo = class _Foo {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
      }
      method() {
      }
      static method() {
      }
      get getter() {
        return;
      }
      static get getter() {
        return;
      }
      set setter(x) {
      }
      static set setter(x) {
      }
    };
    _accessor = new WeakMap();
    _accessor2 = new WeakMap();
    __decorateElement(_init, 9, "method", _method_dec, _Foo);
    __decorateElement(_init, 10, "getter", _getter_dec, _Foo);
    __decorateElement(_init, 11, "setter", _setter_dec, _Foo);
    __decorateElement(_init, 12, "accessor", _accessor_dec, _Foo, _accessor2);
    __decorateElement(_init, 1, "method", _method_dec2, _Foo);
    __decorateElement(_init, 2, "getter", _getter_dec2, _Foo);
    __decorateElement(_init, 3, "setter", _setter_dec2, _Foo);
    __decorateElement(_init, 4, "accessor", _accessor_dec2, _Foo, _accessor);
    __decorateElement(_init, 13, "field", _field_dec, _Foo);
    __decorateElement(_init, 5, "field", _field_dec2, _Foo);
    _Foo = __decorateElement(_init, 0, "Foo", _Foo_decorators, _Foo);
    __runInitializers(_init, 3, _Foo);
    __publicField(_Foo, "field", __runInitializers(_init, 14, _Foo)), __runInitializers(_init, 17, _Foo);
    __privateAdd(_Foo, _accessor2, __runInitializers(_init, 6, _Foo)), __runInitializers(_init, 9, _Foo);
    __runInitializers(_init, 1, _Foo);
    let Foo2 = _Foo;
    const originalFoo = Foo2;
    for (const fn of fns) {
      assertEq(() => fn(), originalFoo);
    }
    Foo2 = null;
    const firstFn = fns.shift();
    assertEq(() => firstFn(), null);
    for (const fn of fns) {
      assertEq(() => fn(), originalFoo);
    }
  },
  "Decorator list evaluation: Class binding (class expression)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _originalFoo_decorators, _init, _a, _accessor, _accessor2;
    const fns = [];
    const capture = (fn) => {
      fns.push(fn);
      return () => {
      };
    };
    const originalFoo = (_init = [, , ,], _originalFoo_decorators = [capture(() => Foo)], _method_dec2 = [capture(() => _a)], _method_dec = [capture(() => _a)], _field_dec2 = [capture(() => _a)], _field_dec = [capture(() => _a)], _getter_dec2 = [capture(() => _a)], _getter_dec = [capture(() => _a)], _setter_dec2 = [capture(() => _a)], _setter_dec = [capture(() => _a)], _accessor_dec2 = [capture(() => _a)], _accessor_dec = [capture(() => _a)], _a = class {
      constructor() {
        __runInitializers(_init, 5, this);
        __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
      }
      method() {
      }
      static method() {
      }
      get getter() {
        return;
      }
      static get getter() {
        return;
      }
      set setter(x) {
      }
      static set setter(x) {
      }
    }, _accessor = new WeakMap(), _accessor2 = new WeakMap(), __decorateElement(_init, 9, "method", _method_dec, _a), __decorateElement(_init, 10, "getter", _getter_dec, _a), __decorateElement(_init, 11, "setter", _setter_dec, _a), __decorateElement(_init, 12, "accessor", _accessor_dec, _a, _accessor2), __decorateElement(_init, 1, "method", _method_dec2, _a), __decorateElement(_init, 2, "getter", _getter_dec2, _a), __decorateElement(_init, 3, "setter", _setter_dec2, _a), __decorateElement(_init, 4, "accessor", _accessor_dec2, _a, _accessor), __decorateElement(_init, 13, "field", _field_dec, _a), __decorateElement(_init, 5, "field", _field_dec2, _a), _a = __decorateElement(_init, 0, "originalFoo", _originalFoo_decorators, _a), __runInitializers(_init, 3, _a), __publicField(_a, "field", __runInitializers(_init, 14, _a)), __runInitializers(_init, 17, _a), __privateAdd(_a, _accessor2, __runInitializers(_init, 6, _a)), __runInitializers(_init, 9, _a), __runInitializers(_init, 1, _a), _a);
    const firstFn = fns.shift();
    assertThrows(() => firstFn(), ReferenceError);
    for (const fn of fns) {
      assertEq(() => fn(), originalFoo);
    }
  },
  // Initializer order
  "Initializer order (public members, class statement)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _Foo_decorators, _init, _accessor, _accessor2;
    const log = [];
    const classDec1 = (cls, ctxClass) => {
      log.push("c2");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c5"));
      ctxClass.addInitializer(() => log.push("c6"));
    };
    const classDec2 = (cls, ctxClass) => {
      log.push("c1");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c3"));
      ctxClass.addInitializer(() => log.push("c4"));
    };
    const methodDec1 = (fn, ctxMethod) => {
      log.push("m2");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m5"));
      ctxMethod.addInitializer(() => log.push("m6"));
    };
    const methodDec2 = (fn, ctxMethod) => {
      log.push("m1");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m3"));
      ctxMethod.addInitializer(() => log.push("m4"));
    };
    const staticMethodDec1 = (fn, ctxStaticMethod) => {
      log.push("M2");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M5"));
      ctxStaticMethod.addInitializer(() => log.push("M6"));
    };
    const staticMethodDec2 = (fn, ctxStaticMethod) => {
      log.push("M1");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M3"));
      ctxStaticMethod.addInitializer(() => log.push("M4"));
    };
    const fieldDec1 = (value, ctxField) => {
      log.push("f2");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f5"));
      ctxField.addInitializer(() => log.push("f6"));
      return () => {
        log.push("f7");
      };
    };
    const fieldDec2 = (value, ctxField) => {
      log.push("f1");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f3"));
      ctxField.addInitializer(() => log.push("f4"));
      return () => {
        log.push("f8");
      };
    };
    const staticFieldDec1 = (value, ctxStaticField) => {
      log.push("F2");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F5"));
      ctxStaticField.addInitializer(() => log.push("F6"));
      return () => {
        log.push("F7");
      };
    };
    const staticFieldDec2 = (value, ctxStaticField) => {
      log.push("F1");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F3"));
      ctxStaticField.addInitializer(() => log.push("F4"));
      return () => {
        log.push("F8");
      };
    };
    const getterDec1 = (fn, ctxGetter) => {
      log.push("g2");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g5"));
      ctxGetter.addInitializer(() => log.push("g6"));
    };
    const getterDec2 = (fn, ctxGetter) => {
      log.push("g1");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g3"));
      ctxGetter.addInitializer(() => log.push("g4"));
    };
    const staticGetterDec1 = (fn, ctxStaticGetter) => {
      log.push("G2");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G5"));
      ctxStaticGetter.addInitializer(() => log.push("G6"));
    };
    const staticGetterDec2 = (fn, ctxStaticGetter) => {
      log.push("G1");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G3"));
      ctxStaticGetter.addInitializer(() => log.push("G4"));
    };
    const setterDec1 = (fn, ctxSetter) => {
      log.push("s2");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s5"));
      ctxSetter.addInitializer(() => log.push("s6"));
    };
    const setterDec2 = (fn, ctxSetter) => {
      log.push("s1");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s3"));
      ctxSetter.addInitializer(() => log.push("s4"));
    };
    const staticSetterDec1 = (fn, ctxStaticSetter) => {
      log.push("S2");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S5"));
      ctxStaticSetter.addInitializer(() => log.push("S6"));
    };
    const staticSetterDec2 = (fn, ctxStaticSetter) => {
      log.push("S1");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S3"));
      ctxStaticSetter.addInitializer(() => log.push("S4"));
    };
    const accessorDec1 = (target, ctxAccessor) => {
      log.push("a2");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a5"));
      ctxAccessor.addInitializer(() => log.push("a6"));
      return { init() {
        log.push("a7");
      } };
    };
    const accessorDec2 = (target, ctxAccessor) => {
      log.push("a1");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a3"));
      ctxAccessor.addInitializer(() => log.push("a4"));
      return { init() {
        log.push("a8");
      } };
    };
    const staticAccessorDec1 = (target, ctxStaticAccessor) => {
      log.push("A2");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A5"));
      ctxStaticAccessor.addInitializer(() => log.push("A6"));
      return { init() {
        log.push("A7");
      } };
    };
    const staticAccessorDec2 = (target, ctxStaticAccessor) => {
      log.push("A1");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A3"));
      ctxStaticAccessor.addInitializer(() => log.push("A4"));
      return { init() {
        log.push("A8");
      } };
    };
    log.push("start");
    _init = [, , ,];
    _Foo_decorators = [classDec1, classDec2];
    class Foo2 extends (_a = (log.push("extends"), Object), _method_dec2 = [methodDec1, methodDec2], _method_dec = [staticMethodDec1, staticMethodDec2], _field_dec2 = [fieldDec1, fieldDec2], _field_dec = [staticFieldDec1, staticFieldDec2], _getter_dec2 = [getterDec1, getterDec2], _getter_dec = [staticGetterDec1, staticGetterDec2], _setter_dec2 = [setterDec1, setterDec2], _setter_dec = [staticSetterDec1, staticSetterDec2], _accessor_dec2 = [accessorDec1, accessorDec2], _accessor_dec = [staticAccessorDec1, staticAccessorDec2], _a) {
      constructor() {
        log.push("ctor:start");
        super();
        __runInitializers(_init, 5, this);
        __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        log.push("ctor:end");
      }
      method() {
      }
      static method() {
      }
      get getter() {
        return;
      }
      static get getter() {
        return;
      }
      set setter(x) {
      }
      static set setter(x) {
      }
    }
    _accessor = new WeakMap();
    _accessor2 = new WeakMap();
    __decorateElement(_init, 9, "method", _method_dec, Foo2);
    __decorateElement(_init, 10, "getter", _getter_dec, Foo2);
    __decorateElement(_init, 11, "setter", _setter_dec, Foo2);
    __decorateElement(_init, 12, "accessor", _accessor_dec, Foo2, _accessor2);
    __decorateElement(_init, 1, "method", _method_dec2, Foo2);
    __decorateElement(_init, 2, "getter", _getter_dec2, Foo2);
    __decorateElement(_init, 3, "setter", _setter_dec2, Foo2);
    __decorateElement(_init, 4, "accessor", _accessor_dec2, Foo2, _accessor);
    __decorateElement(_init, 13, "field", _field_dec, Foo2);
    __decorateElement(_init, 5, "field", _field_dec2, Foo2);
    Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
    __runInitializers(_init, 3, Foo2);
    log.push("static:start");
    __publicField(Foo2, "field", __runInitializers(_init, 14, Foo2)), __runInitializers(_init, 17, Foo2);
    __privateAdd(Foo2, _accessor2, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    log.push("static:end");
    __runInitializers(_init, 1, Foo2);
    log.push("after");
    new Foo2();
    log.push("end");
    assertEq(() => log + "", "start,extends,M1,M2,G1,G2,S1,S2,A1,A2,m1,m2,g1,g2,s1,s2,a1,a2,F1,F2,f1,f2,c1,c2,M3,M4,M5,M6,G3,G4,G5,G6,S3,S4,S5,S6,static:start,F7,F8,F3,F4,F5,F6,A7,A8,A3,A4,A5,A6,static:end,c3,c4,c5,c6,after,ctor:start,m3,m4,m5,m6,g3,g4,g5,g6,s3,s4,s5,s6,f7,f8,f3,f4,f5,f6,a7,a8,a3,a4,a5,a6,ctor:end,end");
  },
  "Initializer order (public members, class expression)": () => {
    var _accessor_dec, _accessor_dec2, _setter_dec, _setter_dec2, _getter_dec, _getter_dec2, _field_dec, _field_dec2, _method_dec, _method_dec2, _a, _Foo_decorators, _init, _b, _accessor, _accessor2;
    const log = [];
    const classDec1 = (cls, ctxClass) => {
      log.push("c2");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c5"));
      ctxClass.addInitializer(() => log.push("c6"));
    };
    const classDec2 = (cls, ctxClass) => {
      log.push("c1");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c3"));
      ctxClass.addInitializer(() => log.push("c4"));
    };
    const methodDec1 = (fn, ctxMethod) => {
      log.push("m2");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m5"));
      ctxMethod.addInitializer(() => log.push("m6"));
    };
    const methodDec2 = (fn, ctxMethod) => {
      log.push("m1");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m3"));
      ctxMethod.addInitializer(() => log.push("m4"));
    };
    const staticMethodDec1 = (fn, ctxStaticMethod) => {
      log.push("M2");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M5"));
      ctxStaticMethod.addInitializer(() => log.push("M6"));
    };
    const staticMethodDec2 = (fn, ctxStaticMethod) => {
      log.push("M1");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M3"));
      ctxStaticMethod.addInitializer(() => log.push("M4"));
    };
    const fieldDec1 = (value, ctxField) => {
      log.push("f2");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f5"));
      ctxField.addInitializer(() => log.push("f6"));
      return () => {
        log.push("f7");
      };
    };
    const fieldDec2 = (value, ctxField) => {
      log.push("f1");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f3"));
      ctxField.addInitializer(() => log.push("f4"));
      return () => {
        log.push("f8");
      };
    };
    const staticFieldDec1 = (value, ctxStaticField) => {
      log.push("F2");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F5"));
      ctxStaticField.addInitializer(() => log.push("F6"));
      return () => {
        log.push("F7");
      };
    };
    const staticFieldDec2 = (value, ctxStaticField) => {
      log.push("F1");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F3"));
      ctxStaticField.addInitializer(() => log.push("F4"));
      return () => {
        log.push("F8");
      };
    };
    const getterDec1 = (fn, ctxGetter) => {
      log.push("g2");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g5"));
      ctxGetter.addInitializer(() => log.push("g6"));
    };
    const getterDec2 = (fn, ctxGetter) => {
      log.push("g1");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g3"));
      ctxGetter.addInitializer(() => log.push("g4"));
    };
    const staticGetterDec1 = (fn, ctxStaticGetter) => {
      log.push("G2");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G5"));
      ctxStaticGetter.addInitializer(() => log.push("G6"));
    };
    const staticGetterDec2 = (fn, ctxStaticGetter) => {
      log.push("G1");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G3"));
      ctxStaticGetter.addInitializer(() => log.push("G4"));
    };
    const setterDec1 = (fn, ctxSetter) => {
      log.push("s2");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s5"));
      ctxSetter.addInitializer(() => log.push("s6"));
    };
    const setterDec2 = (fn, ctxSetter) => {
      log.push("s1");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s3"));
      ctxSetter.addInitializer(() => log.push("s4"));
    };
    const staticSetterDec1 = (fn, ctxStaticSetter) => {
      log.push("S2");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S5"));
      ctxStaticSetter.addInitializer(() => log.push("S6"));
    };
    const staticSetterDec2 = (fn, ctxStaticSetter) => {
      log.push("S1");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S3"));
      ctxStaticSetter.addInitializer(() => log.push("S4"));
    };
    const accessorDec1 = (target, ctxAccessor) => {
      log.push("a2");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a5"));
      ctxAccessor.addInitializer(() => log.push("a6"));
      return { init() {
        log.push("a7");
      } };
    };
    const accessorDec2 = (target, ctxAccessor) => {
      log.push("a1");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a3"));
      ctxAccessor.addInitializer(() => log.push("a4"));
      return { init() {
        log.push("a8");
      } };
    };
    const staticAccessorDec1 = (target, ctxStaticAccessor) => {
      log.push("A2");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A5"));
      ctxStaticAccessor.addInitializer(() => log.push("A6"));
      return { init() {
        log.push("A7");
      } };
    };
    const staticAccessorDec2 = (target, ctxStaticAccessor) => {
      log.push("A1");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A3"));
      ctxStaticAccessor.addInitializer(() => log.push("A4"));
      return { init() {
        log.push("A8");
      } };
    };
    log.push("start");
    const Foo2 = (_init = [, , ,], _Foo_decorators = [classDec1, classDec2], _b = class extends (_a = (log.push("extends"), Object), _method_dec2 = [methodDec1, methodDec2], _method_dec = [staticMethodDec1, staticMethodDec2], _field_dec2 = [fieldDec1, fieldDec2], _field_dec = [staticFieldDec1, staticFieldDec2], _getter_dec2 = [getterDec1, getterDec2], _getter_dec = [staticGetterDec1, staticGetterDec2], _setter_dec2 = [setterDec1, setterDec2], _setter_dec = [staticSetterDec1, staticSetterDec2], _accessor_dec2 = [accessorDec1, accessorDec2], _accessor_dec = [staticAccessorDec1, staticAccessorDec2], _a) {
      constructor() {
        log.push("ctor:start");
        super();
        __runInitializers(_init, 5, this);
        __publicField(this, "field", __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        log.push("ctor:end");
      }
      method() {
      }
      static method() {
      }
      get getter() {
        return;
      }
      static get getter() {
        return;
      }
      set setter(x) {
      }
      static set setter(x) {
      }
    }, _accessor = new WeakMap(), _accessor2 = new WeakMap(), __decorateElement(_init, 9, "method", _method_dec, _b), __decorateElement(_init, 10, "getter", _getter_dec, _b), __decorateElement(_init, 11, "setter", _setter_dec, _b), __decorateElement(_init, 12, "accessor", _accessor_dec, _b, _accessor2), __decorateElement(_init, 1, "method", _method_dec2, _b), __decorateElement(_init, 2, "getter", _getter_dec2, _b), __decorateElement(_init, 3, "setter", _setter_dec2, _b), __decorateElement(_init, 4, "accessor", _accessor_dec2, _b, _accessor), __decorateElement(_init, 13, "field", _field_dec, _b), __decorateElement(_init, 5, "field", _field_dec2, _b), _b = __decorateElement(_init, 0, "Foo", _Foo_decorators, _b), __runInitializers(_init, 3, _b), log.push("static:start"), __publicField(_b, "field", __runInitializers(_init, 14, _b)), __runInitializers(_init, 17, _b), __privateAdd(_b, _accessor2, __runInitializers(_init, 6, _b)), __runInitializers(_init, 9, _b), log.push("static:end"), __runInitializers(_init, 1, _b), _b);
    log.push("after");
    new Foo2();
    log.push("end");
    assertEq(() => log + "", "start,extends,M1,M2,G1,G2,S1,S2,A1,A2,m1,m2,g1,g2,s1,s2,a1,a2,F1,F2,f1,f2,c1,c2,M3,M4,M5,M6,G3,G4,G5,G6,S3,S4,S5,S6,static:start,F7,F8,F3,F4,F5,F6,A7,A8,A3,A4,A5,A6,static:end,c3,c4,c5,c6,after,ctor:start,m3,m4,m5,m6,g3,g4,g5,g6,s3,s4,s5,s6,f7,f8,f3,f4,f5,f6,a7,a8,a3,a4,a5,a6,ctor:end,end");
  },
  "Initializer order (private members, class statement)": () => {
    var _staticAccessor_dec, _accessor_dec, _staticSetter_dec, _setter_dec, _staticGetter_dec, _getter_dec, _staticField_dec, _field_dec, _staticMethod_dec, _method_dec, _a, _Foo_decorators, _init, _Foo_instances, method_fn, _Foo_static, staticMethod_fn, _field, _staticField, getter_get, staticGetter_get, setter_set, staticSetter_set, _accessor, _b, accessor_get, accessor_set, _staticAccessor, _c, staticAccessor_get, staticAccessor_set;
    const log = [];
    const classDec1 = (cls, ctxClass) => {
      log.push("c2");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c5"));
      ctxClass.addInitializer(() => log.push("c6"));
    };
    const classDec2 = (cls, ctxClass) => {
      log.push("c1");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c3"));
      ctxClass.addInitializer(() => log.push("c4"));
    };
    const methodDec1 = (fn, ctxMethod) => {
      log.push("m2");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m5"));
      ctxMethod.addInitializer(() => log.push("m6"));
    };
    const methodDec2 = (fn, ctxMethod) => {
      log.push("m1");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m3"));
      ctxMethod.addInitializer(() => log.push("m4"));
    };
    const staticMethodDec1 = (fn, ctxStaticMethod) => {
      log.push("M2");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M5"));
      ctxStaticMethod.addInitializer(() => log.push("M6"));
    };
    const staticMethodDec2 = (fn, ctxStaticMethod) => {
      log.push("M1");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M3"));
      ctxStaticMethod.addInitializer(() => log.push("M4"));
    };
    const fieldDec1 = (value, ctxField) => {
      log.push("f2");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f5"));
      ctxField.addInitializer(() => log.push("f6"));
      return () => {
        log.push("f7");
      };
    };
    const fieldDec2 = (value, ctxField) => {
      log.push("f1");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f3"));
      ctxField.addInitializer(() => log.push("f4"));
      return () => {
        log.push("f8");
      };
    };
    const staticFieldDec1 = (value, ctxStaticField) => {
      log.push("F2");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F5"));
      ctxStaticField.addInitializer(() => log.push("F6"));
      return () => {
        log.push("F7");
      };
    };
    const staticFieldDec2 = (value, ctxStaticField) => {
      log.push("F1");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F3"));
      ctxStaticField.addInitializer(() => log.push("F4"));
      return () => {
        log.push("F8");
      };
    };
    const getterDec1 = (fn, ctxGetter) => {
      log.push("g2");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g5"));
      ctxGetter.addInitializer(() => log.push("g6"));
    };
    const getterDec2 = (fn, ctxGetter) => {
      log.push("g1");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g3"));
      ctxGetter.addInitializer(() => log.push("g4"));
    };
    const staticGetterDec1 = (fn, ctxStaticGetter) => {
      log.push("G2");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G5"));
      ctxStaticGetter.addInitializer(() => log.push("G6"));
    };
    const staticGetterDec2 = (fn, ctxStaticGetter) => {
      log.push("G1");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G3"));
      ctxStaticGetter.addInitializer(() => log.push("G4"));
    };
    const setterDec1 = (fn, ctxSetter) => {
      log.push("s2");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s5"));
      ctxSetter.addInitializer(() => log.push("s6"));
    };
    const setterDec2 = (fn, ctxSetter) => {
      log.push("s1");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s3"));
      ctxSetter.addInitializer(() => log.push("s4"));
    };
    const staticSetterDec1 = (fn, ctxStaticSetter) => {
      log.push("S2");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S5"));
      ctxStaticSetter.addInitializer(() => log.push("S6"));
    };
    const staticSetterDec2 = (fn, ctxStaticSetter) => {
      log.push("S1");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S3"));
      ctxStaticSetter.addInitializer(() => log.push("S4"));
    };
    const accessorDec1 = (target, ctxAccessor) => {
      log.push("a2");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a5"));
      ctxAccessor.addInitializer(() => log.push("a6"));
      return { init() {
        log.push("a7");
      } };
    };
    const accessorDec2 = (target, ctxAccessor) => {
      log.push("a1");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a3"));
      ctxAccessor.addInitializer(() => log.push("a4"));
      return { init() {
        log.push("a8");
      } };
    };
    const staticAccessorDec1 = (target, ctxStaticAccessor) => {
      log.push("A2");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A5"));
      ctxStaticAccessor.addInitializer(() => log.push("A6"));
      return { init() {
        log.push("A7");
      } };
    };
    const staticAccessorDec2 = (target, ctxStaticAccessor) => {
      log.push("A1");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A3"));
      ctxStaticAccessor.addInitializer(() => log.push("A4"));
      return { init() {
        log.push("A8");
      } };
    };
    log.push("start");
    _init = [, , ,];
    _Foo_decorators = [classDec1, classDec2];
    class Foo2 extends (_a = (log.push("extends"), Object), _method_dec = [methodDec1, methodDec2], _staticMethod_dec = [staticMethodDec1, staticMethodDec2], _field_dec = [fieldDec1, fieldDec2], _staticField_dec = [staticFieldDec1, staticFieldDec2], _getter_dec = [getterDec1, getterDec2], _staticGetter_dec = [staticGetterDec1, staticGetterDec2], _setter_dec = [setterDec1, setterDec2], _staticSetter_dec = [staticSetterDec1, staticSetterDec2], _accessor_dec = [accessorDec1, accessorDec2], _staticAccessor_dec = [staticAccessorDec1, staticAccessorDec2], _a) {
      constructor() {
        log.push("ctor:start");
        super();
        __runInitializers(_init, 5, this);
        __privateAdd(this, _Foo_instances);
        __privateAdd(this, _field, __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        log.push("ctor:end");
      }
    }
    _Foo_instances = new WeakSet();
    method_fn = function() {
    };
    _Foo_static = new WeakSet();
    staticMethod_fn = function() {
    };
    _field = new WeakMap();
    _staticField = new WeakMap();
    getter_get = function() {
      return;
    };
    staticGetter_get = function() {
      return;
    };
    setter_set = function(x) {
    };
    staticSetter_set = function(x) {
    };
    _accessor = new WeakMap();
    _staticAccessor = new WeakMap();
    staticMethod_fn = __decorateElement(_init, 25, "#staticMethod", _staticMethod_dec, _Foo_static, staticMethod_fn);
    staticGetter_get = __decorateElement(_init, 26, "#staticGetter", _staticGetter_dec, _Foo_static, staticGetter_get);
    staticSetter_set = __decorateElement(_init, 27, "#staticSetter", _staticSetter_dec, _Foo_static, staticSetter_set);
    _c = __decorateElement(_init, 28, "#staticAccessor", _staticAccessor_dec, _Foo_static, _staticAccessor), staticAccessor_get = _c.get, staticAccessor_set = _c.set;
    method_fn = __decorateElement(_init, 17, "#method", _method_dec, _Foo_instances, method_fn);
    getter_get = __decorateElement(_init, 18, "#getter", _getter_dec, _Foo_instances, getter_get);
    setter_set = __decorateElement(_init, 19, "#setter", _setter_dec, _Foo_instances, setter_set);
    _b = __decorateElement(_init, 20, "#accessor", _accessor_dec, _Foo_instances, _accessor), accessor_get = _b.get, accessor_set = _b.set;
    __decorateElement(_init, 29, "#staticField", _staticField_dec, _staticField);
    __decorateElement(_init, 21, "#field", _field_dec, _field);
    __privateAdd(Foo2, _Foo_static);
    Foo2 = __decorateElement(_init, 0, "Foo", _Foo_decorators, Foo2);
    __runInitializers(_init, 3, Foo2);
    log.push("static:start");
    __privateAdd(Foo2, _staticField, __runInitializers(_init, 14, Foo2)), __runInitializers(_init, 17, Foo2);
    __privateAdd(Foo2, _staticAccessor, __runInitializers(_init, 6, Foo2)), __runInitializers(_init, 9, Foo2);
    log.push("static:end");
    __runInitializers(_init, 1, Foo2);
    log.push("after");
    new Foo2();
    log.push("end");
    assertEq(() => log + "", "start,extends,M1,M2,G1,G2,S1,S2,A1,A2,m1,m2,g1,g2,s1,s2,a1,a2,F1,F2,f1,f2,c1,c2,M3,M4,M5,M6,G3,G4,G5,G6,S3,S4,S5,S6,static:start,F7,F8,F3,F4,F5,F6,A7,A8,A3,A4,A5,A6,static:end,c3,c4,c5,c6,after,ctor:start,m3,m4,m5,m6,g3,g4,g5,g6,s3,s4,s5,s6,f7,f8,f3,f4,f5,f6,a7,a8,a3,a4,a5,a6,ctor:end,end");
  },
  "Initializer order (private members, class expression)": () => {
    var _staticAccessor_dec, _accessor_dec, _staticSetter_dec, _setter_dec, _staticGetter_dec, _getter_dec, _staticField_dec, _field_dec, _staticMethod_dec, _method_dec, _a, _class_decorators, _init, _instances, method_fn, _static, _b, staticMethod_fn, _field, _staticField, getter_get, staticGetter_get, setter_set, staticSetter_set, _accessor, _c, accessor_get, accessor_set, _staticAccessor, _d, staticAccessor_get, staticAccessor_set;
    const log = [];
    const classDec1 = (cls, ctxClass) => {
      log.push("c2");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c5"));
      ctxClass.addInitializer(() => log.push("c6"));
    };
    const classDec2 = (cls, ctxClass) => {
      log.push("c1");
      if (!assertEq(() => typeof ctxClass.addInitializer, "function")) return;
      ctxClass.addInitializer(() => log.push("c3"));
      ctxClass.addInitializer(() => log.push("c4"));
    };
    const methodDec1 = (fn, ctxMethod) => {
      log.push("m2");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m5"));
      ctxMethod.addInitializer(() => log.push("m6"));
    };
    const methodDec2 = (fn, ctxMethod) => {
      log.push("m1");
      if (!assertEq(() => typeof ctxMethod.addInitializer, "function")) return;
      ctxMethod.addInitializer(() => log.push("m3"));
      ctxMethod.addInitializer(() => log.push("m4"));
    };
    const staticMethodDec1 = (fn, ctxStaticMethod) => {
      log.push("M2");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M5"));
      ctxStaticMethod.addInitializer(() => log.push("M6"));
    };
    const staticMethodDec2 = (fn, ctxStaticMethod) => {
      log.push("M1");
      if (!assertEq(() => typeof ctxStaticMethod.addInitializer, "function")) return;
      ctxStaticMethod.addInitializer(() => log.push("M3"));
      ctxStaticMethod.addInitializer(() => log.push("M4"));
    };
    const fieldDec1 = (value, ctxField) => {
      log.push("f2");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f5"));
      ctxField.addInitializer(() => log.push("f6"));
      return () => {
        log.push("f7");
      };
    };
    const fieldDec2 = (value, ctxField) => {
      log.push("f1");
      if (!assertEq(() => typeof ctxField.addInitializer, "function")) return;
      ctxField.addInitializer(() => log.push("f3"));
      ctxField.addInitializer(() => log.push("f4"));
      return () => {
        log.push("f8");
      };
    };
    const staticFieldDec1 = (value, ctxStaticField) => {
      log.push("F2");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F5"));
      ctxStaticField.addInitializer(() => log.push("F6"));
      return () => {
        log.push("F7");
      };
    };
    const staticFieldDec2 = (value, ctxStaticField) => {
      log.push("F1");
      if (!assertEq(() => typeof ctxStaticField.addInitializer, "function")) return;
      ctxStaticField.addInitializer(() => log.push("F3"));
      ctxStaticField.addInitializer(() => log.push("F4"));
      return () => {
        log.push("F8");
      };
    };
    const getterDec1 = (fn, ctxGetter) => {
      log.push("g2");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g5"));
      ctxGetter.addInitializer(() => log.push("g6"));
    };
    const getterDec2 = (fn, ctxGetter) => {
      log.push("g1");
      if (!assertEq(() => typeof ctxGetter.addInitializer, "function")) return;
      ctxGetter.addInitializer(() => log.push("g3"));
      ctxGetter.addInitializer(() => log.push("g4"));
    };
    const staticGetterDec1 = (fn, ctxStaticGetter) => {
      log.push("G2");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G5"));
      ctxStaticGetter.addInitializer(() => log.push("G6"));
    };
    const staticGetterDec2 = (fn, ctxStaticGetter) => {
      log.push("G1");
      if (!assertEq(() => typeof ctxStaticGetter.addInitializer, "function")) return;
      ctxStaticGetter.addInitializer(() => log.push("G3"));
      ctxStaticGetter.addInitializer(() => log.push("G4"));
    };
    const setterDec1 = (fn, ctxSetter) => {
      log.push("s2");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s5"));
      ctxSetter.addInitializer(() => log.push("s6"));
    };
    const setterDec2 = (fn, ctxSetter) => {
      log.push("s1");
      if (!assertEq(() => typeof ctxSetter.addInitializer, "function")) return;
      ctxSetter.addInitializer(() => log.push("s3"));
      ctxSetter.addInitializer(() => log.push("s4"));
    };
    const staticSetterDec1 = (fn, ctxStaticSetter) => {
      log.push("S2");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S5"));
      ctxStaticSetter.addInitializer(() => log.push("S6"));
    };
    const staticSetterDec2 = (fn, ctxStaticSetter) => {
      log.push("S1");
      if (!assertEq(() => typeof ctxStaticSetter.addInitializer, "function")) return;
      ctxStaticSetter.addInitializer(() => log.push("S3"));
      ctxStaticSetter.addInitializer(() => log.push("S4"));
    };
    const accessorDec1 = (target, ctxAccessor) => {
      log.push("a2");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a5"));
      ctxAccessor.addInitializer(() => log.push("a6"));
      return { init() {
        log.push("a7");
      } };
    };
    const accessorDec2 = (target, ctxAccessor) => {
      log.push("a1");
      if (!assertEq(() => typeof ctxAccessor.addInitializer, "function")) return;
      ctxAccessor.addInitializer(() => log.push("a3"));
      ctxAccessor.addInitializer(() => log.push("a4"));
      return { init() {
        log.push("a8");
      } };
    };
    const staticAccessorDec1 = (target, ctxStaticAccessor) => {
      log.push("A2");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A5"));
      ctxStaticAccessor.addInitializer(() => log.push("A6"));
      return { init() {
        log.push("A7");
      } };
    };
    const staticAccessorDec2 = (target, ctxStaticAccessor) => {
      log.push("A1");
      if (!assertEq(() => typeof ctxStaticAccessor.addInitializer, "function")) return;
      ctxStaticAccessor.addInitializer(() => log.push("A3"));
      ctxStaticAccessor.addInitializer(() => log.push("A4"));
      return { init() {
        log.push("A8");
      } };
    };
    log.push("start");
    const Foo2 = (_init = [, , ,], _class_decorators = [classDec1, classDec2], _b = class extends (_a = (log.push("extends"), Object), _method_dec = [methodDec1, methodDec2], _staticMethod_dec = [staticMethodDec1, staticMethodDec2], _field_dec = [fieldDec1, fieldDec2], _staticField_dec = [staticFieldDec1, staticFieldDec2], _getter_dec = [getterDec1, getterDec2], _staticGetter_dec = [staticGetterDec1, staticGetterDec2], _setter_dec = [setterDec1, setterDec2], _staticSetter_dec = [staticSetterDec1, staticSetterDec2], _accessor_dec = [accessorDec1, accessorDec2], _staticAccessor_dec = [staticAccessorDec1, staticAccessorDec2], _a) {
      constructor() {
        log.push("ctor:start");
        super();
        __runInitializers(_init, 5, this);
        __privateAdd(this, _instances);
        __privateAdd(this, _field, __runInitializers(_init, 18, this)), __runInitializers(_init, 21, this);
        __privateAdd(this, _accessor, __runInitializers(_init, 10, this)), __runInitializers(_init, 13, this);
        log.push("ctor:end");
      }
    }, _instances = new WeakSet(), method_fn = function() {
    }, _static = new WeakSet(), staticMethod_fn = function() {
    }, _field = new WeakMap(), _staticField = new WeakMap(), getter_get = function() {
      return;
    }, staticGetter_get = function() {
      return;
    }, setter_set = function(x) {
    }, staticSetter_set = function(x) {
    }, _accessor = new WeakMap(), _staticAccessor = new WeakMap(), staticMethod_fn = __decorateElement(_init, 25, "#staticMethod", _staticMethod_dec, _static, staticMethod_fn), staticGetter_get = __decorateElement(_init, 26, "#staticGetter", _staticGetter_dec, _static, staticGetter_get), staticSetter_set = __decorateElement(_init, 27, "#staticSetter", _staticSetter_dec, _static, staticSetter_set), _d = __decorateElement(_init, 28, "#staticAccessor", _staticAccessor_dec, _static, _staticAccessor), staticAccessor_get = _d.get, staticAccessor_set = _d.set, method_fn = __decorateElement(_init, 17, "#method", _method_dec, _instances, method_fn), getter_get = __decorateElement(_init, 18, "#getter", _getter_dec, _instances, getter_get), setter_set = __decorateElement(_init, 19, "#setter", _setter_dec, _instances, setter_set), _c = __decorateElement(_init, 20, "#accessor", _accessor_dec, _instances, _accessor), accessor_get = _c.get, accessor_set = _c.set, __decorateElement(_init, 29, "#staticField", _staticField_dec, _staticField), __decorateElement(_init, 21, "#field", _field_dec, _field), __privateAdd(_b, _static), _b = __decorateElement(_init, 0, "", _class_decorators, _b), __runInitializers(_init, 3, _b), log.push("static:start"), __privateAdd(_b, _staticField, __runInitializers(_init, 14, _b)), __runInitializers(_init, 17, _b), __privateAdd(_b, _staticAccessor, __runInitializers(_init, 6, _b)), __runInitializers(_init, 9, _b), log.push("static:end"), __runInitializers(_init, 1, _b), _b);
    log.push("after");
    new Foo2();
    log.push("end");
    assertEq(() => log + "", "start,extends,M1,M2,G1,G2,S1,S2,A1,A2,m1,m2,g1,g2,s1,s2,a1,a2,F1,F2,f1,f2,c1,c2,M3,M4,M5,M6,G3,G4,G5,G6,S3,S4,S5,S6,static:start,F7,F8,F3,F4,F5,F6,A7,A8,A3,A4,A5,A6,static:end,c3,c4,c5,c6,after,ctor:start,m3,m4,m5,m6,g3,g4,g5,g6,s3,s4,s5,s6,f7,f8,f3,f4,f5,f6,a7,a8,a3,a4,a5,a6,ctor:end,end");
  }
};
function prettyPrint(x) {
  if (x && x.prototype && x.prototype.constructor === x) return "class";
  if (typeof x === "string") return JSON.stringify(x);
  return x;
}
function assertEq(callback, expected) {
  let details;
  try {
    let x = callback();
    if (x === expected) return true;
    details = `  Expected: ${prettyPrint(expected)}
  Observed: ${prettyPrint(x)}`;
  } catch (error) {
    details = `  Throws: ${error}`;
  }
  const code = callback.toString().replace(/^\(\) => /, "").replace(/\s+/g, " ");
  console.log(`\u274C ${testName}
  Code: ${code}
${details}
`);
  failures++;
  return false;
}
function assertThrows(callback, expected) {
  let details;
  try {
    let x = callback();
    details = `  Expected: throws instanceof ${expected.name}
  Observed: returns ${prettyPrint(x)}`;
  } catch (error) {
    if (error instanceof expected) return true;
    details = `  Expected: throws instanceof ${expected.name}
  Observed: throws ${error}`;
  }
  const code = callback.toString().replace(/^\(\) => /, "").replace(/\s+/g, " ");
  console.log(`\u274C ${testName}
  Code: ${code}
${details}
`);
  failures++;
  return false;
}
let testName;
let failures = 0;
async function run() {
  for (const [name, test] of Object.entries(tests)) {
    testName = name;
    try {
      await test();
    } catch (err) {
      console.log(`\u274C ${name}
  Throws: ${err}
`);
      failures++;
    }
  }
  if (failures > 0) {
    console.log(`\u274C ${failures} checks failed`);
  } else {
    console.log(`\u2705 All checks passed`);
  }
}
const promise = run();
