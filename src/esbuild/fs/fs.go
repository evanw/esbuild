package fs

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"
)

type FS interface {
	// The returned map is immutable and is cached across invocations. Do not
	// mutate it.
	ReadDirectory(path string) map[string]bool
	ReadFile(path string) (string, bool)

	// This is part of the interface because the mock interface used for tests
	// should not depend on file system behavior (i.e. different slashes for
	// Windows) while the real interface should.
	Dir(path string) string
	Base(path string) string
	Join(parts ...string) string
	RelativeToCwd(path string) (string, bool)
}

////////////////////////////////////////////////////////////////////////////////

type mockFS struct {
	dirs  map[string]map[string]bool
	files map[string]string
}

func MockFS(input map[string]string) FS {
	dirs := make(map[string]map[string]bool)
	files := make(map[string]string)

	for k, v := range input {
		files[k] = v
		original := k

		// Build the directory map
		for {
			kDir := path.Dir(k)
			dir, ok := dirs[kDir]
			if !ok {
				dir = make(map[string]bool)
				dirs[kDir] = dir
			}
			if k == original {
				dir[path.Base(k)] = true
			}
			if kDir == k {
				break
			}
			k = kDir
		}
	}

	return &mockFS{dirs, files}
}

func (fs *mockFS) ReadDirectory(path string) map[string]bool {
	return fs.dirs[path]
}

func (fs *mockFS) ReadFile(path string) (string, bool) {
	contents, ok := fs.files[path]
	return contents, ok
}

func (*mockFS) Dir(p string) string {
	return path.Dir(p)
}

func (*mockFS) Base(p string) string {
	return path.Base(p)
}

func (*mockFS) Join(parts ...string) string {
	return path.Join(parts...)
}

func (*mockFS) RelativeToCwd(path string) (string, bool) {
	return "", false
}

////////////////////////////////////////////////////////////////////////////////

type realFS struct {
	// Stores the file entries for directories we've listed before
	fileEntriesMutex sync.RWMutex
	fileEntries      map[string]map[string]bool

	// Stores the contents of files we've read before
	fileContentsMutex sync.RWMutex
	fileContents      map[string]*string

	// For the current working directory
	cwd   string
	cwdOk bool
}

func RealFS() FS {
	cwd, cwdErr := os.Getwd()
	return &realFS{
		fileEntries:  make(map[string]map[string]bool),
		fileContents: make(map[string]*string),
		cwd:          cwd,
		cwdOk:        cwdErr == nil,
	}
}

func (fs *realFS) ReadDirectory(dir string) map[string]bool {
	// First, check the cache
	cached, ok := func() (map[string]bool, bool) {
		fs.fileEntriesMutex.RLock()
		defer fs.fileEntriesMutex.RUnlock()
		cached, ok := fs.fileEntries[dir]
		return cached, ok
	}()

	// Cache hit: stop now
	if ok {
		return cached
	}

	// Cache miss: read the directory entries
	names, err := readdir(dir)
	fileEntries := make(map[string]bool)
	if err == nil {
		for _, name := range names {
			// Use "stat", not "lstat", because we want to follow symbolic links
			if stat, err := os.Stat(filepath.Join(dir, name)); err == nil && !stat.IsDir() {
				fileEntries[name] = true
			}
		}
	}

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	fs.fileEntriesMutex.Lock()
	defer fs.fileEntriesMutex.Unlock()
	if err != nil {
		fs.fileEntries[dir] = nil
		return nil
	}
	fs.fileEntries[dir] = fileEntries
	return fileEntries
}

func (fs *realFS) ReadFile(path string) (string, bool) {
	// First, check the cache
	cached, ok := func() (*string, bool) {
		fs.fileContentsMutex.RLock()
		defer fs.fileContentsMutex.RUnlock()
		cached, ok := fs.fileContents[path]
		return cached, ok
	}()

	// Cache hit: stop now
	if ok {
		if cached == nil {
			return "", false
		}
		return *cached, true
	}

	// Cache miss: read the file
	buffer, err := ioutil.ReadFile(path)

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The file is inaccessible so trying again is wasted.
	fs.fileContentsMutex.Lock()
	defer fs.fileContentsMutex.Unlock()
	if err != nil {
		fs.fileContents[path] = nil
		return "", false
	}
	contents := string(buffer)
	fs.fileContents[path] = &contents
	return contents, true
}

func (*realFS) Dir(p string) string {
	return filepath.Dir(p)
}

func (*realFS) Base(p string) string {
	return filepath.Base(p)
}

func (*realFS) Join(parts ...string) string {
	return filepath.Join(parts...)
}

func (fs *realFS) RelativeToCwd(path string) (string, bool) {
	if fs.cwdOk {
		if rel, err := filepath.Rel(fs.cwd, path); err == nil {
			return rel, true
		}
	}
	return "", false
}

func readdir(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}
