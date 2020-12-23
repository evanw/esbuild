# Debug tool 
Debug Tool helps you insert debug code and easily to remove it when deploy to production

### LOG
Similar to console.log but logs also identifiers and source code position
```javascript
// app.js
import { DEBUGTOOL } from './debug'
function sum(a,b) {
  DEBUGTOOL.LOG(a,b);
  → console.log('a='+ a, 'b='+ b, ' at app.js:3:3');
  return a+b;
}
```
### ASSERT
Similar to assert but logs also identifier and source code position
```javascript
// app.js
import { DEBUGTOOL } from './debug'
function sum(a,b) {
  DEBUGTOOL.ASSERT(a>0,b>0)
  → if (!(a>0)) throw new Error('assert a>0 at app.js:3:3');
  → if (!(b>0)) throw new Error('assert b>0 at app.js:3:3');
  return a+b
}
```
### TRACE 
Useful for test local variables on test units. It's not for production mode.
```javascript
// lib.js
import { DEBUGTOOL } from './debug'
function quadraticEquation(a,b,c) {
  const delta = b*b - 4*a*c; ← b² – 4ac
  DEBUGTOOL.TRACE(a, b, c, delta)
  → tracelog.push('a=' +a + ' b=' + b +' c=' + c + ' delta=' + delta);
  if (delta<0) return null
  const x1 = (-b + sqrt(delta)) / (2*a)
  const x2 = (-b - sqrt(delta)) / (2*a)
  return {x1, x2}
}
// test.js
import { DEBUGTOOL } from './debug'
it(()=>{
  DEBUGTOOL.RESET() 
  → tracelog = [] // traceLog is a global
  const result = quadraticEquation(1, 8, -9) // x² + 8x - 9 = 0
  const deltaWasTraced = DEBUGTOOL.ASSERT(/delta=100/} 
  → const deltaWasTraced = traceLog.some(l=>/delta=100/.test(l)))
  except(deltaWasTraced).toBe(true)
  except(DEBUGTOOL.HISTORY()).toBe('a=1 b=8 c=-9 delta=100')
  ...
})
```
## Using with esbuilder

## CLI

In development or test mode
```
./node_modules/.bin/esbuild app.jsx --bundle --debug-tool=DEBUGTOOL --outfile=out.js
```

In production mode
```
./node_modules/.bin/esbuild app.jsx --bundle --remove-debug-tool=DEBUGTOOL --outfile=out.js
```

##### API
In development or test mode
```
require('esbuild').buildSync({
  entryPoints: ['in.ts'],
  outfile: 'out.js',
  debugTool: 'DEBUGTOOL',
})
```

In production mode
```
require('esbuild').buildSync({
  entryPoints: ['in.ts'],
  outfile: 'out.js',
  removeDebugTool: 'DEBUGTOOL',
})
```

### why use debugtoos
- Sugar syntax alternative for `if (process.env.NODE_ENV === 'development')` 
- tests can take benefits from historical debugging
- No globals
- Compatible syntax

### debug.js module
You will need a debug module, like that but customized to your needs.

See samples in https://github.com/thr0w/babel-plugin-debug-tools

```javascript

var traceLog = [];
var DEBUGTOOL = {
  LOG(...args) {
    console.log(...args.slice(1).map((a) => JSON.stringify(a)));
  },
  ASSERT(...args) {
    const loc = args[0];
    for (let i = 1; i < args.length; i++) {
      const arg = args[i];
      if (Array.isArray(arg)) {
        if (!arg[1])
          throw new Error("ASSERT FAIL: " + arg[0] + formatLoc(loc) + (arg[2] ? JSON.stringify(arg[2]) : ""));
      } else if (typeof arg === "string") {
        if (!traceLog.some((l) => l.indexOf(arg) > -1))
          throw new Error("NOT FOUND IN HISTORY: " + arg + formatLoc(loc));
      } else {
        if (!traceLog.some((l) => arg.test(l)))
          throw new Error("NOT FOUND IN HISTORY: " + arg.toString() + " at " + formatLoc(loc));
      }
    }
  },
  RESET() {
    traceLog = [];
  },
  HISTORY() {
    return traceLog.join("\n");
  },
  TRACE(...args) {
    traceLog.push(formatArgs(args.slice(1), 0));
  },
  CHECK(regExp) {
    return traceLog.some((l) => regExp.test(l));
  }
};
function formatLoc(loc) {
  if (loc)
    return " at " + (loc.filename || "") + ":" + loc.line + ":" + loc.column + " ";
  return "";
}
function formatArg(arg) {
  if (typeof arg === "string")
    return arg;
  return JSON.stringify(arg);
}
function formatArgs(args, sLoc) {
  const flatArgs = [];
  for (let i = 0; i < args.length - sLoc; i++) {
    const arg = args[i];
    if (Array.isArray(arg) && arg.length == 2) {
      flatArgs.push(formatArg(arg[0]) + ": " + formatArg(arg[1]));
    } else
      flatArgs.push(formatArg(arg));
  }
  if (sLoc)
    flatArgs.push(formatArg(formatLoc(args[args.length - 1])));
  return flatArgs.join(" ");
}

```
