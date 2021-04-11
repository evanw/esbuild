package graph

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/sourcemap"
)

type Module struct {
	Source         logger.Source
	Repr           ModuleRepr
	Loader         config.Loader
	SideEffects    SideEffects
	InputSourceMap *sourcemap.SourceMap

	// If this file ends up being used in the bundle, these are additional files
	// that must be written to the output directory. It's used by the "file"
	// loader.
	AdditionalFiles []OutputFile
}

type OutputFile struct {
	AbsPath  string
	Contents []byte

	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	JSONMetadataChunk string

	IsExecutable bool
}

type SideEffects struct {
	Kind SideEffectsKind

	// This is optional additional information for use in error messages
	Data *resolver.SideEffectsData
}

type SideEffectsKind uint8

const (
	// The default value conservatively considers all files to have side effects.
	HasSideEffects SideEffectsKind = iota

	// This file was listed as not having side effects by a "package.json"
	// file in one of our containing directories with a "sideEffects" field.
	NoSideEffects_PackageJSON

	// This file was loaded using a data-oriented loader (e.g. "text") that is
	// known to not have side effects.
	NoSideEffects_PureData

	// Same as above but it came from a plugin. We don't want to warn about
	// unused imports to these files since running the plugin is a side effect.
	// Removing the import would not call the plugin which is observable.
	NoSideEffects_PureData_FromPlugin
)

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
