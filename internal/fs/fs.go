package fs

// Most of esbuild's internals use this file system abstraction instead of
// using native file system APIs. This lets us easily mock the file system
// for tests and also implement Yarn's virtual ".zip" file system overlay.

import (
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
)

type EntryKind uint8

const (
	DirEntry  EntryKind = 1
	FileEntry EntryKind = 2
)

type Entry struct {
	symlink  string
	dir      string
	base     string
	mutex    sync.Mutex
	kind     EntryKind
	needStat bool
}

func (e *Entry) Kind(fs FS) EntryKind {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.needStat {
		e.needStat = false
		e.symlink, e.kind = fs.kind(e.dir, e.base)
	}
	return e.kind
}

func (e *Entry) Symlink(fs FS) string {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.needStat {
		e.needStat = false
		e.symlink, e.kind = fs.kind(e.dir, e.base)
	}
	return e.symlink
}

type accessedEntries struct {
	wasPresent map[string]bool

	// If this is nil, "SortedKeys()" was not accessed. This means we should
	// check for whether this directory has changed or not by seeing if any of
	// the entries in the "wasPresent" map have changed in "present or not"
	// status, since the only access was to individual entries via "Get()".
	//
	// If this is non-nil, "SortedKeys()" was accessed. This means we should
	// check for whether this directory has changed or not by checking the
	// "allEntries" array for equality with the existing entries list, since the
	// code asked for all entries and may have used the presence or absence of
	// entries in that list.
	//
	// The goal of having these two checks is to be as narrow as possible to
	// avoid unnecessary rebuilds. If only "Get()" is called on a few entries,
	// then we won't invalidate the build if random unrelated entries are added
	// or removed. But if "SortedKeys()" is called, we need to invalidate the
	// build if anything about the set of entries in this directory is changed.
	allEntries []string

	mutex sync.Mutex
}

type DirEntries struct {
	data            map[string]*Entry
	accessedEntries *accessedEntries
	dir             string
}

func MakeEmptyDirEntries(dir string) DirEntries {
	return DirEntries{dir: dir, data: make(map[string]*Entry)}
}

type DifferentCase struct {
	Dir    string
	Query  string
	Actual string
}

func (entries DirEntries) Get(query string) (*Entry, *DifferentCase) {
	if entries.data != nil {
		key := strings.ToLower(query)
		entry := entries.data[key]

		// Track whether this specific entry was present or absent for watch mode
		if accessed := entries.accessedEntries; accessed != nil {
			accessed.mutex.Lock()
			accessed.wasPresent[key] = entry != nil
			accessed.mutex.Unlock()
		}

		if entry != nil {
			if entry.base != query {
				return entry, &DifferentCase{
					Dir:    entries.dir,
					Query:  query,
					Actual: entry.base,
				}
			}
			return entry, nil
		}
	}

	return nil, nil
}

// This function lets you "peek" at the number of entries without watch mode
// considering the number of entries as having been observed. This is used when
// generating debug log messages to log the number of entries without causing
// watch mode to rebuild when the number of entries has been changed.
func (entries DirEntries) PeekEntryCount() int {
	if entries.data != nil {
		return len(entries.data)
	}
	return 0
}

func (entries DirEntries) SortedKeys() (keys []string) {
	if entries.data != nil {
		keys = make([]string, 0, len(entries.data))
		for _, entry := range entries.data {
			keys = append(keys, entry.base)
		}
		sort.Strings(keys)

		// Track the exact set of all entries for watch mode
		if entries.accessedEntries != nil {
			entries.accessedEntries.mutex.Lock()
			entries.accessedEntries.allEntries = keys
			entries.accessedEntries.mutex.Unlock()
		}

		return keys
	}

	return
}

type OpenedFile interface {
	Len() int
	Read(start int, end int) ([]byte, error)
	Close() error
}

type InMemoryOpenedFile struct {
	Contents []byte
}

func (f *InMemoryOpenedFile) Len() int {
	return len(f.Contents)
}

func (f *InMemoryOpenedFile) Read(start int, end int) ([]byte, error) {
	return []byte(f.Contents[start:end]), nil
}

func (f *InMemoryOpenedFile) Close() error {
	return nil
}

type FS interface {
	// The returned map is immutable and is cached across invocations. Do not
	// mutate it.
	ReadDirectory(path string) (entries DirEntries, canonicalError error, originalError error)
	ReadFile(path string) (contents string, canonicalError error, originalError error)
	OpenFile(path string) (result OpenedFile, canonicalError error, originalError error)

	// This is a key made from the information returned by "stat". It is intended
	// to be different if the file has been edited, and to otherwise be equal if
	// the file has not been edited. It should usually work, but no guarantees.
	//
	// See https://apenwarr.ca/log/20181113 for more information about why this
	// can be broken. For example, writing to a file with mmap on WSL on Windows
	// won't change this key. Hopefully this isn't too much of an issue.
	//
	// Additional reading:
	// - https://github.com/npm/npm/pull/20027
	// - https://github.com/golang/go/commit/7dea509703eb5ad66a35628b12a678110fbb1f72
	ModKey(path string) (ModKey, error)

	// This is part of the interface because the mock interface used for tests
	// should not depend on file system behavior (i.e. different slashes for
	// Windows) while the real interface should.
	IsAbs(path string) bool
	Abs(path string) (string, bool)
	Dir(path string) string
	Base(path string) string
	Ext(path string) string
	Join(parts ...string) string
	Cwd() string
	Rel(base string, target string) (string, bool)
	EvalSymlinks(path string) (string, bool)

	// This is used in the implementation of "Entry"
	kind(dir string, base string) (symlink string, kind EntryKind)

	// This is a set of all files used and all directories checked. The build
	// must be invalidated if any of these watched files change.
	WatchData() WatchData
}

type WatchData struct {
	// These functions return a non-empty path as a string if the file system
	// entry has been modified. For files, the returned path is the same as the
	// file path. For directories, the returned path is either the directory
	// itself or a file in the directory that was changed.
	Paths map[string]func() string
}

type ModKey struct {
	// What gets filled in here is OS-dependent
	inode      uint64
	size       int64
	mtime_sec  int64
	mtime_nsec int64
	mode       uint32
	uid        uint32
}

// Some file systems have a time resolution of only a few seconds. If a mtime
// value is too new, we won't be able to tell if it has been recently modified
// or not. So we only use mtimes for comparison if they are sufficiently old.
// Apparently the FAT file system has a resolution of two seconds according to
// this article: https://en.wikipedia.org/wiki/Stat_(system_call).
const modKeySafetyGap = 3 // In seconds
var modKeyUnusable = errors.New("The modification key is unusable")

// Limit the number of files open simultaneously to avoid ulimit issues
var fileOpenLimit = make(chan bool, 32)

func BeforeFileOpen() {
	// This will block if the number of open files is already at the limit
	fileOpenLimit <- false
}

func AfterFileClose() {
	<-fileOpenLimit
}

// This is a fork of "os.MkdirAll" to work around bugs with the WebAssembly
// build target. More information here: https://github.com/golang/go/issues/43768.
func MkdirAll(fs FS, path string, perm os.FileMode) error {
	// Run "Join" once to run "Clean" on the path, which removes trailing slashes
	return mkdirAll(fs, fs.Join(path), perm)
}

func mkdirAll(fs FS, path string, perm os.FileMode) error {
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	if dir, err := os.Stat(path); err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	if parent := fs.Dir(path); parent != path {
		// Create parent.
		if err := mkdirAll(fs, parent, perm); err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	if err := os.Mkdir(path, perm); err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := os.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}
