package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var transpile_suite = suite{
	name: "transpile",
}

func TestNoRemoveConsole(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				console.log(a)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRemoveConsole(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				console.log(a)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			RemoveConsole: true,
			AbsOutputFile: "/out.js",
		},
	})
}
func TestNoRemoveDebbuger(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				debugger;
				console.log(a)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRemoveDebbuger(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				debugger;
				console.log(a);
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:           config.ModeBundle,
			RemoveDebugger: true,
			AbsOutputFile:  "/out.js",
		},
	})
}

func TestDebugToolsBhaskara(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/bhaskara.js": `
			import { BHASKARA } from './debug'

			export function quadraticEquation(a, b, c) {
				BHASKARA.ASSERT(a !== 0)
				const delta = b * b - 4 * a * c; // b² – 4ac
				BHASKARA.TRACE(a, b, c, delta)
				if (delta < 0) return null
				const x1 = (-b + Math.sqrt(delta)) / (2 * a)
				const x2 = (-b - Math.sqrt(delta)) / (2 * a)
				BHASKARA.TRACE(x1, x2)
				return { x1, x2 }
			}
			`,
			"/bhaskara.test.js": `
			import { quadraticEquation } from './bhaskara'
			import { BHASKARA } from './debug'
			
			test('x² + 8x - 9 = 0', () => {
				BHASKARA.RESET()
				BHASKARA.ASSERT(typeof quadraticEquation === 'function')
				const { x1, x2 } = quadraticEquation(1, 8, -9)
				BHASKARA.ASSERT(/delta: 100/, x1 === 1, x2 === -9)
				expect(BHASKARA.HISTORY()).toEqual('a: 1 b: 8 c: -9 delta: 100\nx1: 1 x2: -9')
			});			
			`,
			"/debug.js": `
			let traceLog = [];

			export const BHASKARA = {
				LOG(...args) {
					console.log(...args.slice(1).map( (a)=>JSON.stringify(a)));
				},
				ASSERT(...args) {
					const loc = args[0]
					for (let i = 1; i < args.length; i++) {
						const arg = args[i]
						if (Array.isArray(arg)) {
							if (!arg[1]) throw new Error(
								'ASSERT FAIL: ' + arg[0] + formatLoc(loc) +
								(arg[2] ? JSON.stringify(arg[2]) : '')
							);
						} else if (typeof arg === 'string') {
							if (!traceLog.some(l => l.indexOf(arg) > -1))
								throw new Error('NOT FOUND IN HISTORY: ' + arg + formatLoc(loc))
						} else {
							if (!traceLog.some(l => arg.test(l)))
								throw new Error('NOT FOUND IN HISTORY: ' + arg.toString() + ' at ' + formatLoc(loc))
						}
					}
				},
				RESET() {
					traceLog = [];
				},
				HISTORY() {
					return traceLog.join('\n')
				},
				TRACE(...args) {
					traceLog.push(formatArgs(args.slice(1), 0));
				},
				CHECK(regExp) {
					return traceLog.some(l => regExp.test(l))
				}
			};
			
			function formatLoc(loc) {
				if (loc)
					return ' at ' + (loc.filename || '') + ':' + loc.line + ':' + loc.column + ' ';
				return ''
			}
			function formatArg(arg) {
				if (typeof arg === 'string') return arg
				return JSON.stringify(arg)
			}
			function formatArgs(args, sLoc) {
				const flatArgs = []
				for (let i = 0; i < args.length - sLoc; i++) {
					const arg = args[i]
					if (Array.isArray(arg) && arg.length == 2) {
						flatArgs.push(formatArg(arg[0]) + ': ' + formatArg(arg[1]))
					}
					else flatArgs.push(formatArg(arg))
				}
				if (sLoc)
					flatArgs.push(formatArg(formatLoc(args[args.length - 1])))
				return flatArgs.join(' ')
			}
			`,
		},
		entryPaths: []string{"/bhaskara.test.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			DebugTool:     "BHASKARA",
			AbsOutputFile: "/out.js",
		},
	})
}
