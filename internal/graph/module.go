package graph

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

type Module struct {
	Source logger.Source
	Repr   ModuleRepr
}

type ModuleRepr interface {
	ImportRecords() *[]ast.ImportRecord
}

type JSRepr struct {
	AST  js_ast.AST
	Meta JSReprMeta

	// If present, this is the CSS file that this JavaScript stub corresponds to.
	// A JavaScript stub is automatically generated for a CSS file when it's
	// imported from a JavaScript file.
	CSSSourceIndex ast.Index32

	DidWrapDependencies bool
}

func (repr *JSRepr) ImportRecords() *[]ast.ImportRecord {
	return &repr.AST.ImportRecords
}

type CSSRepr struct {
	AST css_ast.AST

	// If present, this is the JavaScript stub corresponding to this CSS file.
	// A JavaScript stub is automatically generated for a CSS file when it's
	// imported from a JavaScript file.
	JSSourceIndex ast.Index32
}

func (repr *CSSRepr) ImportRecords() *[]ast.ImportRecord {
	return &repr.AST.ImportRecords
}
