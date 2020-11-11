package snap_printer

import (
	"fmt"
	"github.com/evanw/esbuild/internal/js_ast"
)

func stringifyEString(estring *js_ast.EString) string {
	s := ""
	for _, char := range estring.Value {
		s += fmt.Sprintf("%c", char)
	}
	return s
}

func functionCallForId(id string) string {
	return fmt.Sprintf("(%s())", functionNameForId(id))
}

func functionDeclarationForId(id string) string {
	return fmt.Sprintf("%s()", functionNameForId(id))
}

func functionNameForId(id string) string {
	return fmt.Sprintf("__get_%s__", id)
}

func functionNameForGlobal(id string) string {
	// Matches electron-link in order to use same blueprint.
	// See: https://github.com/atom/electron-link/blob/abeb97d8633c06ac6a762ac427b272adebd32c4f/src/blueprint.js#L230-L245
	return fmt.Sprintf("get_%s", id)
}

func functionCallForGlobal(id string) string {
	return fmt.Sprintf("%s()", functionNameForGlobal(id))
}
