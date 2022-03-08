import type { Plugin } from '../../lib/shared/types';
import fs from 'fs';
// import path from 'path';

let loadPlugin: Plugin = {
  name: 'test_load',
  setup(build) {
    build.onLoad({ filter: /.*/ }, args => {
      if (args.path.indexOf(".tsx")>-1){
        console.log(args.path);
        let content = fs.readFileSync(args.path).toString("utf8");
        content = `console.log(${new Date().getTime()});` + "\n" + content;
        return {
          contents: content,
          loader: "tsx",
        }
      }
      return undefined
    })
  },
}

export { loadPlugin };