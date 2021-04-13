package snap_printer

import (
	"fmt"
	"github.com/evanw/esbuild/internal/js_ast"
)

func (p *printer) printRequireReplacementFunctionAssign(
	require *RequireExpr,
	bindingId string,
	isDestructuring bool,
	fnName string) {

	fnHeader := fmt.Sprintf("%s = function() {", fnName)
	fnBodyStart := fmt.Sprintf("  return %s = %s || (", bindingId, bindingId)
	fnClose := "}"

	p.printNewline()
	p.print(fnHeader)
	p.printNewline()
	p.print(fnBodyStart)
	p.printRequireBody(require)
	if isDestructuring {
		p.print(".")
		p.print(bindingId)
	}
	p.print(")")
	p.printNewline()
	p.print(fnClose)
}

func (p *printer) printReferenceReplacementFunctionAssign(
	expr *js_ast.Expr,
	bindingId string,
	isDestructuring bool,
	fnName string) {

	fnHeader := fmt.Sprintf("%s = function() {", fnName)
	fnBodyStart := fmt.Sprintf("  return %s = %s || (", bindingId, bindingId)
	fnClose := "}"

	p.printNewline()
	p.print(fnHeader)
	p.printNewline()
	p.print(fnBodyStart)
	// TODO(thlorenz): not sure where I'd get a level + flags from in this case
	p.printExpr(*expr, js_ast.LLowest, 0)
	if isDestructuring {
		p.print(".")
		p.print(bindingId)
	}
	p.print(")")
	p.printNewline()
	p.print(fnClose)
}

func (p *printer) printBindings(
	bindings []RequireBinding,
	print func(
		bindingId string,
		isDestructuring bool,
		fnName string),
) {
	for _, b := range bindings {
		var fnName string
		var fnCall string
		var id string

		// Ensure that we don't register a replacement for a ref for which we did this already
		// Additionally the `identifierName` will not be the original one in this case so we need
		// to obtain it and then derive the dependent ids from it.
		if p.renamer.HasBeenReplaced(b.identifier) {
			id = p.renamer.GetOriginalId(b.identifier)
			fnName = functionNameForId(id)
			fnCall = functionCallForId(id)
		} else {
			id = b.identifierName
			fnName = functionNameForId(id)
			fnCall = functionCallForId(id)
			p.renamer.Replace(b.identifier, fnCall)
			p.trackTopLevelVar(fnName)
		}
		print(id, b.isDestructuring, fnName)
	}
}

// similar to slocal but assigning to an already declared variable
// x = require('x')
func (p *printer) handleEBinary(e *js_ast.EBinary) (handled bool) {
	if !p.renamer.IsEnabled {
		return false
	}
	if p.uninvokedFunctionDepth > 0 {
		return false
	}
	if e.Op != js_ast.BinOpAssign || p.prevOp == js_ast.BinOpLogicalAnd {
		return false
	}

	require, isRequire := p.extractRequireExpression(e.Right, 0, 0, 0)
	if isRequire {
		export, isExport := p.extractExport(&e.Left, &e.Right)
		if isExport {
			p.printExportGetter(&export)
			return true
		}
		identifiers, ok := p.extractIdentifiers(e.Left.Data)
		if !ok {
			return false
		}
		p.printBindings(identifiers, func(
			bindingId string,
			isDestructuring bool,
			fnName string) {
			p.printRequireReplacementFunctionAssign(require, bindingId, isDestructuring, fnName)
		})
		return true
	}

	expr := &e.Right
	hasRequireOrGlobalReference := p.expressionHasRequireOrGlobalReference(expr)
	if hasRequireOrGlobalReference {
		// export rewrites to getter
		export, isExport := p.extractExport(&e.Left, &e.Right)
		if isExport {
			p.printExportGetter(&export)
			return true
		}

		// other identifier rewrites
		identifiers, ok := p.extractIdentifiers(e.Left.Data)
		if !ok {
			return false
		}

		// We cannot wrap access to an unbound identifier.e. `exports = ...` since it needs to resolve
		// and be assigned during module load.
		if p.haveUnwrappableIdentifier(identifiers) {
			return false
		}
		p.printBindings(identifiers, func(
			bindingId string,
			isDestructuring bool,
			fnName string) {
			p.printReferenceReplacementFunctionAssign(expr, bindingId, isDestructuring, fnName)
		})
		return true
	}
	return false
}
