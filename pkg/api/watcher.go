package api

// This file implements a polling file watcher for esbuild (i.e. it detects
// when files are changed by repeatedly checking their contents). Polling is
// used instead of more efficient platform-specific file system APIs because:
//
//   * Go's standard library doesn't have built-in APIs for file watching
//   * Using platform-specific APIs means using cgo, which I want to avoid
//   * Polling is cross-platform and esbuild needs to work on 20+ platforms
//   * Platform-specific APIs might be unreliable and could introduce bugs
//
// That said, this polling system is designed to use relatively little CPU vs.
// a more traditional polling system that scans the whole directory tree at
// once. The file system is still scanned regularly but each scan only checks
// a random subset of your files, which means a change to a file will be picked
// up soon after the change is made but not necessarily instantly.
//
// With the current heuristics, large projects should be completely scanned
// around every 2 seconds so in the worst case it could take up to 2 seconds
// for a change to be noticed. However, after a change has been noticed the
// change's path goes on a short list of recently changed paths which are
// checked on every scan, so further changes to recently changed files should
// be noticed almost instantly.

import (
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
)

// The time to wait between watch intervals
const watchIntervalSleep = 100 * time.Millisecond

// The maximum number of recently-edited items to check every interval
const maxRecentItemCount = 16

// The minimum number of non-recent items to check every interval
const minItemCountPerIter = 64

// The maximum number of intervals before a change is detected
const maxIntervalsBeforeUpdate = 20

type watcher struct {
	data              fs.WatchData
	fs                fs.FS
	rebuild           func() fs.WatchData
	recentItems       []string
	itemsToScan       []string
	mutex             sync.Mutex
	itemsPerIteration int
	shouldStop        int32
	shouldLog         bool
	useColor          logger.UseColor
	stopWaitGroup     sync.WaitGroup
}

func (w *watcher) setWatchData(data fs.WatchData) {
	defer w.mutex.Unlock()
	w.mutex.Lock()

	// Print something for the end of the first build
	if w.shouldLog && w.data.Paths == nil {
		logger.PrintTextWithColor(os.Stderr, w.useColor, func(colors logger.Colors) string {
			return fmt.Sprintf("%s[watch] build finished, watching for changes...%s\n", colors.Dim, colors.Reset)
		})
	}

	w.data = data
	w.itemsToScan = w.itemsToScan[:0] // Reuse memory

	// Remove any recent items that weren't a part of the latest build
	end := 0
	for _, path := range w.recentItems {
		if data.Paths[path] != nil {
			w.recentItems[end] = path
			end++
		}
	}
	w.recentItems = w.recentItems[:end]
}

func (w *watcher) start() {
	w.stopWaitGroup.Add(1)

	go func() {
		// Note: Do not change these log messages without a breaking version change.
		// People want to run regexes over esbuild's stderr stream to look for these
		// messages instead of using esbuild's API.

		for atomic.LoadInt32(&w.shouldStop) == 0 {
			// Sleep for the watch interval
			time.Sleep(watchIntervalSleep)

			// Rebuild if we're dirty
			if absPath := w.tryToFindDirtyPath(); absPath != "" {
				if w.shouldLog {
					logger.PrintTextWithColor(os.Stderr, w.useColor, func(colors logger.Colors) string {
						prettyPath := resolver.PrettyPath(w.fs, logger.Path{Text: absPath, Namespace: "file"})
						return fmt.Sprintf("%s[watch] build started (change: %q)%s\n", colors.Dim, prettyPath, colors.Reset)
					})
				}

				// Run the build
				w.setWatchData(w.rebuild())

				if w.shouldLog {
					logger.PrintTextWithColor(os.Stderr, w.useColor, func(colors logger.Colors) string {
						return fmt.Sprintf("%s[watch] build finished%s\n", colors.Dim, colors.Reset)
					})
				}
			}
		}

		w.stopWaitGroup.Done()
	}()
}

func (w *watcher) stop() {
	atomic.StoreInt32(&w.shouldStop, 1)
	w.stopWaitGroup.Wait()
}

func (w *watcher) tryToFindDirtyPath() string {
	defer w.mutex.Unlock()
	w.mutex.Lock()

	// If we ran out of items to scan, fill the items back up in a random order
	if len(w.itemsToScan) == 0 {
		items := w.itemsToScan[:0] // Reuse memory
		for path := range w.data.Paths {
			items = append(items, path)
		}
		rand.Seed(time.Now().UnixNano())
		for i := int32(len(items) - 1); i > 0; i-- { // Fisher-Yates shuffle
			j := rand.Int31n(i + 1)
			items[i], items[j] = items[j], items[i]
		}
		w.itemsToScan = items

		// Determine how many items to check every iteration, rounded up
		perIter := (len(items) + maxIntervalsBeforeUpdate - 1) / maxIntervalsBeforeUpdate
		if perIter < minItemCountPerIter {
			perIter = minItemCountPerIter
		}
		w.itemsPerIteration = perIter
	}

	// Always check all recent items every iteration
	for i, path := range w.recentItems {
		if dirtyPath := w.data.Paths[path](); dirtyPath != "" {
			// Move this path to the back of the list (i.e. the "most recent" position)
			copy(w.recentItems[i:], w.recentItems[i+1:])
			w.recentItems[len(w.recentItems)-1] = path
			return dirtyPath
		}
	}

	// Check a constant number of items every iteration
	remainingCount := len(w.itemsToScan) - w.itemsPerIteration
	if remainingCount < 0 {
		remainingCount = 0
	}
	toCheck, remaining := w.itemsToScan[remainingCount:], w.itemsToScan[:remainingCount]
	w.itemsToScan = remaining

	// Check if any of the entries in this iteration have been modified
	for _, path := range toCheck {
		if dirtyPath := w.data.Paths[path](); dirtyPath != "" {
			// Mark this item as recent by adding it to the back of the list
			w.recentItems = append(w.recentItems, path)
			if len(w.recentItems) > maxRecentItemCount {
				// Remove items from the front of the list when we hit the limit
				copy(w.recentItems, w.recentItems[1:])
				w.recentItems = w.recentItems[:maxRecentItemCount]
			}
			return dirtyPath
		}
	}
	return ""
}
