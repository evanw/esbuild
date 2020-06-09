package api

type SourceMap uint8

const (
	SourceMapNone SourceMap = iota
	SourceMapInline
	SourceMapLinked
	SourceMapExternal
)

type Target uint8

const (
	ESNext Target = iota
	ES2015
	ES2016
	ES2017
	ES2018
	ES2019
	ES2020
)

type Loader uint8

const (
	LoaderJS Loader = iota
	LoaderJSX
	LoaderTS
	LoaderTSX
	LoaderJSON
	LoaderText
	LoaderBase64
	LoaderDataURL
	LoaderFile
)

type Platform uint8

const (
	PlatformBrowser Platform = iota
	PlatformNode
)

type Format uint8

const (
	FormatDefault Format = iota
	FormatIIFE
	FormatCommonJS
	FormatESModule
)

type Location struct {
	File     string
	Line     int // 1-based
	Column   int // 0-based, in bytes
	Length   int // in bytes
	LineText string
}

type Message struct {
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
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)

////////////////////////////////////////////////////////////////////////////////
// Build API

type BuildOptions struct {
	Color      StderrColor
	ErrorLimit int
	LogLevel   LogLevel

	Sourcemap SourceMap
	Target    Target

	MinifyWhitespace  bool
	MinifyIdentifiers bool
	MinifySyntax      bool

	JSXFactory  string
	JSXFragment string
	Defines     map[string]string

	GlobalName        string
	Bundle            bool
	Outfile           string
	Metafile          string
	Outdir            string
	Platform          Platform
	Format            Format
	Externals         []string
	Loaders           map[string]Loader
	ResolveExtensions []string

	EntryPoints []string
}

type BuildResult struct {
	Errors   []Message
	Warnings []Message

	OutputFiles []OutputFile
}

type OutputFile struct {
	Path     string
	Contents []byte
}

func Build(options BuildOptions) BuildResult {
	return buildImpl(options)
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

type TransformOptions struct {
	Color      StderrColor
	ErrorLimit int
	LogLevel   LogLevel

	Sourcemap SourceMap
	Target    Target

	MinifyWhitespace  bool
	MinifyIdentifiers bool
	MinifySyntax      bool

	JSXFactory  string
	JSXFragment string
	Defines     map[string]string

	Sourcefile string
	Loader     Loader
}

type TransformResult struct {
	Errors   []Message
	Warnings []Message

	JS          []byte
	JSSourceMap []byte
}

func Transform(input string, options TransformOptions) TransformResult {
	return transformImpl(input, options)
}
