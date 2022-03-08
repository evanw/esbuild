
import type { Plugin } from '../../lib/shared/types';


let resolvePlugin: Plugin = {
  name: 'test_resolve',
  setup(build) {
    build.onResolve({ filter: /.*/ }, args => ({
      path: args.path,
    }))
  },
}

export { resolvePlugin };