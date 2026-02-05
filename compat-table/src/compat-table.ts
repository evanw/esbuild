// This file processes the data contained in https://github.com/kangax/compat-table

import fs = require('fs')
import path = require('path')

import es5 = require('../repos/compat-table/compat-table/data-es5.js')
import es6 = require('../repos/compat-table/compat-table/data-es6.js')
import stage1to3 = require('../repos/compat-table/compat-table/data-esnext.js')
import stage4 = require('../repos/compat-table/compat-table/data-es2016plus.js')
import environments = require('../repos/compat-table/compat-table/environments.json')
import parseEnvsVersions = require('../repos/compat-table/compat-table/build-utils/parse-envs-versions.js')
import interpolateAllResults = require('../repos/compat-table/compat-table/build-utils/interpolate-all-results.js')
import { Engine, JSFeature, SupportMap } from './index'

interpolateAllResults(es5.tests, environments)
interpolateAllResults(es6.tests, environments)
interpolateAllResults(stage1to3.tests, environments)
interpolateAllResults(stage4.tests, environments)

const features: Record<string, JSFeature> = {
  // ES5 features
  'Object/array literal extensions: Getter accessors': 'ObjectAccessors',
  'Object/array literal extensions: Setter accessors': 'ObjectAccessors',

  // ES6 features
  'arrow functions': 'Arrow',
  'class': 'Class',
  'const': 'ConstAndLet',
  'default function parameters': 'DefaultArgument',
  'destructuring, assignment': 'Destructuring',
  'destructuring, declarations': 'Destructuring',
  'destructuring, parameters': 'Destructuring',
  'for..of loops': 'ForOf',
  'function "name" property: isn\'t writable, is configurable': 'FunctionNameConfigurable',
  'generators': 'Generator',
  'let': 'ConstAndLet',
  'new.target': 'NewTarget',
  'object literal extensions': 'ObjectExtensions',
  'RegExp "y" and "u" flags': 'RegexpStickyAndUnicodeFlags',
  'rest parameters': 'RestArgument',
  'spread syntax for iterable objects': 'ArraySpread',
  'template literals': 'TemplateLiteral',
  'Unicode code point escapes': 'UnicodeEscapes',

  // >ES6 features
  'async functions': 'AsyncAwait',
  'Asynchronous Iterators: async generators': 'AsyncGenerator',
  'Asynchronous Iterators: for-await-of loops': 'ForAwait',
  'BigInt: basic functionality': 'Bigint',
  'exponentiation (**) operator': 'ExponentOperator',
  'Hashbang Grammar': 'Hashbang',
  'Logical Assignment': 'LogicalAssignment',
  'nested rest destructuring, declarations': 'NestedRestBinding',
  'nested rest destructuring, parameters': 'NestedRestBinding',
  'nullish coalescing operator (??)': 'NullishCoalescing',
  'object rest/spread properties': 'ObjectRestSpread',
  'optional catch binding': 'OptionalCatchBinding',
  'optional chaining operator (?.)': 'OptionalChain',
  'RegExp Lookbehind Assertions': 'RegexpLookbehindAssertions',
  'RegExp named capture groups': 'RegexpNamedCaptureGroups',
  'RegExp Unicode Property Escapes': 'RegexpUnicodePropertyEscapes',
  's (dotAll) flag for regular expressions': 'RegexpDotAllFlag',

  // Public fields
  'instance class fields: computed instance class fields': 'ClassField',
  'instance class fields: public instance class fields': 'ClassField',
  'static class fields: computed static class fields': 'ClassStaticField',
  'static class fields: public static class fields': 'ClassStaticField',

  // Private fields
  'instance class fields: optional deep private instance class fields access': 'ClassPrivateField',
  'instance class fields: optional private instance class fields access': 'ClassPrivateField',
  'instance class fields: private instance class fields basic support': 'ClassPrivateField',
  'instance class fields: private instance class fields initializers': 'ClassPrivateField',
  'static class fields: private static class fields': 'ClassPrivateStaticField',

  // Private methods
  'private class methods: private accessor properties': 'ClassPrivateAccessor',
  'private class methods: private instance methods': 'ClassPrivateMethod',
  'private class methods: private static accessor properties': 'ClassPrivateStaticAccessor',
  'private class methods: private static methods': 'ClassPrivateStaticMethod',

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

const mergeIndividualTestResults = (map: SupportMap<JSFeature>, feature: JSFeature, testName: string, res: Record<string, boolean | { val: boolean }>): void => {
  const environments = parseEnvsVersions(res)
  for (const environment in environments) {
    const engine = environmentToEngine[environment]
    if (engine) {
      for (const parsed of environments[environment]) {
        const version = parsed.version.join('.')
        if (/^\d+(?:\.\d+(?:\.\d+)?)?$/.test(version)) {
          updateMap(map, feature, engine, version, testName, getValueOfTest(res[parsed.id]))
        }
      }
    }
  }
}

const mergeAllTestResults = (map: SupportMap<JSFeature>, tests: Test[]): void => {
  for (const test of tests) {
    const feature = features[test.name]
    if (feature) {
      if (test.subtests) {
        for (const subtest of test.subtests) {
          const fullName = `${test.name}: ${subtest.name}`
          if (subtestsToSkip[fullName]) continue
          mergeIndividualTestResults(map, feature, fullName, subtest.res)
        }
      } else {
        mergeIndividualTestResults(map, feature, test.name, test.res)
      }
    } else if (test.subtests) {
      for (const subtest of test.subtests) {
        const fullName = `${test.name}: ${subtest.name}`
        if (subtestsToSkip[fullName]) continue
        const feature = features[fullName]
        if (feature) mergeIndividualTestResults(map, feature, fullName, subtest.res)
      }
    }
  }
}

export const js: SupportMap<JSFeature> = {} as SupportMap<JSFeature>
mergeAllTestResults(js, [...es5.tests, ...es6.tests, ...stage4.tests, ...stage1to3.tests])
