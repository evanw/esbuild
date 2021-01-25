package fs

import (
	"io/ioutil"
	"os"
	"syscall"
)

type realFS struct {
	// Stores the file entries for directories we've listed before
	entries map[string]entriesOrErr

	// When building with WebAssembly, the Go compiler doesn't correctly handle
	// platform-specific path behavior. Hack around these bugs by compiling
	// support for both Unix and Windows paths into all executables and switch
	// between them at run-time instead.
	fp goFilepath
}

type entriesOrErr struct {
	entries map[string]*Entry
	err     error
}

func RealFS() FS {
	var fp goFilepath
	if checkIfWindows() {
		fp.isWindows = true
		fp.pathSeparator = '\\'
	} else {
		fp.isWindows = false
		fp.pathSeparator = '/'
	}

	if cwd, err := os.Getwd(); err != nil {
		// This probably only happens in the browser
		fp.cwd = "/"
	} else {
		// Resolve symlinks in the current working directory. Symlinks are resolved
		// when input file paths are converted to absolute paths because we need to
		// recognize an input file as unique even if it has multiple symlinks
		// pointing to it. The build will generate relative paths from the current
		// working directory to the absolute input file paths for error messages,
		// so the current working directory should be processed the same way. Not
		// doing this causes test failures with esbuild when run from inside a
		// symlinked directory.
		//
		// This deliberately ignores errors due to e.g. infinite loops. If there is
		// an error, we will just use the original working directory and likely
		// encounter an error later anyway. And if we don't encounter an error
		// later, then the current working directory didn't even matter and the
		// error is unimportant.
		if path, err := fp.evalSymlinks(cwd); err == nil {
			fp.cwd = path
		} else {
			fp.cwd = cwd
		}
	}

	return &realFS{
		entries: make(map[string]entriesOrErr),
		fp:      fp,
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

func (fs *realFS) IsAbs(p string) bool {
	return fs.fp.isAbs(p)
}

func (fs *realFS) Abs(p string) (string, bool) {
	abs, err := fs.fp.abs(p)
	return abs, err == nil
}

func (fs *realFS) Dir(p string) string {
	return fs.fp.dir(p)
}

func (fs *realFS) Base(p string) string {
	return fs.fp.base(p)
}

func (fs *realFS) Ext(p string) string {
	return fs.fp.ext(p)
}

func (fs *realFS) Join(parts ...string) string {
	return fs.fp.clean(fs.fp.join(parts))
}

func (fs *realFS) Cwd() string {
	return fs.fp.cwd
}

func (fs *realFS) Rel(base string, target string) (string, bool) {
	if rel, err := fs.fp.rel(base, target); err == nil {
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

func (fs *realFS) kind(dir string, base string) (symlink string, kind EntryKind) {
	entryPath := fs.fp.join([]string{dir, base})

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
		if !fs.fp.isAbs(link) {
			link = fs.fp.join([]string{dir, link})
		}
		symlink = fs.fp.clean(link)

		// Re-run "lstat" on the symlink target
		stat2, err2 := os.Lstat(symlink)
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
		kind = DirEntry
	} else {
		kind = FileEntry
	}
	return
}
