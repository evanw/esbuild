package config

type LanguageTarget int8

const (
	// These are arranged such that ESNext is the default zero value and such
	// that earlier releases are less than later releases
	ES2015 = -6
	ES2016 = -5
	ES2017 = -4
	ES2018 = -3
	ES2019 = -2
	ES2020 = -1
	ESNext = 0
)

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
	// Loose:  "a ?? b" => "a != null ? a : b"
	// Strict: "a ?? b" => "a !== null && a !== void 0 ? a : b"
	//
	// The disadvantage of strictness here is code bloat. The only observable
	// difference between the two is when the left operand is the bizarre legacy
	// value "document.all". This value is special-cased in the standard for
	// legacy reasons such that "document.all != null" is false even though it's
	// not "null" or "undefined".
	NullishCoalescing bool

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
)

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
	// If the optional ModuleName is configured, then we'll write out this:
	//
	//   let moduleName = (() => {
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

type StdinInfo struct {
	Loader     Loader
	Contents   string
	SourceFile string
}

type Options struct {
	// true: imports are scanned and bundled along with the file
	// false: imports are left alone and the file is passed through as-is
	IsBundling bool

	RemoveWhitespace  bool
	MinifyIdentifiers bool
	MangleSyntax      bool
	CodeSplitting     bool

	// If true, make sure to generate a single file that can be written to stdout
	WriteToStdout bool

	OmitRuntimeForTests bool

	Strict   StrictOptions
	Defines  *ProcessedDefines
	TS       TSOptions
	JSX      JSXOptions
	Target   LanguageTarget
	Platform Platform

	ExtensionOrder  []string
	ExternalModules map[string]bool

	AbsOutputFile     string
	AbsOutputDir      string
	ModuleName        string
	ExtensionToLoader map[string]Loader
	OutputFormat      Format

	// If present, metadata about the bundle is written as JSON here
	AbsMetadataFile string

	SourceMap SourceMap
	Stdin     *StdinInfo
}
