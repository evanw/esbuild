package cache

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

// Each build parses "package.json" files again, so the "type" field that a
// file inherits points to a different "package.json" source every time. The
// cached AST should only be thrown away if that source actually changed.
func TestJSCacheModuleTypeFromPackageJSON(t *testing.T) {
	source := test.SourceForTest("export let x = 1")

	packageJSON := func(contents string, moduleType js_ast.ModuleType) js_ast.ModuleTypeData {
		jsonSource := &logger.Source{
			KeyPath:  logger.Path{Text: "/package.json", Namespace: "file"},
			Contents: contents,
		}
		return js_ast.ModuleTypeData{
			Type:   moduleType,
			Source: jsonSource,
			Range:  jsonSource.RangeOfString(logger.Loc{Start: 8}),
		}
	}

	parse := func(c *JSCache, moduleTypeData js_ast.ModuleTypeData) js_ast.AST {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		options := config.Options{ModuleTypeData: moduleTypeData}
		tree, ok := c.Parse(log, source, js_parser.OptionsFromConfig(&options))
		if !ok {
			t.Fatal("Parse error")
		}
		return tree
	}

	check := func(name string, first js_ast.ModuleTypeData, second js_ast.ModuleTypeData, expectCacheHit bool) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			c := &MakeCacheSet().JSCache
			a := parse(c, first)
			b := parse(c, second)
			if cacheHit := a.ModuleScope == b.ModuleScope; cacheHit != expectCacheHit {
				t.Fatalf("Expected cache hit to be %v but it was %v", expectCacheHit, cacheHit)
			}
			test.AssertEqual(t, b.ModuleTypeData.Type, second.Type)
		})
	}

	check("NoType",
		js_ast.ModuleTypeData{},
		js_ast.ModuleTypeData{},
		true)
	check("SameModule",
		packageJSON(`{"type":"module"}`, js_ast.ModuleESM_PackageJSON),
		packageJSON(`{"type":"module"}`, js_ast.ModuleESM_PackageJSON),
		true)
	check("SameCommonJS",
		packageJSON(`{"type":"commonjs"}`, js_ast.ModuleCommonJS_PackageJSON),
		packageJSON(`{"type":"commonjs"}`, js_ast.ModuleCommonJS_PackageJSON),
		true)
	check("ModuleToCommonJS",
		packageJSON(`{"type":"module"}`, js_ast.ModuleESM_PackageJSON),
		packageJSON(`{"type":"commonjs"}`, js_ast.ModuleCommonJS_PackageJSON),
		false)
	check("ModuleToNoType",
		packageJSON(`{"type":"module"}`, js_ast.ModuleESM_PackageJSON),
		js_ast.ModuleTypeData{},
		false)
	check("OtherFieldChanged",
		packageJSON(`{"type":"module"}`, js_ast.ModuleESM_PackageJSON),
		packageJSON(`{"type":"module","name":"pkg"}`, js_ast.ModuleESM_PackageJSON),
		false)
}
