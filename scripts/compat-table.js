// Run this using "make compat-table"
const fs = require('fs')
const path = require('path')
const stage1to3 = require('../github/compat-table/data-esnext')
const stage4 = require('../github/compat-table/data-es2016plus')
const environments = require('../github/compat-table/environments.json')
const compareVersions = require('../github/compat-table/build-utils/compare-versions')
const parseEnvsVersions = require('../github/compat-table/build-utils/parse-envs-versions')
const interpolateAllResults = require('../github/compat-table/build-utils/interpolate-all-results')

interpolateAllResults(stage1to3.tests, environments)
interpolateAllResults(stage4.tests, environments)

const features = {
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
      const engine = /^[a-z]*/.exec(key)[0]
      if (engines.indexOf(engine) >= 0) {
        const version = parseEnvsVersions({ [key]: true })[engine][0].version
        if (!map[engine] || compareVersions(version, map[engine]) < 0) {
          map[engine] = version
        }
      }
    }
  }
}

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

for (const test of stage4.tests.concat(stage1to3.tests)) {
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
  return keys.map(x => `\t\t${(upper(x) + ':').padEnd(maxLength)} {${obj[x].join(', ')}},`).join('\n')
}

fs.writeFileSync(__dirname + '/../internal/compat/table.go',
  `// This file was automatically generated by "${path.basename(__filename)}"

package compat

type Engine uint8

const (
${engines.map((x, i) => `\t${upper(x)}${i ? '' : ' Engine = iota'}`).join('\n')}
)

type Feature uint32

const (
${Object.keys(versions).sort().map((x, i) => `\t${x}${i ? '' : ' Feature = 1 << iota'}`).join('\n')}
)

func (features Feature) Has(feature Feature) bool {
\treturn (features & feature) != 0
}

var Table = map[Feature]map[Engine][]int{
${Object.keys(versions).sort().map(x => `\t${x}: {
${writeInnerMap(versions[x])}
\t},`).join('\n')}
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
func UnsupportedFeatures(constraints map[Engine][]int) (unsupported Feature) {
\tfor feature, engines := range Table {
\t\tfor engine, version := range constraints {
\t\t\tif minVersion, ok := engines[engine]; !ok || isVersionLessThan(version, minVersion) {
\t\t\t\tunsupported |= feature
\t\t\t}
\t\t}
\t}
\treturn
}
`)
