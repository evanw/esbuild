package fs

// This is a mock implementation of the "fs" module for use with tests. It does
// not actually read from the file system. Instead, it reads from a pre-specified
// map of file paths to files.

import (
	"errors"
	"path"
	"strings"
	"syscall"
)

type MockKind uint8

const (
	MockUnix MockKind = iota
	MockWindows
)

type mockFS struct {
	dirs          map[string]DirEntries
	files         map[string]string
	absWorkingDir string
	defaultVolume string
	Kind          MockKind
}

func MockFS(input map[string]string, kind MockKind, absWorkingDir string) FS {
	dirs := make(map[string]DirEntries)
	files := make(map[string]string)

	var defaultVolume string
	if kind == MockWindows {
		_, defaultVolume = win2unix(absWorkingDir)
	}

	for k, v := range input {
		var volume string
		files[k] = v
		if kind == MockWindows {
			k, volume = win2unix(k)
		}
		original := k

		// Build the directory map
		for {
			kDir := path.Dir(k)
			key := kDir
			if kind == MockWindows {
				key = unix2win(key, volume, defaultVolume)
			}
			dir, ok := dirs[key]
			if !ok {
				dir = DirEntries{dir: key, data: make(map[string]*Entry)}
				dirs[key] = dir
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

	return &mockFS{dirs, files, absWorkingDir, defaultVolume, kind}
}

func (fs *mockFS) ReadDirectory(path string) (DirEntries, error, error) {
	if fs.Kind == MockWindows {
		path = strings.ReplaceAll(path, "/", "\\")
	}

	var slash byte = '/'
	if fs.Kind == MockWindows {
		slash = '\\'
	}

	// Trim trailing slashes before lookup
	firstSlash := strings.IndexByte(path, slash)
	for {
		i := strings.LastIndexByte(path, slash)
		if i != len(path)-1 || i <= firstSlash {
			break
		}
		path = path[:i]
	}

	if dir, ok := fs.dirs[path]; ok {
		return dir, nil, nil
	}
	return DirEntries{}, syscall.ENOENT, syscall.ENOENT
}

func (fs *mockFS) ReadFile(path string) (string, error, error) {
	if fs.Kind == MockWindows {
		path = strings.ReplaceAll(path, "/", "\\")
	}
	if contents, ok := fs.files[path]; ok {
		return contents, nil, nil
	}
	return "", syscall.ENOENT, syscall.ENOENT
}

func (fs *mockFS) OpenFile(path string) (OpenedFile, error, error) {
	if fs.Kind == MockWindows {
		path = strings.ReplaceAll(path, "/", "\\")
	}
	if contents, ok := fs.files[path]; ok {
		return &InMemoryOpenedFile{Contents: []byte(contents)}, nil, nil
	}
	return nil, syscall.ENOENT, syscall.ENOENT
}

func (fs *mockFS) ModKey(path string) (ModKey, error) {
	return ModKey{}, errors.New("This is not available during tests")
}

func win2unix(p string) (result string, volume string) {
	if len(p) > 0 && strings.HasPrefix(p[1:], ":\\") {
		if c := p[0]; (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			volume = p[0:1]
			p = p[2:]
		}
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return p, volume
}

func unix2win(p string, volume string, defaultVolume string) string {
	p = strings.ReplaceAll(p, "/", "\\")
	if strings.HasPrefix(p, "\\") {
		if volume == "" {
			volume = defaultVolume
		}
		p = volume + ":" + p
	}
	return p
}

func (fs *mockFS) IsAbs(p string) bool {
	if fs.Kind == MockWindows {
		p, _ = win2unix(p)
	}
	return path.IsAbs(p)
}

func (fs *mockFS) Abs(p string) (string, bool) {
	var volume string
	if fs.Kind == MockWindows {
		p, volume = win2unix(p)
	}

	p = path.Clean(path.Join("/", p))

	if fs.Kind == MockWindows {
		p = unix2win(p, volume, fs.defaultVolume)
	}

	return p, true
}

func (fs *mockFS) Dir(p string) string {
	var volume string
	if fs.Kind == MockWindows {
		p, volume = win2unix(p)
	}

	p = path.Dir(p)

	if fs.Kind == MockWindows {
		p = unix2win(p, volume, fs.defaultVolume)
	}

	return p
}

func (fs *mockFS) Base(p string) string {
	var volume string
	if fs.Kind == MockWindows {
		p, volume = win2unix(p)
	}

	p = path.Base(p)

	if fs.Kind == MockWindows && p == "/" {
		p = volume + ":\\"
	}

	return p
}

func (fs *mockFS) Ext(p string) string {
	if fs.Kind == MockWindows {
		p, _ = win2unix(p)
	}

	return path.Ext(p)
}

func (fs *mockFS) Join(parts ...string) string {
	var volume string
	if fs.Kind == MockWindows {
		converted := make([]string, len(parts))
		for i, part := range parts {
			var v string
			converted[i], v = win2unix(part)
			if i == 0 {
				volume = v
			}
		}
		parts = converted
	}

	p := path.Clean(path.Join(parts...))

	if fs.Kind == MockWindows {
		p = unix2win(p, volume, fs.defaultVolume)
	}

	return p
}

func (fs *mockFS) Cwd() string {
	return fs.absWorkingDir
}

func splitOnSlash(path string) (string, string) {
	if slash := strings.IndexByte(path, '/'); slash != -1 {
		return path[:slash], path[slash+1:]
	}
	return path, ""
}

func (fs *mockFS) Rel(base string, target string) (string, bool) {
	var volume string
	if fs.Kind == MockWindows {
		var v string
		base, volume = win2unix(base)
		target, v = win2unix(target)
		if volume == "" {
			volume = fs.defaultVolume
		}
		if v == "" {
			v = fs.defaultVolume
		}
		if strings.ToUpper(v) != strings.ToUpper(volume) {
			return "", false
		}
	}

	base = path.Clean(base)
	target = path.Clean(target)

	// Go's implementation does these checks
	if base == target {
		return ".", true
	}
	if base == "." {
		base = ""
	}

	// Go's implementation fails when this condition is false. I believe this is
	// because of this part of the contract, from Go's documentation: "An error
	// is returned if targpath can't be made relative to basepath or if knowing
	// the current working directory would be necessary to compute it."
	if (len(base) > 0 && base[0] == '/') != (len(target) > 0 && target[0] == '/') {
		return "", false
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
		if fs.Kind == MockWindows {
			target = unix2win(target, volume, fs.defaultVolume)
		}
		return target, true
	}

	// Traverse up to the common parent
	commonParent := strings.Repeat("../", strings.Count(base, "/")+1)

	// Stop now if target is a subpath of base
	if target == "" {
		target = commonParent[:len(commonParent)-1]
		if fs.Kind == MockWindows {
			target = unix2win(target, volume, fs.defaultVolume)
		}
		return target, true
	}

	// Otherwise, down to the parent
	target = commonParent + target
	if fs.Kind == MockWindows {
		target = unix2win(target, volume, fs.defaultVolume)
	}
	return target, true
}

func (fs *mockFS) EvalSymlinks(path string) (string, bool) {
	return "", false
}

func (fs *mockFS) kind(dir string, base string) (symlink string, kind EntryKind) {
	panic("This should never be called")
}

func (fs *mockFS) WatchData() WatchData {
	panic("This should never be called")
}
