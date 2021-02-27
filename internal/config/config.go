package config

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

type LanguageTarget int8

type JSXOptions struct {
	Parse    bool
	Factory  []string
	Fragment []string
}

type TSOptions struct {
	Parse bool
}

type Platform uint8

const (
	PlatformBrowser Platform = iota
	PlatformNode
	PlatformNeutral
)

type StrictOptions struct {
	// Loose:  "class Foo { foo = 1 }" => "class Foo { constructor() { this.foo = 1; } }"
	// Strict: "class Foo { foo = 1 }" => "class Foo { constructor() { __publicField(this, 'foo', 1); } }"
	//
	// The disadvantage of strictness here is code bloat and performance. The
	// advantage is following the class field specification accurately. For
	// example, loose mode will incorrectly trigger setter methods while strict
	// mode won't.
	ClassFields bool
}

type SourceMap uint8

const (
	SourceMapNone SourceMap = iota
	SourceMapInline
	SourceMapLinkedWithComment
	SourceMapExternalWithoutComment
	SourceMapInlineAndExternal
)

type Loader int

const (
	LoaderNone Loader = iota
	LoaderJS
	LoaderJSX
	LoaderTS
	LoaderTSX
	LoaderJSON
	LoaderText
	LoaderBase64
	LoaderDataURL
	LoaderFile
	LoaderBinary
	LoaderCSS
	LoaderDefault
)

func (loader Loader) IsTypeScript() bool {
	return loader == LoaderTS || loader == LoaderTSX
}

func (loader Loader) CanHaveSourceMap() bool {
	return loader == LoaderJS || loader == LoaderJSX || loader == LoaderTS || loader == LoaderTSX
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

func (f Format) KeepES6ImportExportSyntax() bool {
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
	Loader        Loader
	Contents      string
	SourceFile    string
	AbsResolveDir string
}

type WildcardPattern struct {
	Prefix string
	Suffix string
}

type ExternalModules struct {
	NodeModules map[string]bool
	AbsPaths    map[string]bool
	Patterns    []WildcardPattern
}

type Mode uint8

const (
	ModePassThrough Mode = iota
	ModeConvertFormat
	ModeBundle
)

type Options struct {
	Mode              Mode
	PreserveSymlinks  bool
	RemoveWhitespace  bool
	MinifyIdentifiers bool
	MangleSyntax      bool
	CodeSplitting     bool
	WatchMode         bool

	// Setting this to true disables warnings about code that is very likely to
	// be a bug. This is used to ignore issues inside "node_modules" directories.
	// This has caught real issues in the past. However, it's not esbuild's job
	// to find bugs in other libraries, and these warnings are problematic for
	// people using these libraries with esbuild. The only fix is to either
	// disable all esbuild warnings and not get warnings about your own code, or
	// to try to get the warning fixed in the affected library. This is
	// especially annoying if the warning is a false positive as was the case in
	// https://github.com/firebase/firebase-js-sdk/issues/3814. So these warnings
	// are now disabled for code inside "node_modules" directories.
	SuppressWarningsAboutWeirdCode bool

	// If true, make sure to generate a single file that can be written to stdout
	WriteToStdout bool

	OmitRuntimeForTests     bool
	PreserveUnusedImportsTS bool
	UseDefineForClassFields bool
	ASCIIOnly               bool
	KeepNames               bool
	IgnoreDCEAnnotations    bool

	Defines  *ProcessedDefines
	TS       TSOptions
	JSX      JSXOptions
	Platform Platform

	UnsupportedJSFeatures  compat.JSFeature
	UnsupportedCSSFeatures compat.CSSFeature

	ExtensionOrder  []string
	MainFields      []string
	AbsNodePaths    []string // The "NODE_PATH" variable from Node.js
	ExternalModules ExternalModules

	AbsOutputFile      string
	AbsOutputDir       string
	AbsOutputBase      string
	OutputExtensionJS  string
	OutputExtensionCSS string
	GlobalName         []string
	TsConfigOverride   string
	ExtensionToLoader  map[string]Loader
	OutputFormat       Format
	PublicPath         string
	InjectAbsPaths     []string
	InjectedDefines    []InjectedDefine
	InjectedFiles      []InjectedFile
	Banner             string
	Footer             string

	ChunkPathTemplate []PathTemplate
	AssetPathTemplate []PathTemplate

	Plugins []Plugin

	// If present, metadata about the bundle is written as JSON here
	AbsMetadataFile string

	SourceMap             SourceMap
	ExcludeSourcesContent bool
	UseStrict             bool

	Stdin *StdinInfo
}

type PathPlaceholder uint8

const (
	NoPlaceholder PathPlaceholder = iota

	// The original name of the file, or the manual chunk name, or the name of
	// the type of output file ("entry" or "chunk" or "asset")
	NamePlaceholder

	// A hash of the contents of this file, and the contents and output paths of
	// all dependencies (except for their hash placeholders)
	HashPlaceholder
)

type PathTemplate struct {
	Data        string
	Placeholder PathPlaceholder
}

type PathPlaceholders struct {
	Name *string
	Hash *string
}

func (placeholders PathPlaceholders) Get(placeholder PathPlaceholder) *string {
	switch placeholder {
	case NamePlaceholder:
		return placeholders.Name
	case HashPlaceholder:
		return placeholders.Hash
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
		case NamePlaceholder:
			sb.WriteString("[name]")
		case HashPlaceholder:
			sb.WriteString("[hash]")
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

func IsTreeShakingEnabled(mode Mode, outputFormat Format) bool {
	return mode == ModeBundle || (mode == ModeConvertFormat && outputFormat == FormatIIFE)
}

type InjectedDefine struct {
	Source logger.Source
	Data   js_ast.E
	Name   string
}

type InjectedFile struct {
	Path        string
	SourceIndex uint32
	Exports     []string
	IsDefine    bool
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
	OnResolve []OnResolve
	OnLoad    []OnLoad
}

type OnResolve struct {
	Name      string
	Filter    *regexp.Regexp
	Namespace string
	Callback  func(OnResolveArgs) OnResolveResult
}

type OnResolveArgs struct {
	Path       string
	Importer   logger.Path
	ResolveDir string
	Kind       ast.ImportKind
	PluginData interface{}
}

type OnResolveResult struct {
	PluginName string

	Path       logger.Path
	External   bool
	PluginData interface{}

	Msgs        []logger.Msg
	ThrownError error
}

type OnLoad struct {
	Name      string
	Filter    *regexp.Regexp
	Namespace string
	Callback  func(OnLoadArgs) OnLoadResult
}

type OnLoadArgs struct {
	Path       logger.Path
	PluginData interface{}
}

type OnLoadResult struct {
	PluginName string

	Contents      *string
	AbsResolveDir string
	Loader        Loader
	PluginData    interface{}

	Msgs        []logger.Msg
	ThrownError error
}
