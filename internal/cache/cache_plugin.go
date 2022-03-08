package cache

import (
	"github.com/evanw/esbuild/internal/config"
	"sync"
)

type PluginCache struct {
	loadEntries map[string]*config.OnLoadResult
	mutex       sync.Mutex
}

func (c *PluginCache) GetLoadCache(path string) *config.OnLoadResult {
	entry := func() *config.OnLoadResult {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		return c.loadEntries[path]
	}()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return entry
}

func (c *PluginCache) SetLoadCache(path string, res *config.OnLoadResult) {
	c.loadEntries[path] = res
}
