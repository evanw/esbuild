package js_parser

import (
	"github.com/evanw/esbuild/internal/js_ast"
	"path/filepath"
	"strings"
)

func (p *parser) getWrapperRef() js_ast.Ref {
	if p.options.CreateSnapshot {
		relPath, err := filepath.Rel(p.options.SnapshotAbsBaseDir, p.source.KeyPath.Text)
		if err != nil {
			panic(err)
		}
		if !strings.HasPrefix(relPath, ".") {
			relPath = "./" + relPath
		}
		return p.newSymbol(js_ast.SymbolUnbound, relPath)
	} else {
		return p.newSymbol(js_ast.SymbolOther, "require_"+p.source.IdentifierName)
	}
}
