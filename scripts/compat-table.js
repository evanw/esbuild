// Run this using "make compat-table"
const fs = require('fs')
const path = require('path')
const es5 = require('../github/compat-table/data-es5')
const es6 = require('../github/compat-table/data-es6')
const stage1to3 = require('../github/compat-table/data-esnext')
const stage4 = require('../github/compat-table/data-es2016plus')
const environments = require('../github/compat-table/environments.json')
const compareVersions = require('../github/compat-table/build-utils/compare-versions')
const parseEnvsVersions = require('../github/compat-table/build-utils/parse-envs-versions')
const interpolateAllResults = require('../github/compat-table/build-utils/interpolate-all-results')

interpolateAllResults(es5.tests, environments)
interpolateAllResults(es6.tests, environments)
interpolateAllResults(stage1to3.tests, environments)
interpolateAllResults(stage4.tests, environments)

const features = {
  // ES5 features
  'Object/array literal extensions: Getter accessors': { target: 'ObjectAccessors' },
  'Object/array literal extensions: Setter accessors': { target: 'ObjectAccessors' },

  // ES6 features
  'default function parameters': { target: 'DefaultArgument' },
  'rest parameters': { target: 'RestArgument' },
  'spread syntax for iterable objects': { target: 'ArraySpread' },
  'object literal extensions': { target: 'ObjectExtensions' },
  'for..of loops': { target: 'ForOf' },
  'template literals': { target: 'TemplateLiteral' },
  'destructuring, declarations': { target: 'Destructuring' },
  'destructuring, assignment': { target: 'Destructuring' },
  'destructuring, parameters': { target: 'Destructuring' },
  'new.target': { target: 'NewTarget' },
  'const': { target: 'Const' },
  'let': { target: 'Let' },
  'arrow functions': { target: 'Arrow' },
  'class': { target: 'Class' },
  'generators': { target: 'Generator' },
  'Unicode code point escapes': { target: 'UnicodeEscapes' },

  // >ES6 features
  'exponentiation (**) operator': { target: 'ExponentOperator' },
  'nested rest destructuring, declarations': { target: 'NestedRestBinding' },
  'nested rest destructuring, parameters': { target: 'NestedRestBinding' },
  'async functions': { target: 'AsyncAwait' },
  'object rest/spread properties': { target: 'ObjectRestSpread' },
  'Asynchronous Iterators: async generators': { target: 'AsyncGenerator' },
  'Asynchronous Iterators: for-await-of loops': { target: 'ForAwait' },
  'optional catch binding': { target: 'OptionalCatchBinding' },
  'BigInt: basic functionality': { target: 'BigInt' },
  'optional chaining operator (?.)': { target: 'OptionalChain' },
  'nullish coalescing operator (??)': { target: 'NullishCoalescing' },
  'Logical Assignment': { target: 'LogicalAssignment' },
  'Hashbang Grammar': { target: 'Hashbang' },

  // Public fields
  'instance class fields: public instance class fields': { target: 'ClassField' },
  'instance class fields: computed instance class fields': { target: 'ClassField' },
  'static class fields: public static class fields': { target: 'ClassStaticField' },
  'static class fields: computed static class fields': { target: 'ClassStaticField' },

  // Private fields
  'instance class fields: private instance class fields basic support': { target: 'ClassPrivateField' },
  'instance class fields: private instance class fields initializers': { target: 'ClassPrivateField' },
  'instance class fields: optional private instance class fields access': { target: 'ClassPrivateField' },
  'instance class fields: optional deep private instance class fields access': { target: 'ClassPrivateField' },
  'static class fields: private static class fields': { target: 'ClassPrivateStaticField' },

  // Private methods
  'private class methods: private instance methods': { target: 'ClassPrivateMethod' },
  'private class methods: private accessor properties': { target: 'ClassPrivateAccessor' },
  'private class methods: private static methods': { target: 'ClassPrivateStaticMethod' },
  'private class methods: private static accessor properties': { target: 'ClassPrivateStaticAccessor' },
}

const versions = {}
const engines = [
  'chrome',
  'edge',
  'es',
  'firefox',
  'ios',
  'node',
  'safari',
]

function mergeVersions(target, res) {
  const map = versions[target] || (versions[target] = {})
  for (const key in res) {
    if (res[key] === true) {
      const match = /^([a-z_]+)[0-9_]+$/.exec(key)
      if (match) {
        const engine = match[1]
        if (engines.indexOf(engine) >= 0) {
          const version = parseEnvsVersions({ [key]: true })[engine][0].version
          if (!map[engine] || compareVersions(version, map[engine]) < 0) {
            map[engine] = version
          }
        }
      }
    }
  }
}

// ES5 features
mergeVersions('ObjectAccessors', { es5: true })

// ES6 features
mergeVersions('ArraySpread', { es2015: true })
mergeVersions('Arrow', { es2015: true })
mergeVersions('Class', { es2015: true })
mergeVersions('Const', { es2015: true })
mergeVersions('DefaultArgument', { es2015: true })
mergeVersions('Destructuring', { es2015: true })
mergeVersions('ForOf', { es2015: true })
mergeVersions('Generator', { es2015: true })
mergeVersions('Let', { es2015: true })
mergeVersions('NewTarget', { es2015: true })
mergeVersions('ObjectExtensions', { es2015: true })
mergeVersions('RestArgument', { es2015: true })
mergeVersions('TemplateLiteral', { es2015: true })
mergeVersions('UnicodeEscapes', { es2015: true })

// >ES6 features
mergeVersions('ExponentOperator', { es2016: true })
mergeVersions('NestedRestBinding', { es2016: true })
mergeVersions('AsyncAwait', { es2017: true })
mergeVersions('AsyncGenerator', { es2018: true })
mergeVersions('ForAwait', { es2018: true })
mergeVersions('ObjectRestSpread', { es2018: true })
mergeVersions('OptionalCatchBinding', { es2019: true })
mergeVersions('BigInt', { es2020: true })
mergeVersions('ImportMeta', { es2020: true })
mergeVersions('NullishCoalescing', { es2020: true })
mergeVersions('OptionalChain', { es2020: true })
mergeVersions('TopLevelAwait', {})

// Manually copied from https://caniuse.com/?search=export%20*%20as
mergeVersions('ExportStarAs', {
  chrome72: true,
  edge79: true,
  es2020: true,
  firefox80: true,
  node12: true, // From https://developer.mozilla.org/en-US/docs/web/javascript/reference/statements/export
})

// Manually copied from https://caniuse.com/#search=import.meta
mergeVersions('ImportMeta', {
  chrome64: true,
  edge79: true,
  es2020: true,
  firefox62: true,
  ios12: true,
  node10_4: false, // From https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/import.meta
  safari11_1: true,
})

for (const test of [...es5.tests, ...es6.tests, ...stage4.tests, ...stage1to3.tests]) {
  const feature = features[test.name]
  if (feature) {
    feature.found = true
    if (test.subtests) {
      for (const subtest of test.subtests) {
        mergeVersions(feature.target, subtest.res)
      }
    } else {
      mergeVersions(feature.target, test.res)
    }
  } else if (test.subtests) {
    for (const subtest of test.subtests) {
      const feature = features[`${test.name}: ${subtest.name}`]
      if (feature) {
        feature.found = true
        mergeVersions(feature.target, subtest.res)
      }
    }
  }
}

for (const feature in features) {
  if (!features[feature].found) {
    throw new Error(`Did not find ${feature}`)
  }
}

function upper(text) {
  if (text === 'es' || text === 'ios') return text.toUpperCase()
  return text[0].toUpperCase() + text.slice(1)
}

function writeInnerMap(obj) {
  const keys = Object.keys(obj).sort()
  const maxLength = keys.reduce((a, b) => Math.max(a, b.length + 1), 0)
  if (keys.length === 0) return '{}'
  return `{\n${keys.map(x => `\t\t${(upper(x) + ':').padEnd(maxLength)} {${obj[x].join(', ')}},`).join('\n')}\n\t}`
}

fs.writeFileSync(__dirname + '/../internal/compat/js_table.go',
  `// This file was automatically generated by "${path.basename(__filename)}"

package compat

type Engine uint8

const (
${engines.map((x, i) => `\t${upper(x)}${i ? '' : ' Engine = iota'}`).join('\n')}
)

type JSFeature uint64

const (
${Object.keys(versions).sort().map((x, i) => `\t${x}${i ? '' : ' JSFeature = 1 << iota'}`).join('\n')}
)

func (features JSFeature) Has(feature JSFeature) bool {
\treturn (features & feature) != 0
}

var jsTable = map[JSFeature]map[Engine][]int{
${Object.keys(versions).sort().map(x => `\t${x}: ${writeInnerMap(versions[x])},`).join('\n')}
}

func isVersionLessThan(a []int, b []int) bool {
\tfor i := 0; i < len(a) && i < len(b); i++ {
\t\tif a[i] > b[i] {
\t\t\treturn false
\t\t}
\t\tif a[i] < b[i] {
\t\t\treturn true
\t\t}
\t}
\treturn len(a) < len(b)
}

// Return all features that are not available in at least one environment
func UnsupportedJSFeatures(constraints map[Engine][]int) (unsupported JSFeature) {
\tfor feature, engines := range jsTable {
\t\tfor engine, version := range constraints {
\t\t\tif minVersion, ok := engines[engine]; !ok || isVersionLessThan(version, minVersion) {
\t\t\t\tunsupported |= feature
\t\t\t}
\t\t}
\t}
\treturn
}
`)
