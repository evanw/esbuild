package fs

import (
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
	ReadDirectory(path string) map[string]*Entry
	ReadFile(path string) (string, error)

	// This is part of the interface because the mock interface used for tests
	// should not depend on file system behavior (i.e. different slashes for
	// Windows) while the real interface should.
	Abs(path string) (string, bool)
	Dir(path string) string
	Base(path string) string
	Ext(path string) string
	Join(parts ...string) string
	Cwd() string
	Rel(base string, target string) (string, bool)
}

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

func (fs *mockFS) ReadDirectory(path string) map[string]*Entry {
	return fs.dirs[path]
}

func (fs *mockFS) ReadFile(path string) (string, error) {
	contents, ok := fs.files[path]
	if !ok {
		return "", syscall.ENOENT
	}
	return contents, nil
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
	return ""
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
	entries map[string]map[string]*Entry

	// For the current working directory
	cwd string
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
	dir := filepath.Dir(path)
	if dir == path {
		return path
	}
	dir = realpath(dir)
	path = filepath.Join(dir, filepath.Base(path))
	BeforeFileOpen()
	defer AfterFileClose()
	if link, err := os.Readlink(path); err == nil {
		if filepath.IsAbs(link) {
			return link
		}
		return filepath.Join(dir, link)
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
		entries: make(map[string]map[string]*Entry),
		cwd:     cwd,
	}
}

func (fs *realFS) ReadDirectory(dir string) map[string]*Entry {
	// First, check the cache
	cached, ok := fs.entries[dir]

	// Cache hit: stop now
	if ok {
		return cached
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
		fs.entries[dir] = nil
		return nil
	}
	fs.entries[dir] = entries
	return entries
}

func (fs *realFS) ReadFile(path string) (string, error) {
	BeforeFileOpen()
	defer AfterFileClose()
	buffer, err := ioutil.ReadFile(path)
	if err != nil {
		if pathErr, ok := err.(*os.PathError); ok {
			return "", pathErr.Unwrap()
		}
	}
	return string(buffer), err
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
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}
