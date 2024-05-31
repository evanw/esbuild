package fs

import (
	"fmt"
	"io"
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

	// This stores data that will end up being returned by "WatchData()"
	watchData map[string]privateWatchData

	// When building with WebAssembly, the Go compiler doesn't correctly handle
	// platform-specific path behavior. Hack around these bugs by compiling
	// support for both Unix and Windows paths into all executables and switch
	// between them at run-time instead.
	fp goFilepath

	entriesMutex sync.Mutex
	watchMutex   sync.Mutex

	// If true, do not use the "entries" cache
	doNotCacheEntries bool
}

type entriesOrErr struct {
	canonicalError error
	originalError  error
	entries        DirEntries
}

type watchState uint8

const (
	stateNone                  watchState = iota
	stateDirHasAccessedEntries            // Compare "accessedEntries"
	stateDirUnreadable                    // Compare directory readability
	stateFileHasModKey                    // Compare "modKey"
	stateFileNeedModKey                   // Need to transition to "stateFileHasModKey" or "stateFileUnusableModKey" before "WatchData()" returns
	stateFileMissing                      // Compare file presence
	stateFileUnusableModKey               // Compare "fileContents"
)

type privateWatchData struct {
	accessedEntries *accessedEntries
	fileContents    string
	modKey          ModKey
	state           watchState
}

type RealFSOptions struct {
	AbsWorkingDir string
	WantWatchData bool
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

	var result FS = &realFS{
		entries:           make(map[string]entriesOrErr),
		fp:                fp,
		watchData:         watchData,
		doNotCacheEntries: options.DoNotCache,
	}

	// Add a wrapper that lets us traverse into ".zip" files. This is what yarn
	// uses as a package format when in yarn is in its "PnP" mode.
	result = &zipFS{
		inner:    result,
		zipFiles: make(map[string]*zipFile),
	}

	return result, nil
}

func (fs *realFS) ReadDirectory(dir string) (entries DirEntries, canonicalError error, originalError error) {
	if !fs.doNotCacheEntries {
		// First, check the cache
		cached, ok := func() (cached entriesOrErr, ok bool) {
			fs.entriesMutex.Lock()
			defer fs.entriesMutex.Unlock()
			cached, ok = fs.entries[dir]
			return
		}()
		if ok {
			// Cache hit: stop now
			return cached.entries, cached.canonicalError, cached.originalError
		}
	}

	// Cache miss: read the directory entries
	names, canonicalError, originalError := fs.readdir(dir)
	entries = DirEntries{dir: dir, data: make(map[string]*Entry)}

	// Unwrap to get the underlying error
	if pathErr, ok := canonicalError.(*os.PathError); ok {
		canonicalError = pathErr.Unwrap()
	}

	if canonicalError == nil {
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
		state := stateDirHasAccessedEntries
		if canonicalError != nil {
			state = stateDirUnreadable
		}
		entries.accessedEntries = &accessedEntries{wasPresent: make(map[string]bool)}
		fs.watchData[dir] = privateWatchData{
			accessedEntries: entries.accessedEntries,
			state:           state,
		}
	}

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	if canonicalError != nil {
		entries.data = nil
	}
	if !fs.doNotCacheEntries {
		fs.entriesMutex.Lock()
		defer fs.entriesMutex.Unlock()
		fs.entries[dir] = entriesOrErr{
			entries:        entries,
			canonicalError: canonicalError,
			originalError:  originalError,
		}
	}
	return entries, canonicalError, originalError
}

func (fs *realFS) ReadFile(path string) (contents string, canonicalError error, originalError error) {
	BeforeFileOpen()
	defer AfterFileClose()
	buffer, originalError := ioutil.ReadFile(path)
	canonicalError = fs.canonicalizeError(originalError)

	// Allocate the string once
	fileContents := string(buffer)

	// Store data for watch mode
	if fs.watchData != nil {
		defer fs.watchMutex.Unlock()
		fs.watchMutex.Lock()
		data, ok := fs.watchData[path]
		if canonicalError != nil {
			data.state = stateFileMissing
		} else if !ok || data.state == stateDirUnreadable {
			// Note: If "ReadDirectory" is called before "ReadFile" with this same
			// path, then "data.state" will be "stateDirUnreadable". In that case
			// we want to transition to "stateFileNeedModKey" because it's a file.
			data.state = stateFileNeedModKey
		}
		data.fileContents = fileContents
		fs.watchData[path] = data
	}

	return fileContents, canonicalError, originalError
}

type realOpenedFile struct {
	handle *os.File
	len    int
}

func (f *realOpenedFile) Len() int {
	return f.len
}

func (f *realOpenedFile) Read(start int, end int) ([]byte, error) {
	bytes := make([]byte, end-start)
	remaining := bytes

	_, err := f.handle.Seek(int64(start), io.SeekStart)
	if err != nil {
		return nil, err
	}

	for len(remaining) > 0 {
		n, err := f.handle.Read(remaining)
		if err != nil && n <= 0 {
			return nil, err
		}
		remaining = remaining[n:]
	}

	return bytes, nil
}

func (f *realOpenedFile) Close() error {
	return f.handle.Close()
}

func (fs *realFS) OpenFile(path string) (OpenedFile, error, error) {
	BeforeFileOpen()
	defer AfterFileClose()

	f, err := os.Open(path)
	if err != nil {
		return nil, fs.canonicalizeError(err), err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fs.canonicalizeError(err), err
	}

	return &realOpenedFile{f, int(info.Size())}, nil, nil
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

func (fs *realFS) EvalSymlinks(path string) (string, bool) {
	if path, err := fs.fp.evalSymlinks(path); err == nil {
		return path, true
	}
	return "", false
}

func (fs *realFS) readdir(dirname string) (entries []string, canonicalError error, originalError error) {
	BeforeFileOpen()
	defer AfterFileClose()
	f, originalError := os.Open(dirname)
	canonicalError = fs.canonicalizeError(originalError)

	// Stop now if there was an error
	if canonicalError != nil {
		return nil, canonicalError, originalError
	}

	defer f.Close()
	entries, originalError = f.Readdirnames(-1)
	canonicalError = originalError

	// Unwrap to get the underlying error
	if syscallErr, ok := canonicalError.(*os.SyscallError); ok {
		canonicalError = syscallErr.Unwrap()
	}

	// Don't convert ENOTDIR to ENOENT here. ENOTDIR is a legitimate error
	// condition for Readdirnames() on non-Windows platforms.

	// Go's WebAssembly implementation returns EINVAL instead of ENOTDIR if we
	// call "readdir" on a file. Canonicalize this to ENOTDIR so esbuild's path
	// resolution code continues traversing instead of failing with an error.
	// https://github.com/golang/go/blob/2449bbb5e614954ce9e99c8a481ea2ee73d72d61/src/syscall/fs_js.go#L144
	if pathErr, ok := canonicalError.(*os.PathError); ok && pathErr.Unwrap() == syscall.EINVAL {
		canonicalError = syscall.ENOTDIR
	}

	return entries, canonicalError, originalError
}

func (fs *realFS) canonicalizeError(err error) error {
	// Unwrap to get the underlying error
	if pathErr, ok := err.(*os.PathError); ok {
		err = pathErr.Unwrap()
	}

	// Windows is much more restrictive than Unix about file names. If a file name
	// is invalid, it will return ERROR_INVALID_NAME. Treat this as ENOENT (i.e.
	// "the file does not exist") so that the resolver continues trying to resolve
	// the path on this failure instead of aborting with an error.
	if fs.fp.isWindows && is_ERROR_INVALID_NAME(err) {
		err = syscall.ENOENT
	}

	// Windows returns ENOTDIR here even though nothing we've done yet has asked
	// for a directory. This really means ENOENT on Windows. Return ENOENT here
	// so callers that check for ENOENT will successfully detect this file as
	// missing.
	if err == syscall.ENOTDIR {
		err = syscall.ENOENT
	}

	return err
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
		link, err := fs.fp.evalSymlinks(entryPath)
		if err != nil {
			return // Skip over this entry
		}

		// Re-run "lstat" on the symlink target to see if it's a file or not
		stat2, err2 := os.Lstat(link)
		if err2 != nil {
			return // Skip over this entry
		}
		mode = stat2.Mode()
		if (mode & os.ModeSymlink) != 0 {
			return // This should no longer be a symlink, so this is unexpected
		}
		symlink = link
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
	paths := make(map[string]func() string)

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
		case stateDirUnreadable:
			paths[path] = func() string {
				_, err, _ := fs.readdir(path)
				if err == nil {
					return path
				}
				return ""
			}

		case stateDirHasAccessedEntries:
			paths[path] = func() string {
				names, err, _ := fs.readdir(path)
				if err != nil {
					return path
				}
				data.accessedEntries.mutex.Lock()
				defer data.accessedEntries.mutex.Unlock()
				if allEntries := data.accessedEntries.allEntries; allEntries != nil {
					// Check all entries
					if len(names) != len(allEntries) {
						return path
					}
					sort.Strings(names)
					for i, s := range names {
						if s != allEntries[i] {
							return path
						}
					}
				} else {
					// Check individual entries
					lookup := make(map[string]string, len(names))
					for _, name := range names {
						lookup[strings.ToLower(name)] = name
					}
					for name, wasPresent := range data.accessedEntries.wasPresent {
						if originalName, isPresent := lookup[name]; wasPresent != isPresent {
							return fs.Join(path, originalName)
						}
					}
				}
				return ""
			}

		case stateFileMissing:
			paths[path] = func() string {
				if info, err := os.Stat(path); err == nil && !info.IsDir() {
					return path
				}
				return ""
			}

		case stateFileHasModKey:
			paths[path] = func() string {
				if key, err := modKey(path); err != nil || key != data.modKey {
					return path
				}
				return ""
			}

		case stateFileUnusableModKey:
			paths[path] = func() string {
				if buffer, err := ioutil.ReadFile(path); err != nil || string(buffer) != data.fileContents {
					return path
				}
				return ""
			}
		}
	}

	return WatchData{
		Paths: paths,
	}
}
