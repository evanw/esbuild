package cache

import (
	"sync"

	"github.com/evanw/esbuild/internal/fs"
)

// This cache uses information from the "stat" syscall to try to avoid re-
// reading files from the file system during subsequent builds if the file
// hasn't changed. The assumption is reading the file metadata is faster than
// reading the file contents.

type FSCache struct {
	entries map[string]*fsEntry
	mutex   sync.Mutex
}

type fsEntry struct {
	contents       string
	modKey         fs.ModKey
	isModKeyUsable bool
}

func (c *FSCache) ReadFile(fs fs.FS, path string) (contents string, canonicalError error, originalError error) {
	entry := func() *fsEntry {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		return c.entries[path]
	}()

	// If the file's modification key hasn't changed since it was cached, assume
	// the contents of the file are also the same and skip reading the file.
	modKey, modKeyErr := fs.ModKey(path)
	if entry != nil && entry.isModKeyUsable && modKeyErr == nil && entry.modKey == modKey {
		return entry.contents, nil, nil
	}

	contents, err, originalError := fs.ReadFile(path)
	if err != nil {
		return "", err, originalError
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.entries[path] = &fsEntry{
		contents:       contents,
		modKey:         modKey,
		isModKeyUsable: modKeyErr == nil,
	}
	return contents, nil, nil
}
