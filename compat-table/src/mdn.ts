// This file processes data from https://developer.mozilla.org/en-US/docs/Web

import bcd, { BrowserName, SupportBlock } from '@mdn/browser-compat-data'
import { CSSFeature, CSSPrefixMap, CSSProperty, Engine, JSFeature, PrefixData, Support, SupportMap } from './index'

const supportedEnvironments: Record<string, Engine> = {
  chrome: 'Chrome',
  deno: 'Deno',
  edge: 'Edge',
  firefox: 'Firefox',
  ie: 'IE',
  nodejs: 'Node',
  opera: 'Opera',
  safari: 'Safari',
  safari_ios: 'IOS',
}

const jsFeatures: Partial<Record<JSFeature, string>> = {
  RegexpMatchIndices: 'javascript.builtins.RegExp.hasIndices',
  ClassStaticBlocks: 'javascript.classes.static_initialization_blocks',
  TopLevelAwait: 'javascript.operators.await.top_level',
  ImportMeta: 'javascript.operators.import_meta',
  ExportStarAs: 'javascript.statements.export.namespace',
  ImportAssertions: 'javascript.statements.import.import_assertions',
}

const cssFeatures: Partial<Record<CSSFeature, string | string[]>> = {
  InsetProperty: 'css.properties.inset',
  RebeccaPurple: 'css.types.color.named-color.rebeccapurple',
  HexRGBA: 'css.types.color.rgb_hexadecimal_notation.alpha_hexadecimal_notation',
  Modern_RGB_HSL: [
    'css.types.color.hsl.alpha_parameter',
    'css.types.color.hsl.space_separated_parameters',
    'css.types.color.hsla.space_separated_parameters',
    'css.types.color.rgb.alpha_parameter',
    'css.types.color.rgb.float_values',
    'css.types.color.rgb.space_separated_parameters',
    'css.types.color.rgba.float_values',
    'css.types.color.rgba.space_separated_parameters',
  ],
}

const cssPrefixFeatures: Record<string, CSSProperty> = {
  'css.properties.mask-image': 'DMaskImage',
  'css.properties.mask-origin': 'DMaskOrigin',
  'css.properties.mask-position': 'DMaskPosition',
  'css.properties.mask-repeat': 'DMaskRepeat',
  'css.properties.mask-size': 'DMaskSize',
  'css.properties.text-decoration-color': 'DTextDecorationColor',
  'css.properties.text-decoration-line': 'DTextDecorationLine',
  'css.properties.text-decoration-skip': 'DTextDecorationSkip',
  'css.properties.text-emphasis-color': 'DTextEmphasisColor',
  'css.properties.text-emphasis-position': 'DTextEmphasisPosition',
  'css.properties.text-emphasis-style': 'DTextEmphasisStyle',
  'css.properties.user-select': 'DUserSelect',
}

export const js: SupportMap<JSFeature> = {} as SupportMap<JSFeature>
export const css: SupportMap<CSSFeature> = {} as SupportMap<CSSFeature>
export const cssPrefix: CSSPrefixMap = {}

const isSemver = /^\d+(?:\.\d+(?:\.\d+)?)?$/

const compareVersions = (aStr: string, bStr: string): number => {
  const a = aStr.split('.')
  const b = bStr.split('.')
  let diff = +a[0] - +b[0]
  if (diff === 0) {
    diff = +(a[1] || '0') - +(b[1] || '0')
    if (diff === 0) {
      diff = +(a[2] || '0') - +(b[2] || '0')
    }
  }
  return diff
}

const extractProperty = (object: any, fullKey: string): any => {
  for (const key of fullKey.split('.')) {
    object = object[key]
  }
  return object
}

const addFeatures = <F extends string>(map: SupportMap<F>, features: Partial<Record<F, string | string[]>>): void => {
  for (const feature in features) {
    const keys = features[feature]
    const maxVersions: Partial<Record<Engine, { version: string, isSupported: boolean }>> = {}

    for (const fullKey of Array.isArray(keys) ? keys : [keys]) {
      const support: SupportBlock = extractProperty(bcd, fullKey).__compat.support

      for (const env in support) {
        const engine = supportedEnvironments[env]

        if (engine) {
          const entries = support[env as BrowserName]!

          for (const { flags, version_added, version_removed } of Array.isArray(entries) ? entries : [entries]) {
            if (typeof version_added === 'string' && isSemver.test(version_added)) {
              // The feature isn't considered to be supported if it was removed or if it requires a flag
              const isSupported = !version_removed || !flags
              const maxVersion = maxVersions[engine]
              if (
                !maxVersion ||
                compareVersions(version_added, maxVersion.version) > 0 ||
                (compareVersions(version_added, maxVersion.version) === 0 && !isSupported)
              ) {
                maxVersions[engine] = { version: version_added, isSupported }
              }
            }
          }
        }
      }
    }

    const engines: Partial<Record<Engine, Record<string, Support>>> = {}
    for (const engine in maxVersions) {
      const { version, isSupported } = maxVersions[engine as Engine]!
      engines[engine as Engine] = { [version]: { force: isSupported } }
    }
    map[feature] = engines
  }
}

addFeatures(js, jsFeatures)
addFeatures(css, cssFeatures)

for (const fullKey in cssPrefixFeatures) {
  const prefixData: PrefixData[] = []
  const support: SupportBlock = extractProperty(bcd, fullKey).__compat.support

  for (const env in support) {
    const engine = supportedEnvironments[env]

    if (engine) {
      let entries = support[env as BrowserName]!
      if (!Array.isArray(entries)) entries = [entries]

      // Figure out which version this property can be used unprefixed, if any.
      // This assumes that support for these CSS properties is never removed.
      let version_unprefixed: string | undefined
      for (const { prefix, flags, version_added, version_removed } of entries) {
        if (!prefix && !flags && typeof version_added === 'string' && !version_removed && isSemver.test(version_added)) {
          version_unprefixed = version_added
        }
      }

      type PrefixRange = { prefix: string, start: string, end?: string }
      const ranges: PrefixRange[] = []

      // Find all version ranges where a given prefix is supported
      for (let i = 0; i < entries.length; i++) {
        const { prefix, flags, version_added, version_removed } = entries[i]

        if (prefix && !flags && typeof version_added === 'string' && isSemver.test(version_added)) {
          const range: PrefixRange = { prefix, start: version_added }
          let withoutPrefix: string | undefined

          // The prefix is no longer needed if support for the feature was removed
          if (typeof version_removed === 'string' && isSemver.test(version_removed)) {
            withoutPrefix = version_removed
          }

          // The prefix is no longer needed if it can be used unprefixed
          if (version_unprefixed && (!withoutPrefix || compareVersions(version_unprefixed, withoutPrefix) < 0)) {
            withoutPrefix = version_unprefixed
          }

          if (withoutPrefix) {
            if (compareVersions(version_added, withoutPrefix) === 0) {
              // No prefix is needed if support for the property with and without the prefix was added simultaneously
              continue
            }
            range.end = withoutPrefix
          }

          ranges.push(range)
        }
      }

      // Sort earlier versions first, then sort prefixes for equal versions lexicographically
      ranges.sort((a, b) => compareVersions(a.start, b.start) || +(a.prefix > b.prefix) - +(a.prefix < b.prefix))

      for (let i = 0; i < ranges.length; i++) {
        const { prefix, start, end } = ranges[i]

        // Skip this prefix if it's entirely covered by the previous prefix.
        // Sometimes engines add support for multiple prefixes at a time. For
        // example, in version 12 Edge added support for both "-ms-user-select"
        // and "-webkit-user-select", so we don't need to generate both.
        if (i > 0) {
          const prev = ranges[i - 1]
          if (compareVersions(start, prev.start) >= 0 && (!prev.end || (end && compareVersions(end, prev.end) <= 0))) {
            continue
          }
        }

        const data: PrefixData = { engine, prefix: prefix.replace(/^-|-$/g, '') }
        if (end) {
          data.withoutPrefix = end.split('.').map((x: string) => +x)
        }
        prefixData.push(data)
      }
    }
  }

  cssPrefix[cssPrefixFeatures[fullKey]] = prefixData
}
