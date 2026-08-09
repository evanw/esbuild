package fs

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/evanw/esbuild/internal/io_fs"
	"golang.org/x/sys/unix"
)

func FromFS(fs io_fs.FS) FS {
	ret := &ioFS{FS: fs}
	ret.fp.cwd = "."
	ret.fp.pathSeparator = '/'
	return ret
}

type ioFS struct {
	io_fs.FS
	fp goFilepath
}

func (fs *ioFS) ReadDirectory(dir string) (entries DirEntries, canonicalError error, originalError error) {
	// Read the directory entries
	dir = strings.TrimPrefix(dir, "/")
	if dir == "" {
		dir = "."
	}

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

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	if canonicalError != nil {
		entries.data = nil
	}
	return entries, canonicalError, originalError
}

func (fs *ioFS) readdir(dirname string) (entries []string, canonicalError error, originalError error) {
	BeforeFileOpen()
	defer AfterFileClose()
	f, originalError := fs.Open(dirname)
	canonicalError = fs.canonicalizeError(originalError)

	// Stop now if there was an error
	if canonicalError != nil {
		return nil, canonicalError, originalError
	}

	defer f.Close()

	dir, ok := f.(io_fs.ReadDirFile)
	if !ok {
		return
	}

	dirEntries, originalError := dir.ReadDir(-1)
	canonicalError = originalError

	entries = make([]string, len(dirEntries))
	for i, e := range dirEntries {
		entries[i] = e.Name()
	}

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

func (fs *ioFS) canonicalizeError(err error) error {
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

func (fs *ioFS) ReadFile(name string) (contents string, canonicalError error, originalError error) {
	name = strings.TrimPrefix(name, "/")
	BeforeFileOpen()
	defer AfterFileClose()
	f, originalError := fs.Open(name)
	canonicalError = fs.canonicalizeError(originalError)

	// Stop now if there was an error
	if canonicalError != nil {
		return "", canonicalError, originalError
	}

	defer f.Close()

	buffer, originalError := ioutil.ReadAll(f)
	canonicalError = fs.canonicalizeError(originalError)
	return string(buffer), canonicalError, originalError
}

type iofsOpenedFile struct {
	handle io_fs.File
	len    int
}

func (f *iofsOpenedFile) Len() int { return f.len }

func (f *iofsOpenedFile) Read(start int, end int) ([]byte, error) {
	bytes := make([]byte, end-start)
	remaining := bytes

	if r, ok := f.handle.(io.ReaderAt); ok {
		for len(remaining) > 0 {
			n, err := r.ReadAt(remaining, int64(start))
			if err != nil && n <= 0 {
				return nil, err
			}
			remaining = remaining[n:]
			start += n
		}
		return bytes, nil
	}

	if s, ok := f.handle.(io.Seeker); !ok {
		return nil, fmt.Errorf("file does not support random access")
	} else if _, err := s.Seek(int64(start), io.SeekStart); err != nil {
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

func (f *iofsOpenedFile) Close() error {
	return f.handle.Close()
}

func (fs *ioFS) OpenFile(name string) (result OpenedFile, canonicalError error, originalError error) {
	name = strings.TrimPrefix(name, "/")
	BeforeFileOpen()
	defer AfterFileClose()

	f, err := fs.Open(name)
	if err != nil {
		return nil, fs.canonicalizeError(err), err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fs.canonicalizeError(err), err
	}

	return &iofsOpenedFile{f, int(info.Size())}, nil, nil
}

func (fs *ioFS) ModKey(name string) (ModKey, error) {
	stat, err := fs.stat(name)
	if err != nil {
		return ModKey{}, err
	}

	// We can't detect changes if the file system zeros out the modification time
	mtim := stat.ModTime()
	mtim_sec, mtim_nsec := int64(mtim.Second()), int64(mtim.Nanosecond())
	if mtim_sec == 0 && mtim_nsec == 0 {
		return ModKey{}, modKeyUnusable
	}

	// Don't generate a modification key if the file is too new
	now, err := unix.TimeToTimespec(time.Now())
	if err != nil {
		return ModKey{}, err
	}
	mtimeSec := mtim_sec + modKeySafetyGap
	if mtimeSec > now.Sec || (mtimeSec == now.Sec && mtim_nsec > now.Nsec) {
		return ModKey{}, modKeyUnusable
	}

	return ModKey{
		size:       stat.Size(),
		mtime_sec:  mtim_sec,
		mtime_nsec: mtim_nsec,
		mode:       uint32(stat.Mode()),
	}, nil
}

func (fs *ioFS) stat(name string) (io_fs.FileInfo, error) {
	name = strings.TrimPrefix(name, "/")
	BeforeFileOpen()
	defer AfterFileClose()

	f, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return f.Stat()
}

func (fs *ioFS) IsAbs(p string) bool {
	return fs.fp.isAbs(p)
}

func (fs *ioFS) Abs(p string) (string, bool) {
	abs, err := fs.fp.abs(p)
	return abs, err == nil
}

func (fs *ioFS) Dir(p string) string {
	return fs.fp.dir(p)
}

func (fs *ioFS) Base(p string) string {
	return fs.fp.base(p)
}

func (fs *ioFS) Ext(p string) string {
	return fs.fp.ext(p)
}

func (fs *ioFS) Join(parts ...string) string {
	return fs.fp.clean(fs.fp.join(parts))
}

func (fs *ioFS) Cwd() string {
	return fs.fp.cwd
}

func (fs *ioFS) Rel(base string, target string) (string, bool) {
	if rel, err := fs.fp.rel(base, target); err == nil {
		return rel, true
	}
	return "", false
}

func (fs *ioFS) EvalSymlinks(path string) (string, bool) {
	// TODO
	return "", false
}

func (fs *ioFS) kind(dir string, base string) (symlink string, kind EntryKind) {
	entryPath := fs.fp.join([]string{dir, base})

	// Use "lstat" since we want information about symbolic links
	stat, err := fs.stat(entryPath)
	if err != nil {
		return
	}
	mode := stat.Mode()

	// Follow symlinks now so the cache contains the translation
	if (mode & io_fs.ModeSymlink) != 0 {
		link, ok := fs.EvalSymlinks(entryPath)
		if !ok {
			return // Skip over this entry
		}

		// Re-run "lstat" on the symlink target to see if it's a file or not
		stat2, err2 := fs.stat(entryPath)
		if err2 != nil {
			return // Skip over this entry
		}
		mode = stat2.Mode()
		if (mode & io_fs.ModeSymlink) != 0 {
			return // This should no longer be a symlink, so this is unexpected
		}
		symlink = link
	}

	// We consider the entry either a directory or a file
	if mode.IsDir() {
		kind = DirEntry
	} else {
		kind = FileEntry
	}
	return
}

func (fs *ioFS) WatchData() WatchData {
	return WatchData{}
}
