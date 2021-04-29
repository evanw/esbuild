package snap_printer

import (
	"fmt"

	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/snap_renamer"
)

type SnapAstValiator struct {
	renamer *snap_renamer.SnapRenamer
}

func (v *SnapAstValiator) verifySExpr(expr *js_ast.SExpr) (string, bool) {

	// Detect monkey patches on `process`, i.e. `process.emitWarning = function () { ... }`
	// We don't allow them since they cause problems with rewrites performed on
	// top of them, namely this leads to unintended recursive calls

	// <target>.<name> = <function>
	switch binary := expr.Value.Data.(type) {
	case *js_ast.EBinary:
		// <target>.<name>
		switch left := binary.Left.Data.(type) {
		case *js_ast.EDot:
			// <target>
			switch target := left.Target.Data.(type) {

			case *js_ast.EIdentifier:
				if v.renamer.IsProcessRef(target.Ref) {
					// At this point we know that a property on the global `process` object is being assigned
					// Now we look at the assigned value determine if it is a function declared inline
					switch right := binary.Right.Data.(type) {
					case *js_ast.EFunction, *js_ast.EArrow:
						return fmt.Sprintf("Cannot override 'process.%s'", left.Name), false
					case *js_ast.EIdentifier:
						// Or if it is an identifier of a function
						if v.renamer.IsFunctionRef(right.Ref) {
							return fmt.Sprintf("Cannot override 'process.%s'", left.Name), false
						}
					}

				}
			}
		}
	}
	return "", true
}
