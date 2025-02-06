package config

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

type JSXOptions struct {
	Factory          DefineExpr
	Fragment         DefineExpr
	Parse            bool
	Preserve         bool
	AutomaticRuntime bool
	ImportSource     string
	Development      bool
	SideEffects      bool
}

type TSJSX uint8

const (
	TSJSXNone TSJSX = iota
	TSJSXPreserve
	TSJSXReactNative
	TSJSXReact
	TSJSXReactJSX
	TSJSXReactJSXDev
)

type TSOptions struct {
	Config              TSConfig
	Parse               bool
	NoAmbiguousLessThan bool
}

type TSConfigJSX struct {
	// If not empty, these should override the default values
	JSXFactory         []string // Default if empty: "React.createElement"
	JSXFragmentFactory []string // Default if empty: "React.Fragment"
	JSXImportSource    *string  // Default if empty: "react"
	JSX                TSJSX
}

// This is used for "extends" in "tsconfig.json"
func (derived *TSConfigJSX) ApplyExtendedConfig(base TSConfigJSX) {
	if base.JSXFactory != nil {
		derived.JSXFactory = base.JSXFactory
	}
	if base.JSXFragmentFactory != nil {
		derived.JSXFragmentFactory = base.JSXFragmentFactory
	}
	if base.JSXImportSource != nil {
		derived.JSXImportSource = base.JSXImportSource
	}
	if base.JSX != TSJSXNone {
		derived.JSX = base.JSX
	}
}

func (tsConfig *TSConfigJSX) ApplyTo(jsxOptions *JSXOptions) {
	switch tsConfig.JSX {
	case TSJSXPreserve, TSJSXReactNative:
		// Deliberately don't set "Preserve = true" here. Some tools from Vercel
		// apparently automatically set "jsx": "preserve" in "tsconfig.json" and
		// people are then confused when esbuild preserves their JSX. Ignoring this
		// value means you now have to explicitly pass "--jsx=preserve" to esbuild
		// to get this behavior.

	case TSJSXReact:
		jsxOptions.AutomaticRuntime = false
		jsxOptions.Development = false

	case TSJSXReactJSX:
		jsxOptions.AutomaticRuntime = true
		// Deliberately don't set "Development = false" here. People want to be
		// able to have "react-jsx" in their "tsconfig.json" file and then swap
		// that to "react-jsxdev" by passing "--jsx-dev" to esbuild.

	case TSJSXReactJSXDev:
		jsxOptions.AutomaticRuntime = true
		jsxOptions.Development = true
	}

	if len(tsConfig.JSXFactory) > 0 {
		jsxOptions.Factory = DefineExpr{Parts: tsConfig.JSXFactory}
	}

	if len(tsConfig.JSXFragmentFactory) > 0 {
		jsxOptions.Fragment = DefineExpr{Parts: tsConfig.JSXFragmentFactory}
	}

	if tsConfig.JSXImportSource != nil {
		jsxOptions.ImportSource = *tsConfig.JSXImportSource
	}
}

// Note: This can currently only contain primitive values. It's compared
// for equality using a structural equality comparison by the JS parser.
type TSConfig struct {
	ExperimentalDecorators  MaybeBool
	ImportsNotUsedAsValues  TSImportsNotUsedAsValues
	PreserveValueImports    MaybeBool
	Target                  TSTarget
	UseDefineForClassFields MaybeBool
	VerbatimModuleSyntax    MaybeBool
}

// This is used for "extends" in "tsconfig.json"
func (derived *TSConfig) ApplyExtendedConfig(base TSConfig) {
	if base.ExperimentalDecorators != Unspecified {
		derived.ExperimentalDecorators = base.ExperimentalDecorators
	}
	if base.ImportsNotUsedAsValues != TSImportsNotUsedAsValues_None {
		derived.ImportsNotUsedAsValues = base.ImportsNotUsedAsValues
	}
	if base.PreserveValueImports != Unspecified {
		derived.PreserveValueImports = base.PreserveValueImports
	}
	if base.Target != TSTargetUnspecified {
		derived.Target = base.Target
	}
	if base.UseDefineForClassFields != Unspecified {
		derived.UseDefineForClassFields = base.UseDefineForClassFields
	}
	if base.VerbatimModuleSyntax != Unspecified {
		derived.VerbatimModuleSyntax = base.VerbatimModuleSyntax
	}
}

func (cfg *TSConfig) UnusedImportFlags() (flags TSUnusedImportFlags) {
	if cfg.VerbatimModuleSyntax == True {
		return TSUnusedImport_KeepStmt | TSUnusedImport_KeepValues
	}
	if cfg.PreserveValueImports == True {
		flags |= TSUnusedImport_KeepValues
	}
	if cfg.ImportsNotUsedAsValues == TSImportsNotUsedAsValues_Preserve || cfg.ImportsNotUsedAsValues == TSImportsNotUsedAsValues_Error {
		flags |= TSUnusedImport_KeepStmt
	}
	return
}

type Platform uint8

const (
	PlatformBrowser Platform = iota
	PlatformNode
	PlatformNeutral
)

type SourceMap uint8

const (
	SourceMapNone SourceMap = iota
	SourceMapInline
	SourceMapLinkedWithComment
	SourceMapExternalWithoutComment
	SourceMapInlineAndExternal
)

type LegalComments uint8

const (
	LegalCommentsInline LegalComments = iota
	LegalCommentsNone
	LegalCommentsEndOfFile
	LegalCommentsLinkedWithComment
	LegalCommentsExternalWithoutComment
)

func (lc LegalComments) HasExternalFile() bool {
	return lc == LegalCommentsLinkedWithComment || lc == LegalCommentsExternalWithoutComment
}

type Loader uint8

const (
	LoaderNone Loader = iota
	LoaderBase64
	LoaderBinary
	LoaderCopy
	LoaderCSS
	LoaderDataURL
	LoaderDefault
	LoaderEmpty
	LoaderFile
	LoaderGlobalCSS
	LoaderJS
	LoaderJSON
	LoaderWithTypeJSON // Has a "with { type: 'json' }" attribute
	LoaderJSX
	LoaderLocalCSS
	LoaderText
	LoaderTS
	LoaderTSNoAmbiguousLessThan // Used with ".mts" and ".cts"
	LoaderTSX
)

var LoaderToString = []string{
	"none",
	"base64",
	"binary",
	"copy",
	"css",
	"dataurl",
	"default",
	"empty",
	"file",
	"global-css",
	"js",
	"json",
	"json",
	"jsx",
	"local-css",
	"text",
	"ts",
	"ts",
	"tsx",
}

func (loader Loader) IsTypeScript() bool {
	switch loader {
	case LoaderTS, LoaderTSNoAmbiguousLessThan, LoaderTSX:
		return true
	}
	return false
}

func (loader Loader) IsCSS() bool {
	switch loader {
	case
		LoaderCSS, LoaderGlobalCSS, LoaderLocalCSS:
		return true
	}
	return false
}

func (loader Loader) CanHaveSourceMap() bool {
	switch loader {
	case
		LoaderJS, LoaderJSX,
		LoaderTS, LoaderTSNoAmbiguousLessThan, LoaderTSX,
		LoaderCSS, LoaderGlobalCSS, LoaderLocalCSS,
		LoaderJSON, LoaderWithTypeJSON, LoaderText:
		return true
	}
	return false
}

func LoaderFromFileExtension(extensionToLoader map[string]Loader, base string) Loader {
	// Pick the loader with the longest matching extension. So if there's an
	// extension for ".css" and for ".module.css", we want to match the one for
	// ".module.css" before the one for ".css".
	if i := strings.IndexByte(base, '.'); i != -1 {
		for {
			if loader, ok := extensionToLoader[base[i:]]; ok {
				return loader
			}
			base = base[i+1:]
			i = strings.IndexByte(base, '.')
			if i == -1 {
				break
			}
		}
	} else {
		// If there's no extension, explicitly check for an extensionless loader
		if loader, ok := extensionToLoader[""]; ok {
			return loader
		}
	}
	return LoaderNone
}

type Format uint8

const (
	// This is used when not bundling. It means to preserve whatever form the
	// import or export was originally in. ES6 syntax stays ES6 syntax and
	// CommonJS syntax stays CommonJS syntax.
	FormatPreserve Format = iota

	// IIFE stands for immediately-invoked function expression. That looks like
	// this:
	//
	//   (() => {
	//     ... bundled code ...
	//   })();
	//
	// If the optional GlobalName is configured, then we'll write out this:
	//
	//   let globalName = (() => {
	//     ... bundled code ...
	//     return exports;
	//   })();
	//
	FormatIIFE

	// The CommonJS format looks like this:
	//
	//   ... bundled code ...
	//   module.exports = exports;
	//
	FormatCommonJS

	// The ES module format looks like this:
	//
	//   ... bundled code ...
	//   export {...};
	//
	FormatESModule
)

func (f Format) KeepESMImportExportSyntax() bool {
	return f == FormatPreserve || f == FormatESModule
}

func (f Format) String() string {
	switch f {
	case FormatIIFE:
		return "iife"
	case FormatCommonJS:
		return "cjs"
	case FormatESModule:
		return "esm"
	}
	return ""
}

type StdinInfo struct {
	Contents      string
	SourceFile    string
	AbsResolveDir string
	Loader        Loader
}

type WildcardPattern struct {
	Prefix string
	Suffix string
}

type ExternalMatchers struct {
	Exact    map[string]bool
	Patterns []WildcardPattern
}

func (matchers ExternalMatchers) HasMatchers() bool {
	return len(matchers.Exact) > 0 || len(matchers.Patterns) > 0
}

type ExternalSettings struct {
	PreResolve  ExternalMatchers
	PostResolve ExternalMatchers
}

type APICall uint8

const (
	BuildCall APICall = iota
	TransformCall
)

type Mode uint8

const (
	ModePassThrough Mode = iota
	ModeConvertFormat
	ModeBundle
)

type MaybeBool uint8

const (
	Unspecified MaybeBool = iota
	True
	False
)

type CancelFlag struct {
	uint32
}

func (flag *CancelFlag) Cancel() {
	atomic.StoreUint32(&flag.uint32, 1)
}

// This checks for nil in one place so we don't have to do that everywhere
func (flag *CancelFlag) DidCancel() bool {
	return flag != nil && atomic.LoadUint32(&flag.uint32) != 0
}

type Options struct {
	ModuleTypeData js_ast.ModuleTypeData
	Defines        *ProcessedDefines
	TSAlwaysStrict *TSAlwaysStrict
	MangleProps    *regexp.Regexp
	ReserveProps   *regexp.Regexp
	CancelFlag     *CancelFlag

	// When mangling property names, call this function with a callback and do
	// the property name mangling inside the callback. The callback takes an
	// argument which is the mangle cache map to mutate. These callbacks are
	// serialized so mutating the map does not require extra synchronization.
	//
	// This is a callback for determinism reasons. We may be building multiple
	// entry points in parallel that are supposed to share a single cache. We
	// don't want the order that each entry point mangles properties in to cause
	// the output to change, so we serialize the property mangling over all entry
	// points in entry point order. However, we still want to link everything in
	// parallel so only property mangling is serialized, which is implemented by
	// this function blocking until the previous entry point's property mangling
	// has finished.
	ExclusiveMangleCacheUpdate func(cb func(
		mangleCache map[string]interface{},
		cssUsedLocalNames map[string]bool,
	))

	// This is the original information that was used to generate the
	// unsupported feature sets above. It's used for error messages.
	OriginalTargetEnv string

	DropLabels       []string
	ExtensionOrder   []string
	MainFields       []string
	Conditions       []string
	AbsNodePaths     []string // The "NODE_PATH" variable from Node.js
	ExternalSettings ExternalSettings
	ExternalPackages bool
	PackageAliases   map[string]string

	AbsOutputFile      string
	AbsOutputDir       string
	AbsOutputBase      string
	OutputExtensionJS  string
	OutputExtensionCSS string
	GlobalName         []string
	TSConfigPath       string
	TSConfigRaw        string
	ExtensionToLoader  map[string]Loader

	PublicPath      string
	InjectPaths     []string
	InjectedDefines []InjectedDefine
	InjectedFiles   []InjectedFile

	JSBanner  string
	JSFooter  string
	CSSBanner string
	CSSFooter string

	EntryPathTemplate []PathTemplate
	ChunkPathTemplate []PathTemplate
	AssetPathTemplate []PathTemplate

	Plugins    []Plugin
	SourceRoot string
	Stdin      *StdinInfo
	JSX        JSXOptions
	LineLimit  int

	CSSPrefixData          map[css_ast.D]compat.CSSPrefix
	UnsupportedJSFeatures  compat.JSFeature
	UnsupportedCSSFeatures compat.CSSFeature

	UnsupportedJSFeatureOverrides      compat.JSFeature
	UnsupportedJSFeatureOverridesMask  compat.JSFeature
	UnsupportedCSSFeatureOverrides     compat.CSSFeature
	UnsupportedCSSFeatureOverridesMask compat.CSSFeature

	TS                TSOptions
	Mode              Mode
	PreserveSymlinks  bool
	MinifyWhitespace  bool
	MinifyIdentifiers bool
	MinifySyntax      bool
	ProfilerNames     bool
	CodeSplitting     bool
	WatchMode         bool
	AllowOverwrite    bool
	LegalComments     LegalComments

	// If true, make sure to generate a single file that can be written to stdout
	WriteToStdout bool

	OmitRuntimeForTests    bool
	OmitJSXRuntimeForTests bool
	ASCIIOnly              bool
	KeepNames              bool
	IgnoreDCEAnnotations   bool
	TreeShaking            bool
	DropDebugger           bool
	MangleQuoted           bool
	Platform               Platform
	OutputFormat           Format
	NeedsMetafile          bool
	SourceMap              SourceMap
	ExcludeSourcesContent  bool
}

type TSImportsNotUsedAsValues uint8

const (
	TSImportsNotUsedAsValues_None TSImportsNotUsedAsValues = iota
	TSImportsNotUsedAsValues_Remove
	TSImportsNotUsedAsValues_Preserve
	TSImportsNotUsedAsValues_Error
)

// These flags represent the following separate "tsconfig.json" settings:
//
// - importsNotUsedAsValues
// - preserveValueImports
// - verbatimModuleSyntax
//
// TypeScript prefers for people to use "verbatimModuleSyntax" and has
// deprecated the other two settings, but we must still support them.
// All settings are combined into these two behavioral flags for us.
type TSUnusedImportFlags uint8

// With !TSUnusedImport_KeepStmt && !TSUnusedImport_KeepValues:
//
//	"import 'foo'"                      => "import 'foo'"
//	"import * as unused from 'foo'"     => ""
//	"import { unused } from 'foo'"      => ""
//	"import { type unused } from 'foo'" => ""
//
// With TSUnusedImport_KeepStmt && !TSUnusedImport_KeepValues:
//
//	"import 'foo'"                      => "import 'foo'"
//	"import * as unused from 'foo'"     => "import 'foo'"
//	"import { unused } from 'foo'"      => "import 'foo'"
//	"import { type unused } from 'foo'" => "import 'foo'"
//
// With !TSUnusedImport_KeepStmt && TSUnusedImport_KeepValues:
//
//	"import 'foo'"                      => "import 'foo'"
//	"import * as unused from 'foo'"     => "import * as unused from 'foo'"
//	"import { unused } from 'foo'"      => "import { unused } from 'foo'"
//	"import { type unused } from 'foo'" => ""
//
// With TSUnusedImport_KeepStmt && TSUnusedImport_KeepValues:
//
//	"import 'foo'"                      => "import 'foo'"
//	"import * as unused from 'foo'"     => "import * as unused from 'foo'"
//	"import { unused } from 'foo'"      => "import { unused } from 'foo'"
//	"import { type unused } from 'foo'" => "import {} from 'foo'"
const (
	TSUnusedImport_KeepStmt   TSUnusedImportFlags = 1 << iota // "importsNotUsedAsValues" != "remove"
	TSUnusedImport_KeepValues                                 // "preserveValueImports" == true
)

type TSTarget uint8

const (
	TSTargetUnspecified     TSTarget = iota
	TSTargetBelowES2022              // "useDefineForClassFields" defaults to false
	TSTargetAtOrAboveES2022          // "useDefineForClassFields" defaults to true
)

type TSAlwaysStrict struct {
	// This information is only used for error messages
	Name   string
	Source logger.Source
	Range  logger.Range

	// This information can affect code transformation
	Value bool
}

type PathPlaceholder uint8

const (
	NoPlaceholder PathPlaceholder = iota

	// The relative path from the original parent directory to the configured
	// "outbase" directory, or to the lowest common ancestor directory
	DirPlaceholder

	// The original name of the file, or the manual chunk name, or the name of
	// the type of output file ("entry" or "chunk" or "asset")
	NamePlaceholder

	// A hash of the contents of this file, and the contents and output paths of
	// all dependencies (except for their hash placeholders)
	HashPlaceholder

	// The original extension of the file, or the name of the output file
	// (e.g. "css", "svg", "png")
	ExtPlaceholder
)

type PathTemplate struct {
	Data        string
	Placeholder PathPlaceholder
}

type PathPlaceholders struct {
	Dir  *string
	Name *string
	Hash *string
	Ext  *string
}

func (placeholders PathPlaceholders) Get(placeholder PathPlaceholder) *string {
	switch placeholder {
	case DirPlaceholder:
		return placeholders.Dir
	case NamePlaceholder:
		return placeholders.Name
	case HashPlaceholder:
		return placeholders.Hash
	case ExtPlaceholder:
		return placeholders.Ext
	}
	return nil
}

func TemplateToString(template []PathTemplate) string {
	if len(template) == 1 && template[0].Placeholder == NoPlaceholder {
		// Avoid allocations in this case
		return template[0].Data
	}
	sb := strings.Builder{}
	for _, part := range template {
		sb.WriteString(part.Data)
		switch part.Placeholder {
		case DirPlaceholder:
			sb.WriteString("[dir]")
		case NamePlaceholder:
			sb.WriteString("[name]")
		case HashPlaceholder:
			sb.WriteString("[hash]")
		case ExtPlaceholder:
			sb.WriteString("[ext]")
		}
	}
	return sb.String()
}

func HasPlaceholder(template []PathTemplate, placeholder PathPlaceholder) bool {
	for _, part := range template {
		if part.Placeholder == placeholder {
			return true
		}
	}
	return false
}

func SubstituteTemplate(template []PathTemplate, placeholders PathPlaceholders) []PathTemplate {
	// Don't allocate if no substitution is possible and the template is already minimal
	shouldSubstitute := false
	for i, part := range template {
		if placeholders.Get(part.Placeholder) != nil || (part.Placeholder == NoPlaceholder && i+1 < len(template)) {
			shouldSubstitute = true
			break
		}
	}
	if !shouldSubstitute {
		return template
	}

	// Otherwise, substitute and merge as appropriate
	result := make([]PathTemplate, 0, len(template))
	for _, part := range template {
		if sub := placeholders.Get(part.Placeholder); sub != nil {
			part.Data += *sub
			part.Placeholder = NoPlaceholder
		}
		if last := len(result) - 1; last >= 0 && result[last].Placeholder == NoPlaceholder {
			last := &result[last]
			last.Data += part.Data
			last.Placeholder = part.Placeholder
		} else {
			result = append(result, part)
		}
	}
	return result
}

func ShouldCallRuntimeRequire(mode Mode, outputFormat Format) bool {
	return mode == ModeBundle && outputFormat != FormatCommonJS
}

type InjectedDefine struct {
	Data   js_ast.E
	Name   string
	Source logger.Source
}

type InjectedFile struct {
	Exports      []InjectableExport
	DefineName   string // For injected files generated when you "--define" a non-literal
	Source       logger.Source
	IsCopyLoader bool // If you set the loader to "copy" (see https://github.com/evanw/esbuild/issues/3041)
}

type InjectableExport struct {
	Alias string
	Loc   logger.Loc
}

var filterMutex sync.Mutex
var filterCache map[string]*regexp.Regexp

func compileFilter(filter string) (result *regexp.Regexp) {
	if filter == "" {
		// Must provide a filter
		return nil
	}
	ok := false

	// Cache hit?
	(func() {
		filterMutex.Lock()
		defer filterMutex.Unlock()
		if filterCache != nil {
			result, ok = filterCache[filter]
		}
	})()
	if ok {
		return
	}

	// Cache miss
	result, err := regexp.Compile(filter)
	if err != nil {
		return nil
	}

	// Cache for next time
	filterMutex.Lock()
	defer filterMutex.Unlock()
	if filterCache == nil {
		filterCache = make(map[string]*regexp.Regexp)
	}
	filterCache[filter] = result
	return
}

func CompileFilterForPlugin(pluginName string, kind string, filter string) (*regexp.Regexp, error) {
	if filter == "" {
		return nil, fmt.Errorf("[%s] %q is missing a filter", pluginName, kind)
	}

	result := compileFilter(filter)
	if result == nil {
		return nil, fmt.Errorf("[%s] %q filter is not a valid Go regular expression: %q", pluginName, kind, filter)
	}

	return result, nil
}

func PluginAppliesToPath(path logger.Path, filter *regexp.Regexp, namespace string) bool {
	return (namespace == "" || path.Namespace == namespace) && filter.MatchString(path.Text)
}

////////////////////////////////////////////////////////////////////////////////
// Plugin API

type Plugin struct {
	Name      string
	OnStart   []OnStart
	OnResolve []OnResolve
	OnLoad    []OnLoad
}

type OnStart struct {
	Callback func() OnStartResult
	Name     string
}

type OnStartResult struct {
	ThrownError error
	Msgs        []logger.Msg
}

type OnResolve struct {
	Filter    *regexp.Regexp
	Callback  func(OnResolveArgs) OnResolveResult
	Name      string
	Namespace string
}

type OnResolveArgs struct {
	Path       string
	ResolveDir string
	PluginData interface{}
	Importer   logger.Path
	Kind       ast.ImportKind
	With       logger.ImportAttributes
}

type OnResolveResult struct {
	PluginName string

	Msgs        []logger.Msg
	ThrownError error

	AbsWatchFiles []string
	AbsWatchDirs  []string

	PluginData       interface{}
	Path             logger.Path
	External         bool
	IsSideEffectFree bool
}

type OnLoad struct {
	Filter    *regexp.Regexp
	Callback  func(OnLoadArgs) OnLoadResult
	Name      string
	Namespace string
}

type OnLoadArgs struct {
	PluginData interface{}
	Path       logger.Path
}

type OnLoadResult struct {
	PluginName string

	Contents      *string
	AbsResolveDir string
	PluginData    interface{}

	Msgs        []logger.Msg
	ThrownError error

	AbsWatchFiles []string
	AbsWatchDirs  []string

	Loader Loader
}

func PrettyPrintTargetEnvironment(originalTargetEnv string, unsupportedJSFeatureOverridesMask compat.JSFeature) (where string) {
	where = "the configured target environment"
	overrides := ""
	if unsupportedJSFeatureOverridesMask != 0 {
		count := 0
		mask := unsupportedJSFeatureOverridesMask
		for mask != 0 {
			if (mask & 1) != 0 {
				count++
			}
			mask >>= 1
		}
		s := "s"
		if count == 1 {
			s = ""
		}
		overrides = fmt.Sprintf(" + %d override%s", count, s)
	}
	if originalTargetEnv != "" {
		where = fmt.Sprintf("%s (%s%s)", where, originalTargetEnv, overrides)
	}
	return
}
