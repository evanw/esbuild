package config

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/evanw/esbuild/internal/compat"
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
	RemoveWhitespace  bool
	MinifyIdentifiers bool
	MangleSyntax      bool
	CodeSplitting     bool

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
	InjectedFiles      []InjectedFile
	Banner             string
	Footer             string

	Plugins []Plugin

	// If present, metadata about the bundle is written as JSON here
	AbsMetadataFile string

	SourceMap SourceMap
	Stdin     *StdinInfo
}

type InjectedFile struct {
	Path        string
	SourceIndex uint32
	Exports     []string
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
}

type OnResolveResult struct {
	PluginName string

	Path     logger.Path
	External bool

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
	Path logger.Path
}

type OnLoadResult struct {
	PluginName string

	Contents      *string
	AbsResolveDir string
	Loader        Loader

	Msgs        []logger.Msg
	ThrownError error
}
