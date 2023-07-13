// This file processes data from https://caniuse.com

import lite = require('caniuse-lite')
import { Engine, JSFeature, Support, SupportMap } from './index'

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

export const js: SupportMap<JSFeature> = {} as SupportMap<JSFeature>

for (const feature in jsFeatures) {
  const jsFeature = jsFeatures[feature]
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

  js[jsFeature] = engines
}
