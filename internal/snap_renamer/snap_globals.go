package snap_renamer

import (
	"fmt"

	"github.com/evanw/esbuild/internal/js_ast"
)

// globals derived from electron-link blueprint declarations
// See: https://github.com/atom/electron-link/blob/abeb97d8633c06ac6a762ac427b272adebd32c4f/src/blueprint.js#L6
// Also related to: internal/resolver/resolver.go :1246 (BuiltInNodeModules)
var snapGlobals = []string{"process", "document", "global", "window", "console"}

type GlobalSymbols struct {
	process  js_ast.Symbol
	document js_ast.Symbol
	global   js_ast.Symbol
	window   js_ast.Symbol
	console  js_ast.Symbol
}

func getGlobalSymbols(symbols *js_ast.SymbolMap) GlobalSymbols {
	// TODO(thlorenz): even this is not causing any issues (verified) it still is wasteful to perform this
	// step each time a Renamer is created. However we cannot make it static in case that esbuild
	// will run as a service in the future. In that case multiple bundles with
	// different symbol setups would be created in the same process .
	globalSymbols := GlobalSymbols{}
	for _, outer := range symbols.Outer {
		for _, ref := range outer {
			// Globals aren't declared anywhere and thus are unbound
			if ref.Kind == js_ast.SymbolUnbound {
				switch ref.OriginalName {
				case "process":
					globalSymbols.process = ref
					break
				case "document":
					globalSymbols.document = ref
					break
				case "global":
					globalSymbols.global = ref
					break
				case "window":
					globalSymbols.window = ref
					break
				case "console":
					globalSymbols.console = ref
					break
				}
			}
		}
	}
	return globalSymbols
}

func symbolsAreSame(sym1 *js_ast.Symbol, sym2 *js_ast.Symbol) bool {
	// sym1 == sym2 takes considers useCount, but we just want to know if we are
	// dealing with the same global symbol or not
	return sym1.Link == sym2.Link &&
		sym1.Kind == sym2.Kind &&
		sym1.OriginalName == sym2.OriginalName
}

func functionNameForGlobal(id string) string {
	// Matches electron-link in order to use same blueprint.
	// See: https://github.com/atom/electron-link/blob/abeb97d8633c06ac6a762ac427b272adebd32c4f/src/blueprint.js#L230-L245
	return fmt.Sprintf("get_%s", id)
}

func functionCallForGlobal(id string) string {
	return fmt.Sprintf("%s()", functionNameForGlobal(id))
}
