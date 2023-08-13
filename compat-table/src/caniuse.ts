// This file processes data from https://caniuse.com

import lite = require('caniuse-lite')
import { CSSFeature, CSSPrefixMap, CSSProperty, Engine, JSFeature, PrefixData, Support, SupportMap } from './index'

const enum StatusCode {
  Almost = 'a',
  Disabled = 'd',
  No = 'n',
  Polyfill = 'p',
  Prefix = 'x',
  Unknown = 'u',
  Yes = 'y',
}

const supportedAgents: Record<string, Engine> = {
  chrome: 'Chrome',
  edge: 'Edge',
  firefox: 'Firefox',
  ie: 'IE',
  ios_saf: 'IOS',
  opera: 'Opera',
  safari: 'Safari',
}

const jsFeatures: Record<string, JSFeature> = {
  'es6-module-dynamic-import': 'DynamicImport',
}

const cssFeatures: Record<string, CSSFeature> = {
  'css-matches-pseudo': 'IsPseudoClass',
}

const cssPrefixFeatures: Record<string, CSSProperty> = {
  'css-appearance': 'DAppearance',
  'css-backdrop-filter': 'DBackdropFilter',
  'background-clip-text': 'DBackgroundClip',
  'css-boxdecorationbreak': 'DBoxDecorationBreak',
  'css-clip-path': 'DClipPath',
  'font-kerning': 'DFontKerning',
  'css-hyphens': 'DHyphens',
  'css-initial-letter': 'DInitialLetter',
  'css-sticky': 'DPosition',
  'css-color-adjust': 'DPrintColorAdjust',
  'css3-tabsize': 'DTabSize',
  'css-text-orientation': 'DTextOrientation',
  'text-size-adjust': 'DTextSizeAdjust',
}

export const js: SupportMap<JSFeature> = {} as SupportMap<JSFeature>
export const css: SupportMap<CSSFeature> = {} as SupportMap<CSSFeature>
export const cssPrefix: CSSPrefixMap = {}

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

const addFeatures = <F extends string>(map: SupportMap<F>, features: Record<string, F>): void => {
  for (const feature in features) {
    const engines: Partial<Record<Engine, Record<string, Support>>> = {}
    const entry = lite.feature(lite.features[feature])

    for (const agent in entry.stats) {
      const engine = supportedAgents[agent]
      if (!engine) continue

      const versionRanges = entry.stats[agent]
      const versions: Record<string, Support> = {}

      for (const versionRange in versionRanges) {
        const statusCodes = versionRanges[versionRange].split(' ')
        const isSupported = statusCodes.includes(StatusCode.Yes)

        for (const version of versionRange.split('-')) {
          if (/^\d+(?:\.\d+(?:\.\d+)?)?$/.test(version)) {
            versions[version] = { force: isSupported }
          }
        }
      }

      engines[engine] = versions
    }

    map[features[feature]] = engines
  }
}

addFeatures(js, jsFeatures)
addFeatures(css, cssFeatures)

for (const feature in cssPrefixFeatures) {
  const prefixData: PrefixData[] = []
  const entry = lite.feature(lite.features[feature])

  for (const agent in entry.stats) {
    const engine = supportedAgents[agent]
    if (!engine) continue

    const model = lite.agents[agent]!
    const versionRanges = entry.stats[agent]
    const sortedVersions: { version: string, prefix: string | null }[] = []
    const prefixes = new Set<string>()

    for (const versionRange in versionRanges) {
      const statusCodes = versionRanges[versionRange].split(' ')
      const prefix = statusCodes.includes(StatusCode.Prefix)
        ? (model.prefix_exceptions && model.prefix_exceptions[versionRange]) || model.prefix
        : null
      for (const version of versionRange.split('-')) {
        sortedVersions.push({ version, prefix })
      }
      if (prefix !== null) {
        prefixes.add(prefix)
      }
    }

    sortedVersions.sort((a, b) => compareVersions(a.version, b.version))

    for (const prefix of prefixes) {
      // Find the version after the latest version that requires the prefix (if there even is one)
      let i = sortedVersions.length
      while (i > 0 && sortedVersions[i - 1].prefix !== prefix) {
        i--
      }

      // Add an entry for this prefix combination
      const result: PrefixData = { engine, prefix }
      if (i < sortedVersions.length) {
        result.withoutPrefix = sortedVersions[i].version.split('.').map(x => +x)
      }
      prefixData.push(result)
    }
  }

  cssPrefix[cssPrefixFeatures[feature]] = prefixData
}
