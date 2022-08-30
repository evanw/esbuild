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
  'const': { target: 'ConstAndLet' },
  'let': { target: 'ConstAndLet' },
  'arrow functions': { target: 'Arrow' },
  'class': { target: 'Class' },
  'generators': { target: 'Generator' },
  'Unicode code point escapes': { target: 'UnicodeEscapes' },
  'RegExp "y" and "u" flags': { target: 'RegexpStickyAndUnicodeFlags' },

  // >ES6 features
  'exponentiation (**) operator': { target: 'ExponentOperator' },
  'nested rest destructuring, declarations': { target: 'NestedRestBinding' },
  'nested rest destructuring, parameters': { target: 'NestedRestBinding' },
  'async functions': { target: 'AsyncAwait' },
  'object rest/spread properties': { target: 'ObjectRestSpread' },
  'RegExp Lookbehind Assertions': { target: 'RegexpLookbehindAssertions' },
  'RegExp named capture groups': { target: 'RegexpNamedCaptureGroups' },
  'RegExp Unicode Property Escapes': { target: 'RegexpUnicodePropertyEscapes' },
  's (dotAll) flag for regular expressions': { target: 'RegexpDotAllFlag' },
  'Asynchronous Iterators: async generators': { target: 'AsyncGenerator' },
  'Asynchronous Iterators: for-await-of loops': { target: 'ForAwait' },
  'optional catch binding': { target: 'OptionalCatchBinding' },
  'BigInt: basic functionality': { target: 'Bigint' },
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

  // Private "in"
  'Ergonomic brand checks for private fields': { target: 'ClassPrivateBrandCheck' },
}

const versions = {}
const engines = [
  // The JavaScript standard
  'es',

  // Common JavaScript runtimes
  'chrome',
  'edge',
  'firefox',
  'ie',
  'ios',
  'node',
  'opera',
  'safari',

  // Uncommon JavaScript runtimes
  'hermes',
  'rhino',
]

function getValueOfTest(value) {
  // Handle values like this:
  //
  //   {
  //     val: true,
  //     note_id: "ff-shorthand-methods",
  //     ...
  //   }
  //
  if (typeof value === 'object' && value !== null) {
    return value.val === true
  }

  // String values such as "flagged" are considered to be false
  return value === true
}

function mergeVersions(target, res) {
  // The original data set will contain something like "chrome44: true" for a
  // given feature. And the interpolation script will expand this to something
  // like "chrome44: true, chrome45: true, chrome46: true, ..." so we want to
  // take the minimum version to find the boundary.
  const lowestVersionMap = {}

  for (const key in res) {
    if (getValueOfTest(res[key])) {
      const match = /^([a-z_]+)[0-9_]+$/.exec(key)
      if (match) {
        const engine = match[1]
        if (engines.indexOf(engine) >= 0) {
          const version = parseEnvsVersions({ [key]: true })[engine][0].version
          if (!lowestVersionMap[engine] || compareVersions({ version }, { version: lowestVersionMap[engine] }) < 0) {
            lowestVersionMap[engine] = version
          }
        }
      }
    }
  }

  // The original data set can sometimes contain many subtests. We only want to
  // support a given feature if the version is greater than the maximum version
  // for all subtests. This is the inverse of the minimum test below.
  const highestVersionMap = versions[target] || (versions[target] = {})
  for (const engine in lowestVersionMap) {
    const version = lowestVersionMap[engine]
    if (!highestVersionMap[engine] || compareVersions({ version }, { version: highestVersionMap[engine][0].start }) > 0) {
      highestVersionMap[engine] = [{ start: version, end: null }]
    }
  }
}

// ES5 features
mergeVersions('ObjectAccessors', { es5: true })

// ES6 features
mergeVersions('ArraySpread', { es2015: true })
mergeVersions('Arrow', { es2015: true })
mergeVersions('Class', { es2015: true })
mergeVersions('ConstAndLet', { es2015: true })
mergeVersions('DefaultArgument', { es2015: true })
mergeVersions('Destructuring', { es2015: true })
mergeVersions('DynamicImport', { es2015: true })
mergeVersions('ForOf', { es2015: true })
mergeVersions('Generator', { es2015: true })
mergeVersions('NewTarget', { es2015: true })
mergeVersions('ObjectExtensions', { es2015: true })
mergeVersions('RegexpStickyAndUnicodeFlags', { es2015: true })
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
mergeVersions('RegexpDotAllFlag', { es2018: true })
mergeVersions('RegexpLookbehindAssertions', { es2018: true })
mergeVersions('RegexpNamedCaptureGroups', { es2018: true })
mergeVersions('RegexpUnicodePropertyEscapes', { es2018: true })
mergeVersions('OptionalCatchBinding', { es2019: true })
mergeVersions('Bigint', { es2020: true })
mergeVersions('ImportMeta', { es2020: true })
mergeVersions('NullishCoalescing', { es2020: true })
mergeVersions('OptionalChain', { es2020: true })
mergeVersions('TypeofExoticObjectIsObject', { es2020: true }) // https://github.com/tc39/ecma262/pull/1441
mergeVersions('LogicalAssignment', { es2021: true })
mergeVersions('ClassField', { es2022: true })
mergeVersions('ClassPrivateAccessor', { es2022: true })
mergeVersions('ClassPrivateBrandCheck', { es2022: true })
mergeVersions('ClassPrivateField', { es2022: true })
mergeVersions('ClassPrivateMethod', { es2022: true })
mergeVersions('ClassPrivateStaticAccessor', { es2022: true })
mergeVersions('ClassPrivateStaticField', { es2022: true })
mergeVersions('ClassPrivateStaticMethod', { es2022: true })
mergeVersions('ClassStaticBlocks', { es2022: true })
mergeVersions('ClassStaticField', { es2022: true })
mergeVersions('TopLevelAwait', { es2022: true })
mergeVersions('ArbitraryModuleNamespaceNames', { es2022: true })
mergeVersions('RegexpMatchIndices', { es2022: true })
mergeVersions('ImportAssertions', {})

// Manually copied from https://caniuse.com/?search=export%20*%20as
mergeVersions('ExportStarAs', {
  chrome72: true,
  edge79: true,
  es2020: true,
  firefox80: true,
  node12: true, // From https://developer.mozilla.org/en-US/docs/web/javascript/reference/statements/export
  opera60: true,

  // This feature has been implemented in Safari but I have no idea what version
  // this bug corresponds to: https://bugs.webkit.org/show_bug.cgi?id=214379
})

// Manually copied from https://caniuse.com/#search=import.meta
mergeVersions('ImportMeta', {
  chrome64: true,
  edge79: true,
  firefox62: true,
  ios12: true,
  node10_4: true, // From https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/import.meta
  opera51: true,
  safari11_1: true,
})

// Manually copied from https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/await
mergeVersions('TopLevelAwait', {
  chrome89: true,
  edge89: true,
  firefox89: true,
  ios15: true,
  node14_8: true,
  opera75: true,
  safari15: true,
})

// Manually copied from https://caniuse.com/es6-module-dynamic-import
mergeVersions('DynamicImport', {
  chrome63: true,
  edge79: true,
  firefox67: true,
  ios11: true,
  opera50: true,
  safari11_1: true,
})

// This is a problem specific to Internet explorer. See https://github.com/tc39/ecma262/issues/1440
mergeVersions('TypeofExoticObjectIsObject', {
  chrome0: true,
  edge0: true,
  es0: true,
  firefox0: true,
  ios0: true,
  node0: true,
  opera0: true,
  safari0: true,
})

// This is a special case. Node added support for it to both v12.20+ and v13.2+
// so the range is inconveniently discontiguous. Sources:
//
// - https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/import
// - https://github.com/nodejs/node/pull/35950
// - https://github.com/nodejs/node/pull/31974
//
versions.DynamicImport.node = [
  { start: [12, 20], end: [13] },
  { start: [13, 2] },
]

// Manually copied from https://nodejs.org/api/esm.html#node-imports
versions.NodeColonPrefixImport = {
  node: [
    { start: [12, 20], end: [13] },
    { start: [14, 13, 1] },
  ]
}
versions.NodeColonPrefixRequire = {
  node: [
    { start: [14, 18], end: [15] },
    { start: [16] },
  ]
}

mergeVersions('ArbitraryModuleNamespaceNames', {
  // From https://github.com/tc39/ecma262/pull/2154#issuecomment-825201030
  chrome90: true,
  node16: true,

  // From https://bugzilla.mozilla.org/show_bug.cgi?id=1670044
  firefox87: true,

  // This feature has been implemented in Safari but I have no idea what version
  // this bug corresponds to: https://bugs.webkit.org/show_bug.cgi?id=217576
})

mergeVersions('ImportAssertions', {
  // From https://www.chromestatus.com/feature/5765269513306112
  chrome91: true,

  // From https://github.com/nodejs/node/blob/master/doc/changelogs/CHANGELOG_V16.md#16.14.0
  node16_14: true,

  // Not yet in Firefox: https://bugzilla.mozilla.org/show_bug.cgi?id=1736059
})

// Manually copied from https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Classes/Class_static_initialization_blocks
mergeVersions('ClassStaticBlocks', {
  chrome91: true, // From https://www.chromestatus.com/feature/6482797915013120
  edge94: true,
  firefox93: true,
  node16_11: true,
  opera80: true,
})

// Manually copied from https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/RegExp/hasIndices
mergeVersions('RegexpMatchIndices', {
  chrome90: true,
  edge90: true,
  firefox88: true,
  ios15: true,
  opera76: true,
  safari15: true,
})

for (const test of [...es5.tests, ...es6.tests, ...stage4.tests, ...stage1to3.tests]) {
  const feature = features[test.name]
  if (feature) {
    feature.found = true
    if (test.subtests) {
      const res = {}

      // Intersect all subtests (so a key is only true if it's true in all subtests)
      for (const subtest of test.subtests)
        for (const key in subtest.res)
          res[key] = true
      for (const subtest of test.subtests)
        for (const key in res)
          res[key] &&= getValueOfTest(subtest.res[key] ?? false)

      mergeVersions(feature.target, res)
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
  if (text === 'es' || text === 'ios' || text === 'ie') return text.toUpperCase()
  return text[0].toUpperCase() + text.slice(1)
}

function jsFeatureString(feature) {
  return feature.replace(/([A-Z])/g, '-$1').slice(1).toLowerCase()
}

function simpleMap(entries) {
  let maxLength = 0
  for (const [key] of entries) {
    maxLength = Math.max(maxLength, key.length + 1)
  }
  return entries.map(([key, value]) => `\t${(key + ':').padEnd(maxLength)} ${value},`).join('\n')
}

function jsTableMap(obj) {
  const keys = Object.keys(obj).sort()
  const maxLength = keys.reduce((a, b) => Math.max(a, b.length + 1), 0)
  if (keys.length === 0) return '{}'
  return `{\n${keys.map(x => {
    const items = obj[x].map(y => {
      return `{start: v{${y.start.concat(0, 0).slice(0, 3).join(', ')
        }}${y.end ? `, end: v{${y.end.concat(0, 0).slice(0, 3).join(', ')}}` : ''}}`
    })
    return `\t\t${(upper(x) + ':').padEnd(maxLength)} {${items.join(', ')}},`
  }).join('\n')}\n\t}`
}

function jsTableValidEnginesMap(engines) {
  const keys = engines.slice().sort()
  const maxLength = keys.reduce((a, b) => Math.max(a, b.length + 4), 0)
  if (keys.length === 0) return '{}'
  return keys.map(x => {
    return `\t${`"${x}": `.padEnd(maxLength)}api.Engine${upper(x)},`
  }).join('\n')
}

fs.writeFileSync(__dirname + '/../internal/compat/js_table.go',
  `// This file was automatically generated by "${path.basename(__filename)}"

package compat

type Engine uint8

const (
${engines.slice().sort().map((x, i) => `\t${upper(x)}${i ? '' : ' Engine = iota'}`).join('\n')}
)

func (e Engine) String() string {
\tswitch e {
${engines.slice().sort().map(x => `\tcase ${upper(x)}:\n\t\treturn "${x}"`).join('\n')}
\t}
\treturn ""
}

type JSFeature uint64

const (
${Object.keys(versions).sort().map((x, i) => `\t${x}${i ? '' : ' JSFeature = 1 << iota'}`).join('\n')}
)

var StringToJSFeature = map[string]JSFeature{
${simpleMap(Object.keys(versions).sort().map(x => [`"${jsFeatureString(x)}"`, x]))}
}

var JSFeatureToString = map[JSFeature]string{
${simpleMap(Object.keys(versions).sort().map(x => [x, `"${jsFeatureString(x)}"`]))}
}

func (features JSFeature) Has(feature JSFeature) bool {
\treturn (features & feature) != 0
}

func (features JSFeature) ApplyOverrides(overrides JSFeature, mask JSFeature) JSFeature {
\treturn (features & ^mask) | (overrides & mask)
}

var jsTable = map[JSFeature]map[Engine][]versionRange{
${Object.keys(versions).sort().map(x => `\t${x}: ${jsTableMap(versions[x])},`).join('\n')}
}

// Return all features that are not available in at least one environment
func UnsupportedJSFeatures(constraints map[Engine][]int) (unsupported JSFeature) {
\tfor feature, engines := range jsTable {
\t\tfor engine, version := range constraints {
\t\t\tif versionRanges, ok := engines[engine]; !ok || !isVersionSupported(versionRanges, version) {
\t\t\t\tunsupported |= feature
\t\t\t}
\t\t}
\t}
\treturn
}
`)

fs.writeFileSync(__dirname + '/../pkg/api/api_js_table.go',
  `// This file was automatically generated by "${path.basename(__filename)}"

package api

import "github.com/evanw/esbuild/internal/compat"

type EngineName uint8

const (
${engines.filter(x => x !== 'es').map((x, i) => `\tEngine${upper(x)}${i ? '' : ' EngineName = iota'}`).join('\n')}
)

func convertEngineName(engine EngineName) compat.Engine {
\tswitch engine {
${engines.filter(x => x !== 'es').map(x => `\tcase Engine${upper(x)}:\n\t\treturn compat.${upper(x)}`).join('\n')}
\tdefault:
\t\tpanic("Invalid engine name")
\t}
}
`)

fs.writeFileSync(__dirname + '/../pkg/cli/cli_js_table.go',
  `// This file was automatically generated by "${path.basename(__filename)}"

package cli

import "github.com/evanw/esbuild/pkg/api"

var validEngines = map[string]api.EngineName{
${jsTableValidEnginesMap(engines.filter(x => x !== 'es'))}
}
`)
