# Development

Here are some quick notes about how I develop esbuild.

## Primary workflow

My development workflow revolves around the top-level [`Makefile`](../Makefile), which I use as a script runner.

1. **Build**

Assuming you have [Go](https://go.dev/) installed, you can compile esbuild by running `make` in the top-level directory (or `go build ./cmd/esbuild` if you don't have `make` installed). This creates an executable called `esbuild` (or `esbuild.exe` on Windows).

2. **Test**

You can run the tests written in Go by running `make test-go` in the top-level directory (or `go test ./internal/...` if you don't have `make` installed).

If you want to run more kinds of tests, you can run `make test` instead. This requires installing [node](https://nodejs.org/). And it's possible to run even more tests than that with additional `make` commands (read the [`Makefile`](../Makefile) for details).

3. **Publish**

Here's what I do to publish a new release:

1. Bump the version in [`version.txt`](../version.txt)
2. Copy that version into [`CHANGELOG.md`](../CHANGELOG.md)
3. Run `make publish-all` and follow the prompts

## Running in the browser

If you want to test esbuild in the browser (lets you try out lots of things rapidly), you can:

1. Run `make platform-wasm` to build the WebAssembly version of esbuild
2. Serve the repo directory over HTTP (such as with `./esbuild --servedir=.`)
3. Visit [`/scripts/try.html`](../scripts/try.html) in your browser
