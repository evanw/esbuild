const grammar = require('./estree-grammar')

function compileRule(callFactory, type, rule) {
  return random => {
    const node = { type }
    for (const key in rule) node[key] = callFactory(random, rule[key])
    return node
  }
}

function compileFactories(callFactory, rules) {
  const result = {}

  for (let type in rules) {
    const rule = rules[type]

    // Types that start with "$" have special behavior
    if (type.startsWith('$')) {
      type = type.slice(1)

      if (rule instanceof Array) {
        // This means the given type has one of several forms. This is helpful
        // when there are cross-property constraints. For example, a "Property"
        // key can only be an arbitrary expression if "computed" is true.
        const nested = rule.map(x => compileRule(callFactory, type, x))
        result[type] = random => nested[random.choice(nested.length)](random)
      }

      else {
        // This means the given type is actually an alias for a collection of
        // other types. For example, "Expression" is an alias for many other
        // types including "ArrayExpression", "ObjectExpression", etc...
        const nested = compileFactories(callFactory, rule)
        const keys = Object.keys(nested)
        result[type] = random => nested[keys[random.choice(keys.length)]](random)
      }
    }

    else {
      result[type] = compileRule(callFactory, type, rule)
    }
  }

  return result
}

function compileDefaultFactories() {
  const strings = ['a', 'b', 'c']
  const someString = random => strings[random.choice(strings.length)]

  let depth = 0
  const callFactory = (random, target) => {
    depth++
    const result = target.type ? factories[target.type](random, target) : factories[target](random)
    depth--
    return result
  }

  const factories = {
    // These combinators are referenced by rules in the grammar
    literal: (_, { literal }) => literal,
    boolean: random => random.choice(2) > 0,
    number: random => random.choice(10),
    string: random => someString(random),
    regexp: random => new RegExp(someString(random)),
    choice: (random, { of }) => callFactory(random, of[random.choice(of.length)]),
    array: (random, { of, nonEmpty, trailing }) => {
      const result = []
      if (nonEmpty) result.push(callFactory(random, of))
      if (depth < 10) {
        while (random.choice(4) > 0) {
          if (random.push()) {
            result.push(callFactory(random, of))
            random.pop()
          }
        }
      }
      if (trailing && random.choice(4) === 3) {
        if (random.push()) {
          result.push(callFactory(random, trailing))
          random.pop()
        }
      }
      return result
    },
    ...compileFactories(callFactory, grammar),
  }
  return factories
}

const defaultFactories = compileDefaultFactories()
exports.randomAST = random => defaultFactories.Program(random)
