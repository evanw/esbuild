package bundler

import (
	"fmt"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"path/filepath"
)

func fileInfoJSON(f *file) string {
	return fmt.Sprintf(`{
        "fullPath": "%s"
     }`,
		filepath.ToSlash(f.source.KeyPath.Text),
	)
}

func requireDefinition(
	requireDefinitionsRef js_ast.Ref,
	request string,
	assignedValue *js_ast.Expr) js_ast.Stmt {
	requestValue := js_lexer.StringToUTF16(request)
	// <requireDefinitionsRef>["request>"] = <assignedValue>
	return js_ast.Stmt{
		Data: &js_ast.SExpr{
			Value: js_ast.Expr{Data: &js_ast.EBinary{
				Op: js_ast.BinOpAssign,
				Left: js_ast.Expr{
					Data: &js_ast.EIndex{
						Target: js_ast.Expr{Data: &js_ast.EIdentifier{
							Ref: requireDefinitionsRef,
						}},
						Index: js_ast.Expr{Data: &js_ast.EString{
							Value: requestValue,
						}},
						OptionalChain: js_ast.OptionalChainNone,
					},
				},
				Right: *assignedValue,
			}},
		},
	}
}

func requireDefinitionStmt(
	r *renamer.Renamer,
	repr *reprJS,
	stmts []js_ast.Stmt,
	commonJSRef js_ast.Ref,
) js_ast.Stmt {
	// function (exports, module, __filename, __dirname, require) { ... }
	args := []js_ast.Arg{
		{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.ExportsRef}}},
		{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.ModuleRef}}},
		{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.FilenameRef}}},
		{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.DirnameRef}}},
		{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.RequireRef}}},
	}
	value := js_ast.Expr{Data: &js_ast.EFunction{
		Fn: js_ast.Fn{Args: args, Body: js_ast.FnBody{Stmts: stmts}},
	}}
	request := (*r).NameForSymbol(repr.ast.WrapperRef)

	// For Snapshots commonJSRef is set to `var __commonJS = {}` via runtime.Snapshot
	// __commonJS['./foo.js'] = function (exports, module, __filename, __dirname, require) { .. }
	return requireDefinition(commonJSRef, request, &value)
}

func pathIsAlwaysExternal(options config.Options, path logger.Path) bool {
	if options.CreateSnapshot {
		return filepath.Ext(path.Text) == ".node"
	} else {
		return false
	}
}
