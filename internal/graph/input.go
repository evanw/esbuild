package graph

// The code in this file mainly represents data that passes from the scan phase
// to the compile phase of the bundler. There is currently one exception: the
// "meta" member of the JavaScript file representation. That could have been
// stored separately but is stored together for convenience and to avoid an
// extra level of indirection. Instead it's kept in a separate type to keep
// things organized.

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/sourcemap"
)

type InputFile struct {
	Repr           InputFileRepr
	InputSourceMap *sourcemap.SourceMap

	// If this file ends up being used in the bundle, these are additional files
	// that must be written to the output directory. It's used by the "file" and
	// "copy" loaders.
	AdditionalFiles            []OutputFile
	UniqueKeyForAdditionalFile string

	SideEffects SideEffects
	Source      logger.Source
	Loader      config.Loader

	OmitFromSourceMapsAndMetafile bool
}

type OutputFile struct {
	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	JSONMetadataChunk string

	AbsPath      string
	Contents     []byte
	IsExecutable bool
}

type SideEffects struct {
	// This is optional additional information for use in error messages
	Data *resolver.SideEffectsData

	Kind SideEffectsKind
}

type SideEffectsKind uint8

const (
	// The default value conservatively considers all files to have side effects.
	HasSideEffects SideEffectsKind = iota

	// This file was listed as not having side effects by a "package.json"
	// file in one of our containing directories with a "sideEffects" field.
	NoSideEffects_PackageJSON

	// This file is considered to have no side effects because the AST was empty
	// after parsing finished. This should be the case for ".d.ts" files.
	NoSideEffects_EmptyAST

	// This file was loaded using a data-oriented loader (e.g. "text") that is
	// known to not have side effects.
	NoSideEffects_PureData

	// Same as above but it came from a plugin. We don't want to warn about
	// unused imports to these files since running the plugin is a side effect.
	// Removing the import would not call the plugin which is observable.
	NoSideEffects_PureData_FromPlugin
)

type InputFileRepr interface {
	ImportRecords() *[]ast.ImportRecord
}

type JSRepr struct {
	Meta JSReprMeta
	AST  js_ast.AST

	// If present, this is the CSS file that this JavaScript stub corresponds to.
	// A JavaScript stub is automatically generated for a CSS file when it's
	// imported from a JavaScript file.
	CSSSourceIndex ast.Index32
}

func (repr *JSRepr) ImportRecords() *[]ast.ImportRecord {
	return &repr.AST.ImportRecords
}

func (repr *JSRepr) TopLevelSymbolToParts(ref ast.Ref) []uint32 {
	// Overlay the mutable map from the linker
	if parts, ok := repr.Meta.TopLevelSymbolToPartsOverlay[ref]; ok {
		return parts
	}

	// Fall back to the immutable map from the parser
	return repr.AST.TopLevelSymbolToPartsFromParser[ref]
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

type CopyRepr struct {
	// The URL that replaces the contents of any import record paths for this file
	URLForCode string
}

func (repr *CopyRepr) ImportRecords() *[]ast.ImportRecord {
	return nil
}
