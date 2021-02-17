package fs

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
)

type realFS struct {
	// Stores the file entries for directories we've listed before
	entries map[string]entriesOrErr

	// If true, do not use the "entries" cache
	doNotCacheEntries bool

	// This stores data that will end up being returned by "WatchData()"
	watchMutex sync.Mutex
	watchData  map[string]privateWatchData

	// When building with WebAssembly, the Go compiler doesn't correctly handle
	// platform-specific path behavior. Hack around these bugs by compiling
	// support for both Unix and Windows paths into all executables and switch
	// between them at run-time instead.
	fp goFilepath
}

type entriesOrErr struct {
	entries DirEntries
	err     error
}

type watchState uint8

const (
	stateNone               watchState = iota
	stateDirHasEntries                 // Compare "dirEntries"
	stateDirMissing                    // Compare directory presence
	stateFileHasModKey                 // Compare "modKey"
	stateFileNeedModKey                // Need to transition to "stateFileHasModKey" or "stateFileUnusableModKey" before "WatchData()" returns
	stateFileMissing                   // Compare file presence
	stateFileUnusableModKey            // Compare "fileContents"
)

type privateWatchData struct {
	dirEntries   []string
	fileContents string
	modKey       ModKey
	state        watchState
}

type RealFSOptions struct {
	WantWatchData bool
	AbsWorkingDir string
	DoNotCache    bool
}

func RealFS(options RealFSOptions) (FS, error) {
	var fp goFilepath
	if CheckIfWindows() {
		fp.isWindows = true
		fp.pathSeparator = '\\'
	} else {
		fp.isWindows = false
		fp.pathSeparator = '/'
	}

	// Come up with a default working directory if one was not specified
	fp.cwd = options.AbsWorkingDir
	if fp.cwd == "" {
		if cwd, err := os.Getwd(); err == nil {
			fp.cwd = cwd
		} else if fp.isWindows {
			fp.cwd = "C:\\"
		} else {
			fp.cwd = "/"
		}
	} else if !fp.isAbs(fp.cwd) {
		return nil, fmt.Errorf("The working directory %q is not an absolute path", fp.cwd)
	}

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
	if path, err := fp.evalSymlinks(fp.cwd); err == nil {
		fp.cwd = path
	}

	// Only allocate memory for watch data if necessary
	var watchData map[string]privateWatchData
	if options.WantWatchData {
		watchData = make(map[string]privateWatchData)
	}

	return &realFS{
		entries:           make(map[string]entriesOrErr),
		fp:                fp,
		watchData:         watchData,
		doNotCacheEntries: options.DoNotCache,
	}, nil
}

func (fs *realFS) ReadDirectory(dir string) (DirEntries, error) {
	if !fs.doNotCacheEntries {
		// First, check the cache
		if cached, ok := fs.entries[dir]; ok {
			// Cache hit: stop now
			return cached.entries, cached.err
		}
	}

	// Cache miss: read the directory entries
	names, err := readdir(dir)
	entries := DirEntries{dir, make(map[string]*Entry)}
	if err == nil {
		for _, name := range names {
			// Call "stat" lazily for performance. The "@material-ui/icons" package
			// contains a directory with over 11,000 entries in it and running "stat"
			// for each entry was a big performance issue for that package.
			entries.data[strings.ToLower(name)] = &Entry{
				dir:      dir,
				base:     name,
				needStat: true,
			}
		}
	}

	// Store data for watch mode
	if fs.watchData != nil {
		defer fs.watchMutex.Unlock()
		fs.watchMutex.Lock()
		state := stateDirHasEntries
		if err != nil {
			state = stateDirMissing
		}
		sort.Strings(names)
		fs.watchData[dir] = privateWatchData{
			dirEntries: names,
			state:      state,
		}
	}

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	if err != nil {
		entries.data = nil
	}
	if !fs.doNotCacheEntries {
		fs.entries[dir] = entriesOrErr{entries: entries, err: err}
	}
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
		err = syscall.ENOENT
	}

	// Allocate the string once
	fileContents := string(buffer)

	// Store data for watch mode
	if fs.watchData != nil {
		defer fs.watchMutex.Unlock()
		fs.watchMutex.Lock()
		data, ok := fs.watchData[path]
		if err != nil {
			data.state = stateFileMissing
		} else if !ok {
			data.state = stateFileNeedModKey
		}
		data.fileContents = fileContents
		fs.watchData[path] = data
	}

	return fileContents, err
}

func (fs *realFS) ModKey(path string) (ModKey, error) {
	BeforeFileOpen()
	defer AfterFileClose()
	key, err := modKey(path)

	// Store data for watch mode
	if fs.watchData != nil {
		defer fs.watchMutex.Unlock()
		fs.watchMutex.Lock()
		data, ok := fs.watchData[path]
		if !ok {
			if err == modKeyUnusable {
				data.state = stateFileUnusableModKey
			} else if err != nil {
				data.state = stateFileMissing
			} else {
				data.state = stateFileHasModKey
			}
		} else if data.state == stateFileNeedModKey {
			data.state = stateFileHasModKey
		}
		data.modKey = key
		fs.watchData[path] = data
	}

	return key, err
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
		symlink = entryPath
		linksWalked := 0
		for {
			linksWalked++
			if linksWalked > 255 {
				return // Error: too many links
			}
			link, err := os.Readlink(symlink)
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
			if (mode & os.ModeSymlink) == 0 {
				break
			}
			dir = fs.fp.dir(symlink)
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

func (fs *realFS) WatchData() WatchData {
	paths := make(map[string]func() bool)

	for path, data := range fs.watchData {
		// Each closure below needs its own copy of these loop variables
		path := path
		data := data

		// Each function should return true if the state has been changed
		if data.state == stateFileNeedModKey {
			key, err := modKey(path)
			if err == modKeyUnusable {
				data.state = stateFileUnusableModKey
			} else if err != nil {
				data.state = stateFileMissing
			} else {
				data.state = stateFileHasModKey
				data.modKey = key
			}
		}

		switch data.state {
		case stateDirMissing:
			paths[path] = func() bool {
				info, err := os.Stat(path)
				return err == nil && info.IsDir()
			}

		case stateDirHasEntries:
			paths[path] = func() bool {
				names, err := readdir(path)
				if err != nil || len(names) != len(data.dirEntries) {
					return true
				}
				sort.Strings(names)
				for i, s := range names {
					if s != data.dirEntries[i] {
						return true
					}
				}
				return false
			}

		case stateFileMissing:
			paths[path] = func() bool {
				info, err := os.Stat(path)
				return err == nil && !info.IsDir()
			}

		case stateFileHasModKey:
			paths[path] = func() bool {
				key, err := modKey(path)
				return err != nil || key != data.modKey
			}

		case stateFileUnusableModKey:
			paths[path] = func() bool {
				buffer, err := ioutil.ReadFile(path)
				return err != nil || string(buffer) != data.fileContents
			}
		}
	}

	return WatchData{
		Paths: paths,
	}
}
