package config

import (
	"math"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/logging"
)

var processedGlobalsMutex sync.Mutex
var processedGlobals *ProcessedDefines

var knownGlobals = [][]string{
	// These global identifiers should exist in all JavaScript environments
	{"Array"},
	{"Boolean"},
	{"Function"},
	{"Math"},
	{"Number"},
	{"Object"},
	{"RegExp"},
	{"String"},

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

type FindSymbol func(logging.Loc, string) ast.Ref
type DefineFunc func(logging.Loc, FindSymbol) ast.E

type DefineData struct {
	DefineFunc DefineFunc

	// True if a call to this value is known to not have any side effects. For
	// example, a bare call to "Object()" can be removed because it does not
	// have any observable side effects.
	CallCanBeUnwrappedIfUnused bool
}

func mergeDefineData(old DefineData, new DefineData) DefineData {
	if old.CallCanBeUnwrappedIfUnused {
		new.CallCanBeUnwrappedIfUnused = true
	}
	return new
}

type DotDefine struct {
	Parts []string
	Data  DefineData
}

type ProcessedDefines struct {
	IdentifierDefines map[string]DefineData
	DotDefines        map[string][]DotDefine
}

// This transformation is expensive, so we only want to do it once. Make sure
// to only call processDefines() once per compilation. Unfortunately Golang
// doesn't have an efficient way to copy a map and the overhead of copying
// all of the properties into a new map once for every new parser noticeably
// slows down our benchmarks.
func ProcessDefines(userDefines map[string]DefineData) ProcessedDefines {
	// Optimization: reuse known globals if there are no user-specified defines
	hasUserDefines := len(userDefines) != 0
	if !hasUserDefines {
		processedGlobalsMutex.Lock()
		if processedGlobals != nil {
			defer processedGlobalsMutex.Unlock()
			return *processedGlobals
		}
		processedGlobalsMutex.Unlock()
	}

	result := ProcessedDefines{
		IdentifierDefines: make(map[string]DefineData),
		DotDefines:        make(map[string][]DotDefine),
	}

	// Mark these property accesses as free of side effects. That means they can
	// be removed if their result is unused. We can't just remove all unused
	// property accesses since property accesses can have side effects. For
	// example, the property access "a.b.c" has the side effect of throwing an
	// exception if "a.b" is undefined.
	for _, parts := range knownGlobals {
		tail := parts[len(parts)-1]
		if len(parts) == 1 {
			result.IdentifierDefines[tail] = DefineData{}
		} else {
			result.DotDefines[tail] = append(result.DotDefines[tail], DotDefine{Parts: parts})
		}
	}

	// Swap in certain literal values because those can be constant folded
	result.IdentifierDefines["undefined"] = DefineData{
		DefineFunc: func(logging.Loc, FindSymbol) ast.E { return &ast.EUndefined{} },
	}
	result.IdentifierDefines["NaN"] = DefineData{
		DefineFunc: func(logging.Loc, FindSymbol) ast.E { return &ast.ENumber{Value: math.NaN()} },
	}
	result.IdentifierDefines["Infinity"] = DefineData{
		DefineFunc: func(logging.Loc, FindSymbol) ast.E { return &ast.ENumber{Value: math.Inf(1)} },
	}

	// Then copy the user-specified defines in afterwards, which will overwrite
	// any known globals above.
	for key, data := range userDefines {
		parts := strings.Split(key, ".")

		// Identifier defines are special-cased
		if len(parts) == 1 {
			result.IdentifierDefines[key] = mergeDefineData(result.IdentifierDefines[key], data)
			continue
		}

		tail := parts[len(parts)-1]
		dotDefines := result.DotDefines[tail]
		found := false

		// Try to merge with existing dot defines first
		for i, define := range dotDefines {
			if arePartsEqual(parts, define.Parts) {
				define := &dotDefines[i]
				define.Data = mergeDefineData(define.Data, data)
				found = true
				break
			}
		}

		if !found {
			dotDefines = append(dotDefines, DotDefine{Parts: parts, Data: data})
		}
		result.DotDefines[tail] = dotDefines
	}

	// Potentially cache the result for next time
	if !hasUserDefines {
		processedGlobalsMutex.Lock()
		defer processedGlobalsMutex.Unlock()
		if processedGlobals == nil {
			processedGlobals = &result
		}
	}
	return result
}

func arePartsEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
