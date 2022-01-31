// This API exposes esbuild's two main operations: building and transforming.
// It's intended for integrating esbuild into other tools as a library.
//
// If you are just trying to run esbuild from Go without the overhead of
// creating a child process, there is also an API for the command-line
// interface itself: https://godoc.org/github.com/evanw/esbuild/pkg/cli.
//
// Build API
//
// This function runs an end-to-end build operation. It takes an array of file
// paths as entry points, parses them and all of their dependencies, and
// returns the output files to write to the file system. The available options
// roughly correspond to esbuild's command-line flags.
//
// Example usage:
//
//     package main
//
//     import (
//         "os"
//
//         "github.com/evanw/esbuild/pkg/api"
//     )
//
//     func main() {
//         result := api.Build(api.BuildOptions{
//             EntryPoints: []string{"input.js"},
//             Outfile:     "output.js",
//             Bundle:      true,
//             Write:       true,
//             LogLevel:    api.LogLevelInfo,
//         })
//
//         if len(result.Errors) > 0 {
//             os.Exit(1)
//         }
//     }
//
// Transform API
//
// This function transforms a string of source code into JavaScript. It can be
// used to minify JavaScript, convert TypeScript/JSX to JavaScript, or convert
// newer JavaScript to older JavaScript. The available options roughly
// correspond to esbuild's command-line flags.
//
// Example usage:
//
//     package main
//
//     import (
//         "fmt"
//         "os"
//
//         "github.com/evanw/esbuild/pkg/api"
//     )
//
//     func main() {
//         jsx := `
//             import * as React from 'react'
//             import * as ReactDOM from 'react-dom'
//
//             ReactDOM.render(
//                 <h1>Hello, world!</h1>,
//                 document.getElementById('root')
//             );
//         `
//
//         result := api.Transform(jsx, api.TransformOptions{
//             Loader: api.LoaderJSX,
//         })
//
//         fmt.Printf("%d errors and %d warnings\n",
//             len(result.Errors), len(result.Warnings))
//
//         os.Stdout.Write(result.Code)
//     }
//
package api

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

type JSXMode uint8

const (
	JSXModeTransform JSXMode = iota
	JSXModePreserve
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
)

type Loader uint8

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

type Platform uint8

const (
	PlatformBrowser Platform = iota
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

type EngineName uint8

const (
	EngineChrome EngineName = iota
	EngineEdge
	EngineFirefox
	EngineIOS
	EngineNode
	EngineSafari
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

////////////////////////////////////////////////////////////////////////////////
// Build API

type BuildOptions struct {
	Color    StderrColor // Documentation: https://esbuild.github.io/api/#color
	LogLimit int         // Documentation: https://esbuild.github.io/api/#log-limit
	LogLevel LogLevel    // Documentation: https://esbuild.github.io/api/#log-level

	Sourcemap      SourceMap      // Documentation: https://esbuild.github.io/api/#sourcemap
	SourceRoot     string         // Documentation: https://esbuild.github.io/api/#source-root
	SourcesContent SourcesContent // Documentation: https://esbuild.github.io/api/#sources-content

	Target  Target   // Documentation: https://esbuild.github.io/api/#target
	Engines []Engine // Documentation: https://esbuild.github.io/api/#target

	MangleProps       string        // Documentation: https://esbuild.github.io/api/#mangle-props
	ReserveProps      string        // Documentation: https://esbuild.github.io/api/#mangle-props
	Drop              Drop          // Documentation: https://esbuild.github.io/api/#drop
	MinifyWhitespace  bool          // Documentation: https://esbuild.github.io/api/#minify
	MinifyIdentifiers bool          // Documentation: https://esbuild.github.io/api/#minify
	MinifySyntax      bool          // Documentation: https://esbuild.github.io/api/#minify
	Charset           Charset       // Documentation: https://esbuild.github.io/api/#charset
	TreeShaking       TreeShaking   // Documentation: https://esbuild.github.io/api/#tree-shaking
	IgnoreAnnotations bool          // Documentation: https://esbuild.github.io/api/#ignore-annotations
	LegalComments     LegalComments // Documentation: https://esbuild.github.io/api/#legal-comments

	JSXMode     JSXMode // Documentation: https://esbuild.github.io/api/#jsx-mode
	JSXFactory  string  // Documentation: https://esbuild.github.io/api/#jsx-factory
	JSXFragment string  // Documentation: https://esbuild.github.io/api/#jsx-fragment

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
	MainFields        []string          // Documentation: https://esbuild.github.io/api/#main-fields
	Conditions        []string          // Documentation: https://esbuild.github.io/api/#conditions
	Loader            map[string]Loader // Documentation: https://esbuild.github.io/api/#loader
	ResolveExtensions []string          // Documentation: https://esbuild.github.io/api/#resolve-extensions
	Tsconfig          string            // Documentation: https://esbuild.github.io/api/#tsconfig
	OutExtensions     map[string]string // Documentation: https://esbuild.github.io/api/#out-extension
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
	Incremental    bool          // Documentation: https://esbuild.github.io/api/#incremental
	Plugins        []Plugin      // Documentation: https://esbuild.github.io/plugins/

	Watch *WatchMode // Documentation: https://esbuild.github.io/api/#watch
}

type EntryPoint struct {
	InputPath  string
	OutputPath string
}

type WatchMode struct {
	OnRebuild func(BuildResult)
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

	Rebuild func() BuildResult // Only when "Incremental: true"
	Stop    func()             // Only when "Watch: true"
}

type OutputFile struct {
	Path     string
	Contents []byte
}

// Documentation: https://esbuild.github.io/api/#build-api
func Build(options BuildOptions) BuildResult {
	return buildImpl(options).result
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

type TransformOptions struct {
	Color    StderrColor // Documentation: https://esbuild.github.io/api/#color
	LogLimit int         // Documentation: https://esbuild.github.io/api/#log-limit
	LogLevel LogLevel    // Documentation: https://esbuild.github.io/api/#log-level

	Sourcemap      SourceMap      // Documentation: https://esbuild.github.io/api/#sourcemap
	SourceRoot     string         // Documentation: https://esbuild.github.io/api/#source-root
	SourcesContent SourcesContent // Documentation: https://esbuild.github.io/api/#sources-content

	Target  Target   // Documentation: https://esbuild.github.io/api/#target
	Engines []Engine // Documentation: https://esbuild.github.io/api/#target

	Format     Format // Documentation: https://esbuild.github.io/api/#format
	GlobalName string // Documentation: https://esbuild.github.io/api/#global-name

	MangleProps       string        // Documentation: https://esbuild.github.io/api/#mangle-props
	ReserveProps      string        // Documentation: https://esbuild.github.io/api/#mangle-props
	Drop              Drop          // Documentation: https://esbuild.github.io/api/#drop
	MinifyWhitespace  bool          // Documentation: https://esbuild.github.io/api/#minify
	MinifyIdentifiers bool          // Documentation: https://esbuild.github.io/api/#minify
	MinifySyntax      bool          // Documentation: https://esbuild.github.io/api/#minify
	Charset           Charset       // Documentation: https://esbuild.github.io/api/#charset
	TreeShaking       TreeShaking   // Documentation: https://esbuild.github.io/api/#tree-shaking
	IgnoreAnnotations bool          // Documentation: https://esbuild.github.io/api/#ignore-annotations
	LegalComments     LegalComments // Documentation: https://esbuild.github.io/api/#legal-comments

	JSXMode     JSXMode // Documentation: https://esbuild.github.io/api/#jsx
	JSXFactory  string  // Documentation: https://esbuild.github.io/api/#jsx-factory
	JSXFragment string  // Documentation: https://esbuild.github.io/api/#jsx-fragment

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

	Code []byte
	Map  []byte
}

// Documentation: https://esbuild.github.io/api/#transform-api
func Transform(input string, options TransformOptions) TransformResult {
	return transformImpl(input, options)
}

////////////////////////////////////////////////////////////////////////////////
// Serve API

// Documentation: https://esbuild.github.io/api/#serve-arguments
type ServeOptions struct {
	Port      uint16
	Host      string
	Servedir  string
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
	Port uint16
	Host string
	Wait func() error
	Stop func()
}

// Documentation: https://esbuild.github.io/api/#serve
func Serve(serveOptions ServeOptions, buildOptions BuildOptions) (ServeResult, error) {
	return serveImpl(serveOptions, buildOptions)
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
	InitialOptions *BuildOptions
	Resolve        func(path string, options ResolveOptions) ResolveResult
	OnStart        func(callback func() (OnStartResult, error))
	OnEnd          func(callback func(result *BuildResult))
	OnResolve      func(options OnResolveOptions, callback func(OnResolveArgs) (OnResolveResult, error))
	OnLoad         func(options OnLoadOptions, callback func(OnLoadArgs) (OnLoadResult, error))
}

type ResolveOptions struct {
	PluginName string
	Importer   string
	Namespace  string
	ResolveDir string
	Kind       ResolveKind
	PluginData interface{}
}

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

type OnResolveOptions struct {
	Filter    string
	Namespace string
}

type OnResolveArgs struct {
	Path       string
	Importer   string
	Namespace  string
	ResolveDir string
	Kind       ResolveKind
	PluginData interface{}
}

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

type OnLoadOptions struct {
	Filter    string
	Namespace string
}

type OnLoadArgs struct {
	Path       string
	Namespace  string
	Suffix     string
	PluginData interface{}
}

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
	ResolveEntryPoint ResolveKind = iota
	ResolveJSImportStatement
	ResolveJSRequireCall
	ResolveJSDynamicImport
	ResolveJSRequireResolve
	ResolveCSSImportRule
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
