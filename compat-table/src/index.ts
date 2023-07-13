// Run "make compat-table" to run this code

import child_process = require('child_process')
import fs = require('fs')
import path = require('path')
import { generateTableForJS } from './js_table'
import * as caniuse from './caniuse'
import * as mdn from './mdn'

export type Engine = keyof typeof engines
export const engines = {
  Chrome: true,
  Deno: true,
  Edge: true,
  ES: true,
  Firefox: true,
  Hermes: true,
  IE: true,
  IOS: true,
  Node: true,
  Opera: true,
  Rhino: true,
  Safari: true,
}

export type JSFeature = keyof typeof jsFeatures
export const jsFeatures = {
  ArbitraryModuleNamespaceNames: true,
  ArraySpread: true,
  Arrow: true,
  AsyncAwait: true,
  AsyncGenerator: true,
  Bigint: true,
  Class: true,
  ClassField: true,
  ClassPrivateAccessor: true,
  ClassPrivateBrandCheck: true,
  ClassPrivateField: true,
  ClassPrivateMethod: true,
  ClassPrivateStaticAccessor: true,
  ClassPrivateStaticField: true,
  ClassPrivateStaticMethod: true,
  ClassStaticBlocks: true,
  ClassStaticField: true,
  ConstAndLet: true,
  Decorators: true,
  DefaultArgument: true,
  Destructuring: true,
  DynamicImport: true,
  ExponentOperator: true,
  ExportStarAs: true,
  ForAwait: true,
  ForOf: true,
  FunctionOrClassPropertyAccess: true,
  Generator: true,
  Hashbang: true,
  ImportAssertions: true,
  ImportMeta: true,
  InlineScript: true,
  LogicalAssignment: true,
  NestedRestBinding: true,
  NewTarget: true,
  NodeColonPrefixImport: true,
  NodeColonPrefixRequire: true,
  NullishCoalescing: true,
  ObjectAccessors: true,
  ObjectExtensions: true,
  ObjectRestSpread: true,
  OptionalCatchBinding: true,
  OptionalChain: true,
  RegexpDotAllFlag: true,
  RegexpLookbehindAssertions: true,
  RegexpMatchIndices: true,
  RegexpNamedCaptureGroups: true,
  RegexpSetNotation: true,
  RegexpStickyAndUnicodeFlags: true,
  RegexpUnicodePropertyEscapes: true,
  RestArgument: true,
  TemplateLiteral: true,
  TopLevelAwait: true,
  TypeofExoticObjectIsObject: true,
  UnicodeEscapes: true,
  Using: true,
}

export interface Support {
  force?: boolean
  passed?: number
  failed?: number
}

export interface VersionRange {
  start: number[]
  end?: number[]
}

export type SupportMap<F extends string> = Record<F, Partial<Record<Engine, Record<string, Support>>>>
export type VersionRangeMap<F extends string> = Partial<Record<F, Partial<Record<Engine, VersionRange[]>>>>

const compareVersions = (a: number[], b: number[]): number => {
  let diff = a[0] - b[0]
  if (!diff) {
    diff = (a[1] || 0) - (b[1] || 0)
    if (!diff) {
      diff = (a[2] || 0) - (b[2] || 0)
    }
  }
  return diff
}

const mergeSupportMaps = <F extends string>(to: SupportMap<F>, from: SupportMap<F>): void => {
  for (const feature in from) {
    const fromEngines = from[feature as F]
    const toEngines = to[feature as F] || (to[feature as F] = {})

    for (const engine in fromEngines) {
      const fromVersions = fromEngines[engine as Engine]
      const toVersions = toEngines[engine as Engine] || (toEngines[engine as Engine] = {})

      for (const version in fromVersions) {
        if (version in toVersions) {
          throw new Error(`Merge conflict with feature=${feature} engine=${engine} version=${version}`)
        }

        toVersions[version] = fromVersions[version]
      }
    }
  }
}

const supportMapToVersionRanges = <F extends string>(supportMap: SupportMap<F>): VersionRangeMap<F> => {
  const versionRangeMap: VersionRangeMap<F> = {}

  for (const feature in supportMap) {
    const engines = supportMap[feature as F]
    const featureMap: Partial<Record<Engine, VersionRange[]>> = {}

    // Compute the maximum number of tests that any one engine has passed
    let maxPassed = 0
    for (const engine in engines) {
      const versions = engines[engine as Engine]
      for (const version in versions) {
        const { passed } = versions[version]
        if (passed && passed > maxPassed) maxPassed = passed
      }
    }

    for (const engine in engines) {
      const versions = engines[engine as Engine]
      const sortedVersions: { version: number[], supported: boolean }[] = []

      for (const version in versions) {
        const { force, passed, failed } = versions[version]
        const parsed = version.split('.').map(x => +x)
        sortedVersions.push({
          version: parsed,
          supported: force !== void 0 ? force :
            // If no test failed but less than the maximum number of tests passed,
            // that means we have partial data (some tests have never been run for
            // those versions). This happens for really old browser versions that
            // people can't even run anymore. We conservatively consider this
            // feature to be unsupported if not all tests were run, since it could
            // be dangerous to assume otherwise.
            !failed && passed === maxPassed,
        })
      }

      sortedVersions.sort((a, b) => compareVersions(a.version, b.version))

      const versionRanges: VersionRange[] = []
      let i = 0

      while (i < sortedVersions.length) {
        const { version, supported } = sortedVersions[i++]
        if (supported) {
          while (i < sortedVersions.length && sortedVersions[i].supported) {
            i++
          }
          const range: VersionRange = { start: version }
          if (i < sortedVersions.length) range.end = sortedVersions[i].version
          versionRanges.push(range)
        }
      }

      // The target is typically used to mean "make sure it works in this
      // version and later". So we just take the last version range here.
      //
      // However, we make an exception for node since people sometimes use
      // the target to build for the version of node that they currently
      // have. Node has a discontiguous version range for the support of
      // several features that people want to use.
      if (versionRanges.length && engine as Engine !== 'Node') {
        if (versionRanges[versionRanges.length - 1].end) {
          // We say this engine doesn't support this feature at all if
          // the feature is broken in the latest version of this engine.
          // This sometimes happens when engines deliberately decide to
          // not implement a feature of the language (e.g. Hermes).
          continue
        }

        // Otherwise, only consider this feature to be supported for the
        // last version range (i.e. the earliest version for which all
        // later versions support this feature). Delete all version ranges
        // before the last version range.
        versionRanges.splice(0, versionRanges.length - 1)
      }

      if (versionRanges.length) {
        featureMap[engine as Engine] = versionRanges
      }
    }

    versionRangeMap[feature as F] = featureMap
  }

  return versionRangeMap
}

const updateGithubDependencies = (): void => {
  const jsonPath = path.join(__dirname, 'package.json')
  const jsonData = JSON.parse(fs.readFileSync(jsonPath, 'utf8'))

  Object.keys(jsonData.githubDependencies).forEach(repo => {
    const fullPath = path.join(__dirname, 'repos', repo)
    if (!fs.existsSync(fullPath)) {
      fs.mkdirSync(fullPath, { recursive: true })
      child_process.execFileSync('git', ['clone', '-b', 'gh-pages', `https://github.com/${repo}.git`, fullPath], { cwd: fullPath, stdio: 'inherit' })
    }

    child_process.execFileSync('git', ['fetch'], { cwd: fullPath, stdio: 'inherit' })
    child_process.execFileSync('git', ['reset', '--hard', '--quiet', 'origin/gh-pages'], { cwd: fullPath, stdio: 'inherit' })

    const commit = child_process.execFileSync('git', ['rev-parse', 'HEAD'], { cwd: fullPath }).toString().trim()
    jsonData.githubDependencies[repo] = commit
  })

  fs.writeFileSync(jsonPath, JSON.stringify(jsonData, null, 2) + '\n')
}

updateGithubDependencies()

import('./kangax').then(({ js }) => {
  // Merge data from https://caniuse.com into the JavaScript support map
  mergeSupportMaps(js, caniuse.js)

  // Merge data from the Mozilla Developer Network into the JavaScript support map
  mergeSupportMaps(js, mdn.js)

  // ES5 features
  js.ObjectAccessors.ES = { 5: { force: true } }
  js.ObjectAccessors.Node = { '0.4': { force: true } } // "node-compat-table" doesn't appear to cover ES5 features...

  // ES6/ES2015 features
  js.ArraySpread.ES = { 2015: { force: true } }
  js.Arrow.ES = { 2015: { force: true } }
  js.Class.ES = { 2015: { force: true } }
  js.ConstAndLet.ES = { 2015: { force: true } }
  js.DefaultArgument.ES = { 2015: { force: true } }
  js.Destructuring.ES = { 2015: { force: true } }
  js.DynamicImport.ES = { 2015: { force: true } }
  js.ForOf.ES = { 2015: { force: true } }
  js.Generator.ES = { 2015: { force: true } }
  js.NewTarget.ES = { 2015: { force: true } }
  js.ObjectExtensions.ES = { 2015: { force: true } }
  js.RegexpStickyAndUnicodeFlags.ES = { 2015: { force: true } }
  js.RestArgument.ES = { 2015: { force: true } }
  js.TemplateLiteral.ES = { 2015: { force: true } }
  js.UnicodeEscapes.ES = { 2015: { force: true } }

  // ES2016 features
  js.ExponentOperator.ES = { 2016: { force: true } }
  js.NestedRestBinding.ES = { 2016: { force: true } }

  // ES2017 features
  js.AsyncAwait.ES = { 2017: { force: true } }

  // ES2018 features
  js.AsyncGenerator.ES = { 2018: { force: true } }
  js.ForAwait.ES = { 2018: { force: true } }
  js.ObjectRestSpread.ES = { 2018: { force: true } }
  js.RegexpDotAllFlag.ES = { 2018: { force: true } }
  js.RegexpLookbehindAssertions.ES = { 2018: { force: true } }
  js.RegexpNamedCaptureGroups.ES = { 2018: { force: true } }
  js.RegexpUnicodePropertyEscapes.ES = { 2018: { force: true } }

  // ES2019 features
  js.OptionalCatchBinding.ES = { 2019: { force: true } }

  // ES2020 features
  js.Bigint.ES = { 2020: { force: true } }
  js.ExportStarAs.ES = { 2020: { force: true } }
  js.ImportMeta.ES = { 2020: { force: true } }
  js.NullishCoalescing.ES = { 2020: { force: true } }
  js.OptionalChain.ES = { 2020: { force: true } }
  js.TypeofExoticObjectIsObject.ES = { 2020: { force: true } } // https://github.com/tc39/ecma262/pull/1441

  // ES2021 features
  js.LogicalAssignment.ES = { 2021: { force: true } }

  // ES2022 features
  js.ClassField.ES = { 2022: { force: true } }
  js.ClassPrivateAccessor.ES = { 2022: { force: true } }
  js.ClassPrivateBrandCheck.ES = { 2022: { force: true } }
  js.ClassPrivateField.ES = { 2022: { force: true } }
  js.ClassPrivateMethod.ES = { 2022: { force: true } }
  js.ClassPrivateStaticAccessor.ES = { 2022: { force: true } }
  js.ClassPrivateStaticField.ES = { 2022: { force: true } }
  js.ClassPrivateStaticMethod.ES = { 2022: { force: true } }
  js.ClassStaticBlocks.ES = { 2022: { force: true } }
  js.ClassStaticField.ES = { 2022: { force: true } }
  js.TopLevelAwait.ES = { 2022: { force: true } }
  js.ArbitraryModuleNamespaceNames.ES = { 2022: { force: true } }
  js.RegexpMatchIndices.ES = { 2022: { force: true } }

  // This is a problem specific to Internet Explorer. See https://github.com/tc39/ecma262/issues/1440
  for (const engine in engines) {
    if (engine as Engine !== 'ES' && engine as Engine !== 'IE') {
      js.TypeofExoticObjectIsObject[engine as Engine] = { 0: { force: true } }
    }
  }

  // This is a problem specific to JavaScriptCore. Some examples of when the
  // problematic case happens (checked in Safari 12.1):
  //
  //   ❱ x(function(y=-1){}.z=2)
  //   SyntaxError: Left hand side of operator '=' must be a reference.
  //
  //   ❱ x(class{f(y=-1){}}.z=2)
  //   SyntaxError: Left hand side of operator '=' must be a reference.
  //
  // Some examples of cases that aren't problematic (checked in Safari 12.1):
  //
  //   // Adding parentheses makes it ok
  //   x((function(y=-1){}).z=2)
  //   x((class{f(y=-1){}}).z=2)
  //
  //   // Not using a unary operator in the default argument makes it ok
  //   x(function(y=1){}.z=2)
  //   x(class{f(y=1){}}.z=2)
  //
  //   // Methods in object literals are not affected
  //   x({f(y=-1){}}.z=2)
  //
  // We don't attempt to reverse-engineer the specific conditions that cause JSC
  // to exhibit the bug. Instead we just always wrap function and class literals
  // when they are nested inside of a property access. This workaround is overly
  // conservative but is the same thing that UglifyJS does to handle this case.
  //
  // See https://github.com/mishoo/UglifyJS/pull/2056 and https://github.com/evanw/esbuild/issues/3072
  for (const engine in engines) {
    if (engine as Engine !== 'Safari') {
      js.FunctionOrClassPropertyAccess[engine as Engine] = { 0: { force: true } }
    } else {
      // These bugs are known to be fixed in Safari 16.3+
      js.FunctionOrClassPropertyAccess.Safari = { '16.3': { force: true } }
    }
  }

  // This is a special case. Node added support for it to both v12.20+ and v13.2+
  // so the range is inconveniently discontiguous. Sources:
  //
  // - https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/import
  // - https://github.com/nodejs/node/pull/35950
  // - https://github.com/nodejs/node/pull/31974
  //
  js.DynamicImport.Node = {
    '12.20': { force: true },
    '13': { force: false },
    '13.2': { force: true },
  }

  // Manually copied from https://nodejs.org/api/esm.html#node-imports
  js.NodeColonPrefixImport.Node = {
    '12.20': { force: true },
    '13': { force: false },
    '14.13.1': { force: true },
  }
  js.NodeColonPrefixRequire.Node = {
    '14.18': { force: true },
    '15': { force: false },
    '16': { force: true },
  }

  // Arbitrary Module Namespace Names
  {
    // From https://github.com/tc39/ecma262/pull/2154#issuecomment-825201030
    js.ArbitraryModuleNamespaceNames.Chrome = { 90: { force: true } }
    js.ArbitraryModuleNamespaceNames.Node = { 16: { force: true } }

    // From https://bugzilla.mozilla.org/show_bug.cgi?id=1670044
    js.ArbitraryModuleNamespaceNames.Firefox = { 87: { force: true } }

    // This feature has been implemented in Safari but I have no idea what version
    // this bug corresponds to: https://bugs.webkit.org/show_bug.cgi?id=217576
  }

  // Import assertions (note: these were removed from the JavaScript specification and never standardized)
  {
    // From https://github.com/nodejs/node/blob/master/doc/changelogs/CHANGELOG_V16.md#16.14.0
    js.ImportAssertions.Node = { '16.14': { force: true } }

    // MDN data is wrong here: https://bugs.webkit.org/show_bug.cgi?id=251600
    delete js.ImportAssertions.IOS
    delete js.ImportAssertions.Safari
  }

  // MDN data is wrong here: https://www.chromestatus.com/feature/6482797915013120
  js.ClassStaticBlocks.Chrome = { 91: { force: true } }

  const jsVersionRanges = supportMapToVersionRanges(js)
  generateTableForJS(jsVersionRanges)
})
