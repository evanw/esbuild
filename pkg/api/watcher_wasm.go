//go:build js && wasm
// +build js,wasm

package api

import (
	"fmt"

	"github.com/evanw/esbuild/internal/fs"
)

// Stub out the file watcher in the WebAssembly build. Watch mode is not
// useful in environments like Cloudflare Workers where there are no
// filesystem change notifications.

type watcher struct{}

func (w *watcher) setWatchData(data fs.WatchData) {}
func (w *watcher) start()                         {}
func (w *watcher) stop()                          {}

func (*internalContext) Watch(options WatchOptions) error {
	return fmt.Errorf("The \"watch\" API is not supported when using WebAssembly")
}
