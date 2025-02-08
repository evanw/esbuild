// This API exposes esbuild's two main operations: building and transforming.
// It's intended for integrating esbuild into other tools as a library.
//
// If you are just trying to run esbuild from Go without the overhead of
// creating a child process, there is also an API for the command-line
// interface itself: https://pkg.go.dev/github.com/evanw/esbuild/pkg/cli.
//
// # Build API
//
// This function runs an end-to-end build operation. It takes an array of file
// paths as entry points, parses them and all of their dependencies, and
// returns the output files to write to the file system. The available options
// roughly correspond to esbuild's command-line flags.
//
// Example usage:
//
//	package main
//
//	import (
//	    "os"
//
//	    "github.com/evanw/esbuild/pkg/api"
//	)
//
//	func main() {
//	    result := api.Build(api.BuildOptions{
//	        EntryPoints: []string{"input.js"},
//	        Outfile:     "output.js",
//	        Bundle:      true,
//	        Write:       true,
//	        LogLevel:    api.LogLevelInfo,
//	    })
//
//	    if len(result.Errors) > 0 {
//	        os.Exit(1)
//	    }
//	}
//
// # Transform API
//
// This function transforms a string of source code into JavaScript. It can be
// used to minify JavaScript, convert TypeScript/JSX to JavaScript, or convert
// newer JavaScript to older JavaScript. The available options roughly
// correspond to esbuild's command-line flags.
//
// Example usage:
//
//	package main
//
//	import (
//	    "fmt"
//	    "os"
//
//	    "github.com/evanw/esbuild/pkg/api"
//	)
//
//	func main() {
//	    jsx := `
//	        import * as React from 'react'
//	        import * as ReactDOM from 'react-dom'
//
//	        ReactDOM.render(
//	            <h1>Hello, world!</h1>,
//	            document.getElementById('root')
//	        );
//	    `
//
//	    result := api.Transform(jsx, api.TransformOptions{
//	        Loader: api.LoaderJSX,
//	    })
//
//	    fmt.Printf("%d errors and %d warnings\n",
//	        len(result.Errors), len(result.Warnings))
//
//	    os.Stdout.Write(result.Code)
//	}
package api

import (
	"time"

	"github.com/evanw/esbuild/internal/logger"
)

type SourceMap uint8

const (
	SourceMapNone SourceMap = iota
	SourceMapInline
	SourceMapLinked
	SourceMapExternal
	SourceMapInlineAndExternal
)

type SourcesContent uint8

const (
	SourcesContentInclude SourcesContent = iota
	SourcesContentExclude
)

type LegalComments uint8

const (
	LegalCommentsDefault LegalComments = iota
	LegalCommentsNone
	LegalCommentsInline
	LegalCommentsEndOfFile
	LegalCommentsLinked
	LegalCommentsExternal
)

type JSX uint8

const (
	JSXTransform JSX = iota
	JSXPreserve
	JSXAutomatic
)

type Target uint8

const (
	DefaultTarget Target = iota
	ESNext
	ES5
	ES2015
	ES2016
	ES2017
	ES2018
	ES2019
	ES2020
	ES2021
	ES2022
	ES2023
	ES2024
)

type Loader uint16

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
	LoaderJSX
	LoaderLocalCSS
	LoaderText
	LoaderTS
	LoaderTSX
)

type Platform uint8

const (
	PlatformDefault Platform = iota
	PlatformBrowser
	PlatformNode
	PlatformNeutral
)

type Format uint8

const (
	FormatDefault Format = iota
	FormatIIFE
	FormatCommonJS
	FormatESModule
)

type Packages uint8

const (
	PackagesDefault Packages = iota
	PackagesBundle
	PackagesExternal
)

type Engine struct {
	Name    EngineName
	Version string
}

type Location struct {
	File       string
	Namespace  string
	Line       int // 1-based
	Column     int // 0-based, in bytes
	Length     int // in bytes
	LineText   string
	Suggestion string
}

type Message struct {
	ID         string
	PluginName string
	Text       string
	Location   *Location
	Notes      []Note

	// Optional user-specified data that is passed through unmodified. You can
	// use this to stash the original error, for example.
	Detail interface{}
}

type Note struct {
	Text     string
	Location *Location
}

type StderrColor uint8

const (
	ColorIfTerminal StderrColor = iota
	ColorNever
	ColorAlways
)

type LogLevel uint8

const (
	LogLevelSilent LogLevel = iota
	LogLevelVerbose
	LogLevelDebug
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)

type Charset uint8

const (
	CharsetDefault Charset = iota
	CharsetASCII
	CharsetUTF8
)

type TreeShaking uint8

const (
	TreeShakingDefault TreeShaking = iota
	TreeShakingFalse
	TreeShakingTrue
)

type Drop uint8

const (
	DropConsole Drop = 1 << iota
	DropDebugger
)

type MangleQuoted uint8

const (
	MangleQuotedFalse MangleQuoted = iota
	MangleQuotedTrue
)

////////////////////////////////////////////////////////////////////////////////
// Build API

type BuildOptions struct {
	Color       StderrColor         // Documentation: https://esbuild.github.io/api/#color
	LogLevel    LogLevel            // Documentation: https://esbuild.github.io/api/#log-level
	LogLimit    int                 // Documentation: https://esbuild.github.io/api/#log-limit
	LogOverride map[string]LogLevel // Documentation: https://esbuild.github.io/api/#log-override

	Sourcemap      SourceMap      // Documentation: https://esbuild.github.io/api/#sourcemap
	SourceRoot     string         // Documentation: https://esbuild.github.io/api/#source-root
	SourcesContent SourcesContent // Documentation: https://esbuild.github.io/api/#sources-content

	Target    Target          // Documentation: https://esbuild.github.io/api/#target
	Engines   []Engine        // Documentation: https://esbuild.github.io/api/#target
	Supported map[string]bool // Documentation: https://esbuild.github.io/api/#supported

	MangleProps       string                 // Documentation: https://esbuild.github.io/api/#mangle-props
	ReserveProps      string                 // Documentation: https://esbuild.github.io/api/#mangle-props
	MangleQuoted      MangleQuoted           // Documentation: https://esbuild.github.io/api/#mangle-props
	MangleCache       map[string]interface{} // Documentation: https://esbuild.github.io/api/#mangle-props
	Drop              Drop                   // Documentation: https://esbuild.github.io/api/#drop
	DropLabels        []string               // Documentation: https://esbuild.github.io/api/#drop-labels
	MinifyWhitespace  bool                   // Documentation: https://esbuild.github.io/api/#minify
	MinifyIdentifiers bool                   // Documentation: https://esbuild.github.io/api/#minify
	MinifySyntax      bool                   // Documentation: https://esbuild.github.io/api/#minify
	LineLimit         int                    // Documentation: https://esbuild.github.io/api/#line-limit
	Charset           Charset                // Documentation: https://esbuild.github.io/api/#charset
	TreeShaking       TreeShaking            // Documentation: https://esbuild.github.io/api/#tree-shaking
	IgnoreAnnotations bool                   // Documentation: https://esbuild.github.io/api/#ignore-annotations
	LegalComments     LegalComments          // Documentation: https://esbuild.github.io/api/#legal-comments

	JSX             JSX    // Documentation: https://esbuild.github.io/api/#jsx-mode
	JSXFactory      string // Documentation: https://esbuild.github.io/api/#jsx-factory
	JSXFragment     string // Documentation: https://esbuild.github.io/api/#jsx-fragment
	JSXImportSource string // Documentation: https://esbuild.github.io/api/#jsx-import-source
	JSXDev          bool   // Documentation: https://esbuild.github.io/api/#jsx-dev
	JSXSideEffects  bool   // Documentation: https://esbuild.github.io/api/#jsx-side-effects

	Define    map[string]string // Documentation: https://esbuild.github.io/api/#define
	Pure      []string          // Documentation: https://esbuild.github.io/api/#pure
	KeepNames bool              // Documentation: https://esbuild.github.io/api/#keep-names

	GlobalName        string            // Documentation: https://esbuild.github.io/api/#global-name
	Bundle            bool              // Documentation: https://esbuild.github.io/api/#bundle
	PreserveSymlinks  bool              // Documentation: https://esbuild.github.io/api/#preserve-symlinks
	Splitting         bool              // Documentation: https://esbuild.github.io/api/#splitting
	Outfile           string            // Documentation: https://esbuild.github.io/api/#outfile
	Metafile          bool              // Documentation: https://esbuild.github.io/api/#metafile
	Outdir            string            // Documentation: https://esbuild.github.io/api/#outdir
	Outbase           string            // Documentation: https://esbuild.github.io/api/#outbase
	AbsWorkingDir     string            // Documentation: https://esbuild.github.io/api/#working-directory
	Platform          Platform          // Documentation: https://esbuild.github.io/api/#platform
	Format            Format            // Documentation: https://esbuild.github.io/api/#format
	External          []string          // Documentation: https://esbuild.github.io/api/#external
	Packages          Packages          // Documentation: https://esbuild.github.io/api/#packages
	Alias             map[string]string // Documentation: https://esbuild.github.io/api/#alias
	MainFields        []string          // Documentation: https://esbuild.github.io/api/#main-fields
	Conditions        []string          // Documentation: https://esbuild.github.io/api/#conditions
	Loader            map[string]Loader // Documentation: https://esbuild.github.io/api/#loader
	ResolveExtensions []string          // Documentation: https://esbuild.github.io/api/#resolve-extensions
	Tsconfig          string            // Documentation: https://esbuild.github.io/api/#tsconfig
	TsconfigRaw       string            // Documentation: https://esbuild.github.io/api/#tsconfig-raw
	OutExtension      map[string]string // Documentation: https://esbuild.github.io/api/#out-extension
	PublicPath        string            // Documentation: https://esbuild.github.io/api/#public-path
	Inject            []string          // Documentation: https://esbuild.github.io/api/#inject
	Banner            map[string]string // Documentation: https://esbuild.github.io/api/#banner
	Footer            map[string]string // Documentation: https://esbuild.github.io/api/#footer
	NodePaths         []string          // Documentation: https://esbuild.github.io/api/#node-paths

	EntryNames string // Documentation: https://esbuild.github.io/api/#entry-names
	ChunkNames string // Documentation: https://esbuild.github.io/api/#chunk-names
	AssetNames string // Documentation: https://esbuild.github.io/api/#asset-names

	EntryPoints         []string     // Documentation: https://esbuild.github.io/api/#entry-points
	EntryPointsAdvanced []EntryPoint // Documentation: https://esbuild.github.io/api/#entry-points

	Stdin          *StdinOptions // Documentation: https://esbuild.github.io/api/#stdin
	Write          bool          // Documentation: https://esbuild.github.io/api/#write
	AllowOverwrite bool          // Documentation: https://esbuild.github.io/api/#allow-overwrite
	Plugins        []Plugin      // Documentation: https://esbuild.github.io/plugins/
}

type EntryPoint struct {
	InputPath  string
	OutputPath string
}

type StdinOptions struct {
	Contents   string
	ResolveDir string
	Sourcefile string
	Loader     Loader
}

type BuildResult struct {
	Errors   []Message
	Warnings []Message

	OutputFiles []OutputFile
	Metafile    string
	MangleCache map[string]interface{}
}

type OutputFile struct {
	Path     string
	Contents []byte
	Hash     string
}

// Documentation: https://esbuild.github.io/api/#build
func Build(options BuildOptions) BuildResult {
	start := time.Now()

	ctx, errors := contextImpl(options)
	if ctx == nil {
		return BuildResult{Errors: errors}
	}

	result := ctx.Rebuild()

	// Print a summary of the generated files to stderr. Except don't do
	// this if the terminal is already being used for something else.
	if ctx.args.logOptions.LogLevel <= logger.LevelInfo && !ctx.args.options.WriteToStdout {
		printSummary(ctx.args.logOptions.Color, result.OutputFiles, start)
	}

	ctx.Dispose()
	return result
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

type TransformOptions struct {
	Color       StderrColor         // Documentation: https://esbuild.github.io/api/#color
	LogLevel    LogLevel            // Documentation: https://esbuild.github.io/api/#log-level
	LogLimit    int                 // Documentation: https://esbuild.github.io/api/#log-limit
	LogOverride map[string]LogLevel // Documentation: https://esbuild.github.io/api/#log-override

	Sourcemap      SourceMap      // Documentation: https://esbuild.github.io/api/#sourcemap
	SourceRoot     string         // Documentation: https://esbuild.github.io/api/#source-root
	SourcesContent SourcesContent // Documentation: https://esbuild.github.io/api/#sources-content

	Target    Target          // Documentation: https://esbuild.github.io/api/#target
	Engines   []Engine        // Documentation: https://esbuild.github.io/api/#target
	Supported map[string]bool // Documentation: https://esbuild.github.io/api/#supported

	Platform   Platform // Documentation: https://esbuild.github.io/api/#platform
	Format     Format   // Documentation: https://esbuild.github.io/api/#format
	GlobalName string   // Documentation: https://esbuild.github.io/api/#global-name

	MangleProps       string                 // Documentation: https://esbuild.github.io/api/#mangle-props
	ReserveProps      string                 // Documentation: https://esbuild.github.io/api/#mangle-props
	MangleQuoted      MangleQuoted           // Documentation: https://esbuild.github.io/api/#mangle-props
	MangleCache       map[string]interface{} // Documentation: https://esbuild.github.io/api/#mangle-props
	Drop              Drop                   // Documentation: https://esbuild.github.io/api/#drop
	DropLabels        []string               // Documentation: https://esbuild.github.io/api/#drop-labels
	MinifyWhitespace  bool                   // Documentation: https://esbuild.github.io/api/#minify
	MinifyIdentifiers bool                   // Documentation: https://esbuild.github.io/api/#minify
	MinifySyntax      bool                   // Documentation: https://esbuild.github.io/api/#minify
	LineLimit         int                    // Documentation: https://esbuild.github.io/api/#line-limit
	Charset           Charset                // Documentation: https://esbuild.github.io/api/#charset
	TreeShaking       TreeShaking            // Documentation: https://esbuild.github.io/api/#tree-shaking
	IgnoreAnnotations bool                   // Documentation: https://esbuild.github.io/api/#ignore-annotations
	LegalComments     LegalComments          // Documentation: https://esbuild.github.io/api/#legal-comments

	JSX             JSX    // Documentation: https://esbuild.github.io/api/#jsx
	JSXFactory      string // Documentation: https://esbuild.github.io/api/#jsx-factory
	JSXFragment     string // Documentation: https://esbuild.github.io/api/#jsx-fragment
	JSXImportSource string // Documentation: https://esbuild.github.io/api/#jsx-import-source
	JSXDev          bool   // Documentation: https://esbuild.github.io/api/#jsx-dev
	JSXSideEffects  bool   // Documentation: https://esbuild.github.io/api/#jsx-side-effects

	TsconfigRaw string // Documentation: https://esbuild.github.io/api/#tsconfig-raw
	Banner      string // Documentation: https://esbuild.github.io/api/#banner
	Footer      string // Documentation: https://esbuild.github.io/api/#footer

	Define    map[string]string // Documentation: https://esbuild.github.io/api/#define
	Pure      []string          // Documentation: https://esbuild.github.io/api/#pure
	KeepNames bool              // Documentation: https://esbuild.github.io/api/#keep-names

	Sourcefile string // Documentation: https://esbuild.github.io/api/#sourcefile
	Loader     Loader // Documentation: https://esbuild.github.io/api/#loader
}

type TransformResult struct {
	Errors   []Message
	Warnings []Message

	Code          []byte
	Map           []byte
	LegalComments []byte

	MangleCache map[string]interface{}
}

// Documentation: https://esbuild.github.io/api/#transform
func Transform(input string, options TransformOptions) TransformResult {
	return transformImpl(input, options)
}

////////////////////////////////////////////////////////////////////////////////
// Context API

// Documentation: https://esbuild.github.io/api/#serve-arguments
type ServeOptions struct {
	Port      int
	Host      string
	Servedir  string
	Keyfile   string
	Certfile  string
	Fallback  string
	OnRequest func(ServeOnRequestArgs)
}

type ServeOnRequestArgs struct {
	RemoteAddress string
	Method        string
	Path          string
	Status        int
	TimeInMS      int // The time to generate the response, not to send it
}

// Documentation: https://esbuild.github.io/api/#serve-return-values
type ServeResult struct {
	Port  uint16
	Hosts []string
}

type WatchOptions struct {
}

type BuildContext interface {
	// Documentation: https://esbuild.github.io/api/#rebuild
	Rebuild() BuildResult

	// Documentation: https://esbuild.github.io/api/#watch
	Watch(options WatchOptions) error

	// Documentation: https://esbuild.github.io/api/#serve
	Serve(options ServeOptions) (ServeResult, error)

	Cancel()
	Dispose()
}

type ContextError struct {
	Errors []Message // Option validation errors are returned here
}

func (err *ContextError) Error() string {
	if len(err.Errors) > 0 {
		return err.Errors[0].Text
	}
	return "Context creation failed"
}

// Documentation: https://esbuild.github.io/api/#build
func Context(buildOptions BuildOptions) (BuildContext, *ContextError) {
	ctx, errors := contextImpl(buildOptions)
	if ctx == nil {
		return nil, &ContextError{Errors: errors}
	}
	return ctx, nil
}

////////////////////////////////////////////////////////////////////////////////
// Plugin API

type SideEffects uint8

const (
	SideEffectsTrue SideEffects = iota
	SideEffectsFalse
)

type Plugin struct {
	Name  string
	Setup func(PluginBuild)
}

type PluginBuild struct {
	// Documentation: https://esbuild.github.io/plugins/#build-options
	InitialOptions *BuildOptions

	// Documentation: https://esbuild.github.io/plugins/#resolve
	Resolve func(path string, options ResolveOptions) ResolveResult

	// Documentation: https://esbuild.github.io/plugins/#on-start
	OnStart func(callback func() (OnStartResult, error))

	// Documentation: https://esbuild.github.io/plugins/#on-end
	OnEnd func(callback func(result *BuildResult) (OnEndResult, error))

	// Documentation: https://esbuild.github.io/plugins/#on-resolve
	OnResolve func(options OnResolveOptions, callback func(OnResolveArgs) (OnResolveResult, error))

	// Documentation: https://esbuild.github.io/plugins/#on-load
	OnLoad func(options OnLoadOptions, callback func(OnLoadArgs) (OnLoadResult, error))

	// Documentation: https://esbuild.github.io/plugins/#on-dispose
	OnDispose func(callback func())
}

// Documentation: https://esbuild.github.io/plugins/#resolve-options
type ResolveOptions struct {
	PluginName string
	Importer   string
	Namespace  string
	ResolveDir string
	Kind       ResolveKind
	PluginData interface{}
	With       map[string]string
}

// Documentation: https://esbuild.github.io/plugins/#resolve-results
type ResolveResult struct {
	Errors   []Message
	Warnings []Message

	Path        string
	External    bool
	SideEffects bool
	Namespace   string
	Suffix      string
	PluginData  interface{}
}

type OnStartResult struct {
	Errors   []Message
	Warnings []Message
}

type OnEndResult struct {
	Errors   []Message
	Warnings []Message
}

// Documentation: https://esbuild.github.io/plugins/#on-resolve-options
type OnResolveOptions struct {
	Filter    string
	Namespace string
}

// Documentation: https://esbuild.github.io/plugins/#on-resolve-arguments
type OnResolveArgs struct {
	Path       string
	Importer   string
	Namespace  string
	ResolveDir string
	Kind       ResolveKind
	PluginData interface{}
	With       map[string]string
}

// Documentation: https://esbuild.github.io/plugins/#on-resolve-results
type OnResolveResult struct {
	PluginName string

	Errors   []Message
	Warnings []Message

	Path        string
	External    bool
	SideEffects SideEffects
	Namespace   string
	Suffix      string
	PluginData  interface{}

	WatchFiles []string
	WatchDirs  []string
}

// Documentation: https://esbuild.github.io/plugins/#on-load-options
type OnLoadOptions struct {
	Filter    string
	Namespace string
}

// Documentation: https://esbuild.github.io/plugins/#on-load-arguments
type OnLoadArgs struct {
	Path       string
	Namespace  string
	Suffix     string
	PluginData interface{}
	With       map[string]string
}

// Documentation: https://esbuild.github.io/plugins/#on-load-results
type OnLoadResult struct {
	PluginName string

	Errors   []Message
	Warnings []Message

	Contents   *string
	ResolveDir string
	Loader     Loader
	PluginData interface{}

	WatchFiles []string
	WatchDirs  []string
}

type ResolveKind uint8

const (
	ResolveNone ResolveKind = iota
	ResolveEntryPoint
	ResolveJSImportStatement
	ResolveJSRequireCall
	ResolveJSDynamicImport
	ResolveJSRequireResolve
	ResolveCSSImportRule
	ResolveCSSComposesFrom
	ResolveCSSURLToken
)

////////////////////////////////////////////////////////////////////////////////
// FormatMessages API

type MessageKind uint8

const (
	ErrorMessage MessageKind = iota
	WarningMessage
)

type FormatMessagesOptions struct {
	TerminalWidth int
	Kind          MessageKind
	Color         bool
}

func FormatMessages(msgs []Message, opts FormatMessagesOptions) []string {
	return formatMsgsImpl(msgs, opts)
}

////////////////////////////////////////////////////////////////////////////////
// AnalyzeMetafile API

type AnalyzeMetafileOptions struct {
	Color   bool
	Verbose bool
}

// Documentation: https://esbuild.github.io/api/#analyze
func AnalyzeMetafile(metafile string, opts AnalyzeMetafileOptions) string {
	return analyzeMetafileImpl(metafile, opts)
}
