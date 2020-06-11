# Go API Documentation

There are two Go modules meant for public use: the [API module](#api-module) and the [CLI module](#cli-module). Note that the Go code under the [`internal`](../internal) directory is not meant to be depended upon (it sometimes changes in backwards-incompatible ways).

## API Module

Install: `go get github.com/evanw/esbuild/pkg/api`

### `func Build(options BuildOptions) BuildResult`

This function runs an end-to-end build operation. It takes an array of file paths as entry points, parses them and all of their dependencies, and returns the output files to write to the file system. The available options roughly correspond to esbuild's command-line flags. See [pkg/api/api.go](../pkg/api/api.go) for the complete list of options.

Example usage:

```go
package main

import (
	"fmt"
	"io/ioutil"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{"input.js"},
		Outfile:     "output.js",
		Bundle:      true,
	})

	fmt.Printf("%d errors and %d warnings\n",
		len(result.Errors), len(result.Warnings))

	for _, out := range result.OutputFiles {
		ioutil.WriteFile(out.Path, out.Contents, 0644)
	}
}
```

### `func Transform(input string, options TransformOptions) TransformResult`

This function transforms a string of source code into JavaScript. It can be used to minify JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript to older JavaScript. The available options roughly correspond to esbuild's command-line flags. See [pkg/api/api.go](../pkg/api/api.go) for the complete list of options.

Example usage:

```go
package main

import (
	"fmt"
	"os"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	jsx := `
		import * as React from 'react'
		import * as ReactDOM from 'react-dom'

		ReactDOM.render(
			<h1>Hello, world!</h1>,
			document.getElementById('root')
		);
	`

	result := api.Transform(jsx, api.TransformOptions{
		Loader: api.LoaderJSX,
	})

	fmt.Printf("%d errors and %d warnings\n",
		len(result.Errors), len(result.Warnings))

	os.Stdout.Write(result.JS)
}
```

## CLI Module

Install: `go get github.com/evanw/esbuild/pkg/cli`

### `func Run(osArgs []string) int`

This function invokes the esbuild CLI. It takes an array of command-line arguments (excluding the executable argument itself) and returns an exit code. There are some minor differences between this CLI and the actual `esbuild` executable such as the lack of auxiliary flags (e.g. `--help` and `--version`) but it is otherwise exactly the same code. This could be useful if you need to wrap esbuild's CLI using Go code and want to compile everything into a single executable.

Example usage:

```go
package main

import (
	"os"

	"github.com/evanw/esbuild/pkg/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
```

### `func ParseBuildOptions(osArgs []string) (api.BuildOptions, error)`

This parses an array of strings into an options object suitable for passing to `api.Build()`. Use this if you need to reuse the same argument parsing logic as the esbuild CLI.

Example usage:

```go
options, err := cli.ParseBuildOptions([]string{
	"--bundle", "--minify", "--format=esm"})
if err != nil {
	log.Fatal(err)
}
```

### `func ParseTransformOptions(osArgs []string) (api.TransformOptions, error)`

This parses an array of strings into an options object suitable for passing to `api.Transform()`. Use this if you need to reuse the same argument parsing logic as the esbuild CLI.

Example usage:

```go
options, err := cli.ParseTransformOptions([]string{
	"--minify", "--loader=tsx", "--define:DEBUG=false"})
if err != nil {
	log.Fatal(err)
}
```
