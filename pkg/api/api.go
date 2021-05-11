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

type Target uint8

const (
	ESNext Target = iota
	ES5
	ES2015
	ES2016
	ES2017
	ES2018
	ES2019
	ES2020
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
	TreeShakingIgnoreAnnotations
)

////////////////////////////////////////////////////////////////////////////////
// Build API

type BuildOptions struct {
	Color    StderrColor
	LogLimit int
	LogLevel LogLevel

	Sourcemap      SourceMap
	SourceRoot     string
	SourcesContent SourcesContent

	Target  Target
	Engines []Engine

	MinifyWhitespace  bool
	MinifyIdentifiers bool
	MinifySyntax      bool
	Charset           Charset
	TreeShaking       TreeShaking
	LegalComments     LegalComments

	JSXFactory  string
	JSXFragment string

	Define    map[string]string
	Pure      []string
	KeepNames bool

	GlobalName        string
	Bundle            bool
	PreserveSymlinks  bool
	Splitting         bool
	Outfile           string
	Metafile          bool
	Outdir            string
	Outbase           string
	AbsWorkingDir     string
	Platform          Platform
	Format            Format
	External          []string
	MainFields        []string
	Conditions        []string // For the "exports" field in "package.json"
	Loader            map[string]Loader
	ResolveExtensions []string
	Tsconfig          string
	OutExtensions     map[string]string
	PublicPath        string
	Inject            []string
	Banner            map[string]string
	Footer            map[string]string
	NodePaths         []string // The "NODE_PATH" variable from Node.js

	EntryNames string
	ChunkNames string
	AssetNames string

	EntryPoints         []string
	EntryPointsAdvanced []EntryPoint

	Stdin          *StdinOptions
	Write          bool
	AllowOverwrite bool
	Incremental    bool
	Plugins        []Plugin

	Watch *WatchMode
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

func Build(options BuildOptions) BuildResult {
	return buildImpl(options).result
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

type TransformOptions struct {
	Color    StderrColor
	LogLimit int
	LogLevel LogLevel

	Sourcemap      SourceMap
	SourceRoot     string
	SourcesContent SourcesContent

	Target     Target
	Format     Format
	GlobalName string
	Engines    []Engine

	MinifyWhitespace  bool
	MinifyIdentifiers bool
	MinifySyntax      bool
	Charset           Charset
	TreeShaking       TreeShaking
	LegalComments     LegalComments

	JSXFactory  string
	JSXFragment string
	TsconfigRaw string
	Footer      string
	Banner      string

	Define    map[string]string
	Pure      []string
	KeepNames bool

	Sourcefile string
	Loader     Loader
}

type TransformResult struct {
	Errors   []Message
	Warnings []Message

	Code []byte
	Map  []byte
}

func Transform(input string, options TransformOptions) TransformResult {
	return transformImpl(input, options)
}

////////////////////////////////////////////////////////////////////////////////
// Serve API

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

type ServeResult struct {
	Port uint16
	Host string
	Wait func() error
	Stop func()
}

func Serve(serveOptions ServeOptions, buildOptions BuildOptions) (ServeResult, error) {
	return serveImpl(serveOptions, buildOptions)
}

////////////////////////////////////////////////////////////////////////////////
// Plugin API

type Plugin struct {
	Name  string
	Setup func(PluginBuild)
}

type PluginBuild struct {
	InitialOptions  *BuildOptions
	OnStart         func(callback func() (OnStartResult, error))
	OnEnd           func(callback func(result *BuildResult))
	OnResolve       func(options OnResolveOptions, callback func(OnResolveArgs) (OnResolveResult, error))
	OnLoad          func(options OnLoadOptions, callback func(OnLoadArgs) (OnLoadResult, error))
	OnDynamicImport func(options OnDynamicImportOptions, callback func(OnDynamicImportArgs) (OnDynamicImportResult, error))
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

	Path       string
	External   bool
	Namespace  string
	PluginData interface{}

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

type OnDynamicImportOptions struct {
	Filter    string
	Namespace string
}

type OnDynamicImportArgs struct {
	Expression string
	Importer   string
	Namespace  string
	PluginData interface{}
}

type OnDynamicImportResult struct {
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
