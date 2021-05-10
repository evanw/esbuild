package api

import (
	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/snap_printer"
	"github.com/evanw/esbuild/internal/snap_renamer"
)

func replaceNone(string) bool { return false }
func rewriteAll(string) bool  { return true }

func createPrintAST(snapshot *SnapshotOptions, log *logger.Log) bundler.PrintAST {
	if snapshot.CreateSnapshot {
		shouldReplaceRequire := snapshot.ShouldReplaceRequire
		if shouldReplaceRequire == nil {
			shouldReplaceRequire = replaceNone
		}
		shouldRewriteModule := snapshot.ShouldRewriteModule
		if shouldRewriteModule == nil {
			shouldRewriteModule = rewriteAll
		}

		return func(
			tree js_ast.AST,
			symbols js_ast.SymbolMap,
			jsRenamer renamer.Renamer,
			options js_printer.Options) js_printer.PrintResult {
			r := snap_renamer.WrapRenamer(
				&jsRenamer,
				symbols,
				options.FilePath,
				tree.DirnameRef,
				tree.FilenameRef,
				shouldRewriteModule(options.FilePath))

			if options.IsRuntime {
				return js_printer.Print(tree, symbols, &r, options)
			} else {
				result := snap_printer.Print(
					tree,
					symbols,
					&r,
					options,
					snapshot.Doctor,
					true,
					shouldReplaceRequire)

				reportedError := reportValidationErrors(&result, log, options.FilePath)
				if reportedError {
					return result
				}

				if snapshot.VerifyPrint {
					verifyPrint(&result, log, options.FilePath, snapshot.PanicOnError)
				}
				return result
			}
		}
	} else {
		return js_printer.Print
	}
}

func addSnapshotOpts(buildOpts *BuildOptions, configOpts *config.Options) {
	if buildOpts.Snapshot == nil || !buildOpts.Snapshot.CreateSnapshot {
		return
	}
	if buildOpts.Snapshot.AbsBasedir == "" {
		panic("Build configOpts need to have 'Snapshot.AbsBasedir' set when creating a snapshot")
	}
	configOpts.CreateSnapshot = true
	configOpts.SnapshotAbsBaseDir = buildOpts.Snapshot.AbsBasedir
}
