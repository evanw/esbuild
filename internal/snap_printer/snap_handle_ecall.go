package snap_printer

import "github.com/evanw/esbuild/internal/js_ast"

func (p *printer) handleRequireResolve(ecall *js_ast.ECall) (handled bool) {
	switch tgt := ecall.Target.Data.(type) {
	case *js_ast.EDot:
		if tgt.Name != "resolve" {
			return false
		}
		// Ensure it is a `require.resolve`
		switch reqTgt := tgt.Target.Data.(type) {
		case *js_ast.EIdentifier:
			if p.renamer.IsRequire(reqTgt.Ref) {
				// Cannot rewrite non-custom require.resolve
				if len(ecall.Args) > 1 {
					return false
				}
				p._printRequireResolve(&ecall.Args[0])
				return true
			}
		}
	}
	return false
}

// require.resolve
func (p *printer) handleECall(ecall *js_ast.ECall) (handled bool) {
	return p.handleRequireResolve(ecall)
}

// -----------------
// Printers
// -----------------

func (p *printer) _printRequireResolve(request *js_ast.Expr) {
	p.print("require.resolve(")
	p.printExpr(*request, js_ast.LComma, 0)
	// NOTE: more info about __dirname2/__dirname inside
	// internal/snap_renamer/snap_renamer.go (functionWrapperForAbsPath)
	p.print(", (typeof __filename2 !== 'undefined' ? __filename2 : __filename)")
	p.print(", (typeof __dirname2 !== 'undefined' ? __dirname2 : __dirname)")
}
