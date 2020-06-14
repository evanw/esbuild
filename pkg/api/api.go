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
//         "fmt"
//         "io/ioutil"
//
//         "github.com/evanw/esbuild/pkg/api"
//     )
//
//     func main() {
//         result := api.Build(api.BuildOptions{
//             EntryPoints: []string{"input.js"},
//             Outfile:     "output.js",
//             Bundle:      true,
//         })
//
//         fmt.Printf("%d errors and %d warnings\n",
//             len(result.Errors), len(result.Warnings))
//
//         for _, out := range result.OutputFiles {
//             ioutil.WriteFile(out.Path, out.Contents, 0644)
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
//         os.Stdout.Write(result.JS)
//     }
//
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
	Origindir         string
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
