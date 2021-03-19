// This is a mock implementation of the "fs" module for use with tests. It does
// not actually read from the file system. Instead, it reads from a pre-specified
// map of file paths to files.

package fs

import (
	"errors"
	"path"
	"strings"
	"syscall"
)

type mockFS struct {
	dirs  map[string]DirEntries
	files map[string]string
}

func MockFS(input map[string]string) FS {
	dirs := make(map[string]DirEntries)
	files := make(map[string]string)

	for k, v := range input {
		files[k] = v
		original := k

		// Build the directory map
		for {
			kDir := path.Dir(k)
			dir, ok := dirs[kDir]
			if !ok {
				dir = DirEntries{kDir, make(map[string]*Entry)}
				dirs[kDir] = dir
			}
			if kDir == k {
				break
			}
			base := path.Base(k)
			if k == original {
				dir.data[strings.ToLower(base)] = &Entry{kind: FileEntry, base: base}
			} else {
				dir.data[strings.ToLower(base)] = &Entry{kind: DirEntry, base: base}
			}
			k = kDir
		}
	}

	return &mockFS{dirs, files}
}

func (fs *mockFS) ReadDirectory(path string) (DirEntries, error) {
	if dir, ok := fs.dirs[path]; ok {
		return dir, nil
	}
	return DirEntries{}, syscall.ENOENT
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
	base = path.Clean(base)
	target = path.Clean(target)

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

func (fs *mockFS) kind(dir string, base string) (symlink string, kind EntryKind) {
	panic("This should never be called")
}

func (fs *mockFS) WatchData() WatchData {
	panic("This should never be called")
}
