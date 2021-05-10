package snap_printer

import (
	"fmt"

	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/snap_renamer"
)

const SNAPSHOT_REWRITE_FAILURE = "[SNAPSHOT_REWRITE_FAILURE]"
const SNAPSHOT_CACHE_FAILURE = "[SNAPSHOT_CACHE_FAILURE]"

type SnapAstValiator struct {
	renamer        *snap_renamer.SnapRenamer
	validateStrict bool
}

func (v *SnapAstValiator) verifySExpr(expr *js_ast.SExpr) (string, bool) {
	if !v.validateStrict {
		return "", true
	}

	// Detect monkey patches on `process`, i.e. `process.emitWarning = function () { ... }`
	// We don't allow them since they cause problems with rewrites performed on
	// top of them, namely this leads to unintended recursive calls

	// This kind of validation error should cause a norewrite and thus will exit printing
	// JavaScript early and report the error.

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

func (v *SnapAstValiator) verifyEIfBranchTarget(expr *js_ast.Expr) (string, bool) {
	if !v.validateStrict {
		return "", true
	}
	// Detect conditional assignments that depend on globals, i.e. `var x = Buffer ? Buffer.isBuffer : undefined`

	// This kind of validation error should cause a defer and will be rewritten by the printer so that
	// the section of code is rewritten to throw an Error. This guarantees that any dependent modules
	// triggering the section of code to run will also be deferred.

	// TODO(thlorenz): the above should only occur when we're in the 'doctor' phase

	switch access := expr.Data.(type) {
	// <target>.<property>
	case *js_ast.EDot:
		switch target := access.Target.Data.(type) {
		case *js_ast.EIdentifier:
			if name, isGlobal := v.renamer.IsGlobalEntityRef(target.Ref); isGlobal {
				return fmt.Sprintf("Cannot probe '%s' properties", name), false
			}

		}
	}
	return "", true
}

// Prints code that will throw an Error when it runs. The error message is derived from the
// validation error message.
func (p *printer) printThrowValidationError(err *ValidationError) {
	p.print("(function () { throw new Error(")
	var msg string
	switch err.Kind {
	case Defer:
		msg = fmt.Sprintf("%s %s", SNAPSHOT_CACHE_FAILURE, err.Msg)
		break
	case NoRewrite:
		msg = fmt.Sprintf("%s %s", SNAPSHOT_REWRITE_FAILURE, err.Msg)
		break
	default:
		panic("Invalid validation error kind")
	}
	p.printQuotedUTF8(msg, true)
	p.print(") })()")
}
