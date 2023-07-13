// This file processes data from https://developer.mozilla.org/en-US/docs/Web

import bcd from '@mdn/browser-compat-data'
import { Engine, JSFeature, Support, SupportMap } from './index'

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

const jsFeatures: Record<string, JSFeature> = {
  'javascript.builtins.RegExp.hasIndices': 'RegexpMatchIndices',
  'javascript.classes.static_initialization_blocks': 'ClassStaticBlocks',
  'javascript.operators.await.top_level': 'TopLevelAwait',
  'javascript.operators.import_meta': 'ImportMeta',
  'javascript.statements.export.namespace': 'ExportStarAs',
  'javascript.statements.import.import_assertions': 'ImportAssertions',
}

export const js: SupportMap<JSFeature> = {} as SupportMap<JSFeature>

for (const feature in jsFeatures) {
  const jsFeature = jsFeatures[feature]
  const engines: Partial<Record<Engine, Record<string, Support>>> = {}
  let object: any = bcd

  // Traverse the JSON object to find the data
  for (const key of feature.split('.')) {
    object = object[key]
  }

  const support = object.__compat.support

  for (const env in support) {
    const engine = supportedEnvironments[env]

    if (engine) {
      const entries = support[env]

      for (const { flags, version_added, version_removed } of Array.isArray(entries) ? entries : [entries]) {
        if (flags && flags.length > 0) {
          // The feature isn't considered to be supported if it requires a flag
          continue
        }
        if (version_added && !version_removed && /^\d+(?:\.\d+(?:\.\d+)?)?$/.test(version_added)) {
          engines[engine] = { [version_added]: { force: true } }
        }
      }
    }
  }

  js[jsFeature] = engines
}
