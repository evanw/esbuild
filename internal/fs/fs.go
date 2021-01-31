package fs

import (
	"errors"
	"sync"
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

type FS interface {
	// The returned map is immutable and is cached across invocations. Do not
	// mutate it.
	ReadDirectory(path string) (map[string]*Entry, error)
	ReadFile(path string) (string, error)

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

	// This is used in the implementation of "Entry"
	kind(dir string, base string) (symlink string, kind EntryKind)

	// This is a set of all files used and all directories checked. The build
	// must be invalidated if any of these watched files change.
	WatchData() WatchData
}

type WatchData struct {
	// These functions return true if the file system entry has been modified
	Paths map[string]func() bool
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
