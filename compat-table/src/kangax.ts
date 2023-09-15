// This file processes the data contained in https://github.com/kangax/compat-table

import fs = require('fs')
import path = require('path')

import es5 = require('../repos/kangax/compat-table/data-es5.js')
import es6 = require('../repos/kangax/compat-table/data-es6.js')
import stage1to3 = require('../repos/kangax/compat-table/data-esnext.js')
import stage4 = require('../repos/kangax/compat-table/data-es2016plus.js')
import environments = require('../repos/kangax/compat-table/environments.json')
import parseEnvsVersions = require('../repos/kangax/compat-table/build-utils/parse-envs-versions.js')
import interpolateAllResults = require('../repos/kangax/compat-table/build-utils/interpolate-all-results.js')
import { Engine, JSFeature, SupportMap, jsFeatures } from './index'

interpolateAllResults(es5.tests, environments)
interpolateAllResults(es6.tests, environments)
interpolateAllResults(stage1to3.tests, environments)
interpolateAllResults(stage4.tests, environments)

const features: Record<string, JSFeature> = {
  // ES5 features
  'Object/array literal extensions: Getter accessors': 'ObjectAccessors',
  'Object/array literal extensions: Setter accessors': 'ObjectAccessors',

  // ES6 features
  'default function parameters': 'DefaultArgument',
  'rest parameters': 'RestArgument',
  'spread syntax for iterable objects': 'ArraySpread',
  'object literal extensions': 'ObjectExtensions',
  'for..of loops': 'ForOf',
  'template literals': 'TemplateLiteral',
  'destructuring, declarations': 'Destructuring',
  'destructuring, assignment': 'Destructuring',
  'destructuring, parameters': 'Destructuring',
  'new.target': 'NewTarget',
  'const': 'ConstAndLet',
  'let': 'ConstAndLet',
  'arrow functions': 'Arrow',
  'class': 'Class',
  'generators': 'Generator',
  'Unicode code point escapes': 'UnicodeEscapes',
  'RegExp "y" and "u" flags': 'RegexpStickyAndUnicodeFlags',

  // >ES6 features
  'exponentiation (**) operator': 'ExponentOperator',
  'nested rest destructuring, declarations': 'NestedRestBinding',
  'nested rest destructuring, parameters': 'NestedRestBinding',
  'async functions': 'AsyncAwait',
  'object rest/spread properties': 'ObjectRestSpread',
  'RegExp Lookbehind Assertions': 'RegexpLookbehindAssertions',
  'RegExp named capture groups': 'RegexpNamedCaptureGroups',
  'RegExp Unicode Property Escapes': 'RegexpUnicodePropertyEscapes',
  's (dotAll) flag for regular expressions': 'RegexpDotAllFlag',
  'Asynchronous Iterators: async generators': 'AsyncGenerator',
  'Asynchronous Iterators: for-await-of loops': 'ForAwait',
  'optional catch binding': 'OptionalCatchBinding',
  'BigInt: basic functionality': 'Bigint',
  'optional chaining operator (?.)': 'OptionalChain',
  'nullish coalescing operator (??)': 'NullishCoalescing',
  'Logical Assignment': 'LogicalAssignment',
  'Hashbang Grammar': 'Hashbang',

  // Public fields
  'instance class fields: public instance class fields': 'ClassField',
  'instance class fields: computed instance class fields': 'ClassField',
  'static class fields: public static class fields': 'ClassStaticField',
  'static class fields: computed static class fields': 'ClassStaticField',

  // Private fields
  'instance class fields: private instance class fields basic support': 'ClassPrivateField',
  'instance class fields: private instance class fields initializers': 'ClassPrivateField',
  'instance class fields: optional private instance class fields access': 'ClassPrivateField',
  'instance class fields: optional deep private instance class fields access': 'ClassPrivateField',
  'static class fields: private static class fields': 'ClassPrivateStaticField',

  // Private methods
  'private class methods: private instance methods': 'ClassPrivateMethod',
  'private class methods: private accessor properties': 'ClassPrivateAccessor',
  'private class methods: private static methods': 'ClassPrivateStaticMethod',
  'private class methods: private static accessor properties': 'ClassPrivateStaticAccessor',

  // Private "in"
  'Ergonomic brand checks for private fields': 'ClassPrivateBrandCheck',
}

const environmentToEngine: Record<string, Engine> = {
  // The JavaScript standard
  'es': 'ES',

  // Common JavaScript runtimes
  'chrome': 'Chrome',
  'edge': 'Edge',
  'firefox': 'Firefox',
  'ie': 'IE',
  'ios': 'IOS',
  'node': 'Node',
  'opera': 'Opera',
  'safari': 'Safari',

  // Uncommon JavaScript runtimes
  'deno': 'Deno',
  'hermes': 'Hermes',
  'rhino': 'Rhino',
}

const subtestsToSkip: Record<string, boolean> = {
  // Safari supposedly doesn't throw an error for duplicate identifiers in
  // a function parameter list. The failing test case looks like this:
  //
  //   var f = function f([id, id]) { return id }
  //
  // However, this code will cause a compile error with esbuild so it's not
  // possible to encounter this issue when running esbuild-generated code in
  // Safari. I'm ignoring this test since Safari's destructuring otherwise
  // works fine so destructuring shouldn't be forbidden when building for
  // Safari.
  'destructuring, parameters: duplicate identifier': true,
}

const getValueOfTest = (value: boolean | { val: boolean }): boolean => {
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

interface Test {
  name: string
  res: Record<string, boolean | { val: boolean }>
  subtests?: { name: string, res: Record<string, boolean | { val: boolean }> }[]
}

const updateMap = (map: SupportMap<JSFeature>, feature: JSFeature, engine: Engine, version: string, testName: string, passed: boolean): void => {
  const engines = map[feature] || (map[feature] = {})
  const versions = engines[engine] || (engines[engine] = {})
  const support = versions[version] || (versions[version] = {})
  if (passed) {
    support.passed = (support.passed || 0) + 1
  } else {
    support.failed ||= new Set
    support.failed.add(testName)
  }
}

const mergeIndividualTestResults = (map: SupportMap<JSFeature>, feature: JSFeature, testName: string, res: Record<string, boolean | { val: boolean }>, omit: Engine[]): void => {
  const environments = parseEnvsVersions(res)
  for (const environment in environments) {
    const engine = environmentToEngine[environment]
    if (engine && omit.indexOf(engine) < 0) {
      for (const parsed of environments[environment]) {
        const version = parsed.version.join('.')
        if (/^\d+(?:\.\d+(?:\.\d+)?)?$/.test(version)) {
          updateMap(map, feature, engine, version, testName, getValueOfTest(res[parsed.id]))
        }
      }
    }
  }
}

const mergeAllTestResults = (map: SupportMap<JSFeature>, tests: Test[], { omit = [] }: { omit?: Engine[] } = {}): void => {
  for (const test of tests) {
    const feature = features[test.name]
    if (feature) {
      if (test.subtests) {
        for (const subtest of test.subtests) {
          const fullName = `${test.name}: ${subtest.name}`
          if (subtestsToSkip[fullName]) continue
          mergeIndividualTestResults(map, feature, fullName, subtest.res, omit)
        }
      } else {
        mergeIndividualTestResults(map, feature, test.name, test.res, omit)
      }
    } else if (test.subtests) {
      for (const subtest of test.subtests) {
        const fullName = `${test.name}: ${subtest.name}`
        if (subtestsToSkip[fullName]) continue
        const feature = features[fullName]
        if (feature) mergeIndividualTestResults(map, feature, fullName, subtest.res, omit)
      }
    }
  }
}

// Node compatibility data is handled separately because the data source
// https://github.com/williamkapke/node-compat-table is (for now at least)
// more up to date than https://github.com/kangax/compat-table.
const reformatNodeCompatTable = (): Test[] => {
  const nodeCompatTableDir = path.join(__dirname, 'repos/williamkapke/node-compat-table/results/v8')
  const testMap: Record<string, Test> = {}
  const subtestMap: Record<string, Test> = {}
  const tests: Test[] = []

  // Format the data like the kangax table
  for (const entry of fs.readdirSync(nodeCompatTableDir)) {
    // Note: this omits data for the "0.x.y" releases because the data isn't clean
    const match = /^([1-9]\d*\.\d+\.\d+)\.json$/.exec(entry)
    if (match) {
      const version = 'node' + match[1].replace(/\./g, '_')
      const jsonPath = path.join(nodeCompatTableDir, entry)
      const json = JSON.parse(fs.readFileSync(jsonPath, 'utf8'))

      for (const key in json) {
        if (key.startsWith('ES')) {
          const object = json[key]

          for (const key in object) {
            const testResult = object[key]
            const split = key.replace('<code>', '').replace('</code>', '').split('›')

            if (split.length === 2) {
              let test = testMap[split[1]]
              if (!test) {
                test = testMap[split[1]] = { name: split[1], res: {} }
                tests.push(test)
              }
              test.res[version] = testResult
            }

            else if (split.length === 3) {
              const subtestKey = `${split[1]}: ${split[2]}`
              let subtest = subtestMap[subtestKey]
              if (!subtest) {
                let test = testMap[split[1]]
                if (!test) {
                  test = testMap[split[1]] = { name: split[1], res: {} }
                  tests.push(test)
                }
                subtest = subtestMap[subtestKey] = { name: split[2], res: {} }
                test.subtests ||= []
                test.subtests.push(subtest)
              }
              subtest.res[version] = testResult
            }
          }
        }
      }
    }
  }

  return tests
}

export const js: SupportMap<JSFeature> = {} as SupportMap<JSFeature>

mergeAllTestResults(js, [...es5.tests, ...es6.tests, ...stage4.tests, ...stage1to3.tests], { omit: ['Node'] })
mergeAllTestResults(js, reformatNodeCompatTable())
