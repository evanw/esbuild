package fs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
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

func (e *Entry) Kind() EntryKind {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.needStat {
		e.stat()
	}
	return e.kind
}

func (e *Entry) Symlink() string {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.needStat {
		e.stat()
	}
	return e.symlink
}

func (e *Entry) stat() {
	e.needStat = false
	entryPath := filepath.Join(e.dir, e.base)

	// Use "lstat" since we want information about symbolic links
	BeforeFileOpen()
	defer AfterFileClose()
	stat, err := os.Lstat(entryPath)
	if err != nil {
		return
	}
	mode := stat.Mode()

	// Follow symlinks now so the cache contains the translation
	if (mode & os.ModeSymlink) != 0 {
		link, err := os.Readlink(entryPath)
		if err != nil {
			return // Skip over this entry
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(e.dir, link)
		}
		e.symlink = filepath.Clean(link)

		// Re-run "lstat" on the symlink target
		stat2, err2 := os.Lstat(e.symlink)
		if err2 != nil {
			return // Skip over this entry
		}
		mode = stat2.Mode()
		if (mode & os.ModeSymlink) != 0 {
			return // Symlink chains are not supported
		}
	}

	// We consider the entry either a directory or a file
	if (mode & os.ModeDir) != 0 {
		e.kind = DirEntry
	} else {
		e.kind = FileEntry
	}
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

////////////////////////////////////////////////////////////////////////////////

type mockFS struct {
	dirs  map[string]map[string]*Entry
	files map[string]string
}

func MockFS(input map[string]string) FS {
	dirs := make(map[string]map[string]*Entry)
	files := make(map[string]string)

	for k, v := range input {
		files[k] = v
		original := k

		// Build the directory map
		for {
			kDir := path.Dir(k)
			dir, ok := dirs[kDir]
			if !ok {
				dir = make(map[string]*Entry)
				dirs[kDir] = dir
			}
			if kDir == k {
				break
			}
			if k == original {
				dir[path.Base(k)] = &Entry{kind: FileEntry}
			} else {
				dir[path.Base(k)] = &Entry{kind: DirEntry}
			}
			k = kDir
		}
	}

	return &mockFS{dirs, files}
}

func (fs *mockFS) ReadDirectory(path string) (map[string]*Entry, error) {
	dir := fs.dirs[path]
	if dir == nil {
		return nil, syscall.ENOENT
	}
	return dir, nil
}

func (fs *mockFS) ReadFile(path string) (string, error) {
	contents, ok := fs.files[path]
	if !ok {
		return "", syscall.ENOENT
	}
	return contents, nil
}

func (fs *mockFS) ModKey(path string) (ModKey, error) {
	return ModKey{}, errors.New("This is not available during tests")
}

func (*mockFS) IsAbs(p string) bool {
	return path.IsAbs(p)
}

func (*mockFS) Abs(p string) (string, bool) {
	return path.Clean(path.Join("/", p)), true
}

func (*mockFS) Dir(p string) string {
	return path.Dir(p)
}

func (*mockFS) Base(p string) string {
	return path.Base(p)
}

func (*mockFS) Ext(p string) string {
	return path.Ext(p)
}

func (*mockFS) Join(parts ...string) string {
	return path.Clean(path.Join(parts...))
}

func (*mockFS) Cwd() string {
	return "/"
}

func splitOnSlash(path string) (string, string) {
	if slash := strings.IndexByte(path, '/'); slash != -1 {
		return path[:slash], path[slash+1:]
	}
	return path, ""
}

func (*mockFS) Rel(base string, target string) (string, bool) {
	// Base cases
	if base == "" || base == "." {
		return target, true
	}
	if base == target {
		return ".", true
	}

	// Find the common parent directory
	for {
		bHead, bTail := splitOnSlash(base)
		tHead, tTail := splitOnSlash(target)
		if bHead != tHead {
			break
		}
		base = bTail
		target = tTail
	}

	// Stop now if base is a subpath of target
	if base == "" {
		return target, true
	}

	// Traverse up to the common parent
	commonParent := strings.Repeat("../", strings.Count(base, "/")+1)

	// Stop now if target is a subpath of base
	if target == "" {
		return commonParent[:len(commonParent)-1], true
	}

	// Otherwise, down to the parent
	return commonParent + target, true
}

////////////////////////////////////////////////////////////////////////////////

type realFS struct {
	// Stores the file entries for directories we've listed before
	entries map[string]entriesOrErr

	// For the current working directory
	cwd string
}

type entriesOrErr struct {
	entries map[string]*Entry
	err     error
}

// Limit the number of files open simultaneously to avoid ulimit issues
var fileOpenLimit = make(chan bool, 32)

func BeforeFileOpen() {
	// This will block if the number of open files is already at the limit
	fileOpenLimit <- false
}

func AfterFileClose() {
	<-fileOpenLimit
}

func realpath(path string) string {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		panic(fmt.Sprintf("EvalSymlinks(%q) error: %v", path, err))
	}
	return path
}

func RealFS() FS {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	} else {
		// Resolve symlinks in the current working directory. Symlinks are resolved
		// when input file paths are converted to absolute paths because we need to
		// recognize an input file as unique even if it has multiple symlinks
		// pointing to it. The build will generate relative paths from the current
		// working directory to the absolute input file paths for error messages,
		// so the current working directory should be processed the same way. Not
		// doing this causes test failures with esbuild when run from inside a
		// symlinked directory.
		cwd = realpath(cwd)
	}
	return &realFS{
		entries: make(map[string]entriesOrErr),
		cwd:     cwd,
	}
}

func (fs *realFS) ReadDirectory(dir string) (map[string]*Entry, error) {
	// First, check the cache
	cached, ok := fs.entries[dir]

	// Cache hit: stop now
	if ok {
		return cached.entries, cached.err
	}

	// Cache miss: read the directory entries
	names, err := readdir(dir)
	entries := make(map[string]*Entry)
	if err == nil {
		for _, name := range names {
			// Call "stat" lazily for performance. The "@material-ui/icons" package
			// contains a directory with over 11,000 entries in it and running "stat"
			// for each entry was a big performance issue for that package.
			entries[name] = &Entry{
				dir:      dir,
				base:     name,
				needStat: true,
			}
		}
	}

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	if err != nil {
		entries = nil
	}
	fs.entries[dir] = entriesOrErr{entries: entries, err: err}
	return entries, err
}

func (fs *realFS) ReadFile(path string) (string, error) {
	BeforeFileOpen()
	defer AfterFileClose()
	buffer, err := ioutil.ReadFile(path)

	// Unwrap to get the underlying error
	if pathErr, ok := err.(*os.PathError); ok {
		err = pathErr.Unwrap()
	}

	// Windows returns ENOTDIR here even though nothing we've done yet has asked
	// for a directory. This really means ENOENT on Windows. Return ENOENT here
	// so callers that check for ENOENT will successfully detect this file as
	// missing.
	if err == syscall.ENOTDIR {
		return "", syscall.ENOENT
	}

	return string(buffer), err
}

func (fs *realFS) ModKey(path string) (ModKey, error) {
	BeforeFileOpen()
	defer AfterFileClose()
	return modKey(path)
}

func (*realFS) IsAbs(p string) bool {
	return filepath.IsAbs(p)
}

func (*realFS) Abs(p string) (string, bool) {
	abs, err := filepath.Abs(p)
	return abs, err == nil
}

func (*realFS) Dir(p string) string {
	return filepath.Dir(p)
}

func (*realFS) Base(p string) string {
	return filepath.Base(p)
}

func (*realFS) Ext(p string) string {
	return filepath.Ext(p)
}

func (*realFS) Join(parts ...string) string {
	return filepath.Clean(filepath.Join(parts...))
}

func (fs *realFS) Cwd() string {
	return fs.cwd
}

func (*realFS) Rel(base string, target string) (string, bool) {
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel, true
	}
	return "", false
}

func readdir(dirname string) ([]string, error) {
	BeforeFileOpen()
	defer AfterFileClose()
	f, err := os.Open(dirname)

	// Unwrap to get the underlying error
	if pathErr, ok := err.(*os.PathError); ok {
		err = pathErr.Unwrap()
	}

	// Windows returns ENOTDIR here even though nothing we've done yet has asked
	// for a directory. This really means ENOENT on Windows. Return ENOENT here
	// so callers that check for ENOENT will successfully detect this directory
	// as missing.
	if err == syscall.ENOTDIR {
		return nil, syscall.ENOENT
	}

	// Stop now if there was an error
	if err != nil {
		return nil, err
	}

	defer f.Close()
	entries, err := f.Readdirnames(-1)

	// Unwrap to get the underlying error
	if syscallErr, ok := err.(*os.SyscallError); ok {
		err = syscallErr.Unwrap()
	}

	// Don't convert ENOTDIR to ENOENT here. ENOTDIR is a legitimate error
	// condition for Readdirnames() on non-Windows platforms.

	return entries, err
}
