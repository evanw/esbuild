package parser

import (
	"math"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
)

var processedGlobals *ProcessedDefines
var knownGlobals = [][]string{
	// Object: Static methods
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Object#Static_methods
	{"Object", "assign"},
	{"Object", "create"},
	{"Object", "defineProperties"},
	{"Object", "defineProperty"},
	{"Object", "entries"},
	{"Object", "freeze"},
	{"Object", "fromEntries"},
	{"Object", "getOwnPropertyDescriptor"},
	{"Object", "getOwnPropertyDescriptors"},
	{"Object", "getOwnPropertyNames"},
	{"Object", "getOwnPropertySymbols"},
	{"Object", "getPrototypeOf"},
	{"Object", "is"},
	{"Object", "isExtensible"},
	{"Object", "isFrozen"},
	{"Object", "isSealed"},
	{"Object", "keys"},
	{"Object", "preventExtensions"},
	{"Object", "seal"},
	{"Object", "setPrototypeOf"},
	{"Object", "values"},

	// Object: Instance methods
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Object#Instance_methods
	{"Object", "prototype", "__defineGetter__"},
	{"Object", "prototype", "__defineSetter__"},
	{"Object", "prototype", "__lookupGetter__"},
	{"Object", "prototype", "__lookupSetter__"},
	{"Object", "prototype", "hasOwnProperty"},
	{"Object", "prototype", "isPrototypeOf"},
	{"Object", "prototype", "propertyIsEnumerable"},
	{"Object", "prototype", "toLocaleString"},
	{"Object", "prototype", "toString"},
	{"Object", "prototype", "unwatch"},
	{"Object", "prototype", "valueOf"},
	{"Object", "prototype", "watch"},

	// Math: Static properties
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Math#Static_properties
	{"Math", "E"},
	{"Math", "LN10"},
	{"Math", "LN2"},
	{"Math", "LOG10E"},
	{"Math", "LOG2E"},
	{"Math", "PI"},
	{"Math", "SQRT1_2"},
	{"Math", "SQRT2"},

	// Math: Static methods
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Math#Static_methods
	{"Math", "abs"},
	{"Math", "acos"},
	{"Math", "acosh"},
	{"Math", "asin"},
	{"Math", "asinh"},
	{"Math", "atan"},
	{"Math", "atan2"},
	{"Math", "atanh"},
	{"Math", "cbrt"},
	{"Math", "ceil"},
	{"Math", "clz32"},
	{"Math", "cos"},
	{"Math", "cosh"},
	{"Math", "exp"},
	{"Math", "expm1"},
	{"Math", "floor"},
	{"Math", "fround"},
	{"Math", "hypot"},
	{"Math", "imul"},
	{"Math", "log"},
	{"Math", "log10"},
	{"Math", "log1p"},
	{"Math", "log2"},
	{"Math", "max"},
	{"Math", "min"},
	{"Math", "pow"},
	{"Math", "random"},
	{"Math", "round"},
	{"Math", "sign"},
	{"Math", "sin"},
	{"Math", "sinh"},
	{"Math", "sqrt"},
	{"Math", "tan"},
	{"Math", "tanh"},
	{"Math", "trunc"},
}

type FindSymbol func(name string) ast.Ref
type DefineFunc func(FindSymbol) ast.E

type DotDefine struct {
	Parts      []string
	DefineFunc DefineFunc

	// This is used to whitelist certain functions that are known to be safe to
	// remove if their result is unused
	CanBeRemovedIfUnused bool
}

type ProcessedDefines struct {
	IdentifierDefines map[string]DefineFunc
	DotDefines        map[string][]DotDefine
}

// This transformation is expensive, so we only want to do it once. Make sure
// to only call processDefines() once per compilation. Unfortunately Golang
// doesn't have an efficient way to copy a map and the overhead of copying
// all of the properties into a new map once for every new parser noticeably
// slows down our benchmarks.
func ProcessDefines(userDefines map[string]DefineFunc) ProcessedDefines {
	// Optimization: reuse known globals if there are no user-specified defines
	hasUserDefines := userDefines == nil || len(userDefines) == 0
	if !hasUserDefines && processedGlobals != nil {
		return *processedGlobals
	}

	result := ProcessedDefines{
		IdentifierDefines: make(map[string]DefineFunc),
		DotDefines:        make(map[string][]DotDefine),
	}

	// Mark these property accesses as free of side effects. That means they can
	// be removed if their result is unused. We can't just remove all unused
	// property accesses since property accesses can have side effects. For
	// example, the property access "a.b.c" has the side effect of throwing an
	// exception if "a.b" is undefined.
	for _, parts := range knownGlobals {
		tail := parts[len(parts)-1]
		result.DotDefines[tail] = append(result.DotDefines[tail], DotDefine{
			Parts:                parts,
			CanBeRemovedIfUnused: true,
		})
	}

	// Swap in certain literal values because those can be constant folded
	result.IdentifierDefines["undefined"] = func(FindSymbol) ast.E { return &ast.EUndefined{} }
	result.IdentifierDefines["NaN"] = func(FindSymbol) ast.E { return &ast.ENumber{math.NaN()} }
	result.IdentifierDefines["Infinity"] = func(FindSymbol) ast.E { return &ast.ENumber{math.Inf(1)} }

	// Then copy the user-specified defines in afterwards, which will overwrite
	// any known globals above.
	for k, v := range userDefines {
		parts := strings.Split(k, ".")
		if len(parts) == 1 {
			result.IdentifierDefines[k] = v
		} else {
			tail := parts[len(parts)-1]
			result.DotDefines[tail] = append(result.DotDefines[tail], DotDefine{
				Parts:      parts,
				DefineFunc: v,
			})
		}
	}

	// Potentially cache the result for next time
	if !hasUserDefines {
		processedGlobals = &result
	}
	return result
}
