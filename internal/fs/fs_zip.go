package fs

import (
	"archive/zip"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type zipFS struct {
	inner FS

	zipFilesMutex sync.Mutex
	zipFiles      map[string]*zipFile
}

type zipFile struct {
	reader *zip.ReadCloser
	err    error

	dirs  map[string]*compressedDir
	files map[string]*compressedFile
	wait  sync.WaitGroup
}

type compressedDir struct {
	entries map[string]EntryKind
	path    string

	// Compatible entries are decoded lazily
	mutex      sync.Mutex
	dirEntries DirEntries
}

type compressedFile struct {
	compressed *zip.File

	// The file is decompressed lazily
	mutex    sync.Mutex
	contents string
	err      error
	wasRead  bool
}

func (fs *zipFS) checkForZip(path string, kind EntryKind) (*zipFile, string) {
	var zipPath string
	var pathTail string

	// Do a quick check for a ".zip" in the path at all
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.Index(path, ".zip/"); i != -1 {
		zipPath = path[:i+len(".zip")]
		pathTail = path[i+len(".zip/"):]
	} else if kind == DirEntry && strings.HasSuffix(path, ".zip") {
		zipPath = path
	} else {
		return nil, ""
	}

	// If there is one, then check whether it's a file on the file system or not
	fs.zipFilesMutex.Lock()
	archive := fs.zipFiles[zipPath]
	if archive != nil {
		fs.zipFilesMutex.Unlock()
		archive.wait.Wait()
	} else {
		archive = &zipFile{}
		archive.wait.Add(1)
		fs.zipFiles[zipPath] = archive
		fs.zipFilesMutex.Unlock()
		defer archive.wait.Done()

		// Try reading the zip archive if it's not in the cache
		tryToReadZipArchive(zipPath, archive)
	}

	if archive.err != nil {
		return nil, ""
	}
	return archive, pathTail
}

func tryToReadZipArchive(zipPath string, archive *zipFile) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		archive.err = err
		return
	}

	dirs := make(map[string]*compressedDir)
	files := make(map[string]*compressedFile)

	// Build an index of all files in the archive
	for _, file := range reader.File {
		baseName := file.Name
		if strings.HasSuffix(baseName, "/") {
			baseName = baseName[:len(baseName)-1]
		}
		dirPath := ""
		if slash := strings.LastIndexByte(baseName, '/'); slash != -1 {
			dirPath = baseName[:slash]
			baseName = baseName[slash+1:]
		}
		if file.FileInfo().IsDir() {
			// Handle a directory
			lowerDir := strings.ToLower(dirPath)
			if _, ok := dirs[lowerDir]; !ok {
				dirs[lowerDir] = &compressedDir{
					path:    dirPath,
					entries: make(map[string]EntryKind),
				}
			}
		} else {
			// Handle a file
			files[strings.ToLower(file.Name)] = &compressedFile{compressed: file}
			lowerDir := strings.ToLower(dirPath)
			dir, ok := dirs[lowerDir]
			if !ok {
				dir = &compressedDir{
					path:    dirPath,
					entries: make(map[string]EntryKind),
				}
				dirs[lowerDir] = dir
			}
			dir.entries[baseName] = FileEntry
		}
	}

	// Populate child directories
	seeds := make([]string, 0, len(dirs))
	for dir := range dirs {
		seeds = append(seeds, dir)
	}
	for _, baseName := range seeds {
		for baseName != "" {
			dirPath := ""
			if slash := strings.LastIndexByte(baseName, '/'); slash != -1 {
				dirPath = baseName[:slash]
				baseName = baseName[slash+1:]
			}
			lowerDir := strings.ToLower(dirPath)
			dir, ok := dirs[lowerDir]
			if !ok {
				dir = &compressedDir{
					path:    dirPath,
					entries: make(map[string]EntryKind),
				}
				dirs[lowerDir] = dir
			}
			dir.entries[baseName] = DirEntry
			baseName = dirPath
		}
	}

	archive.dirs = dirs
	archive.files = files
	archive.reader = reader
}

func (fs *zipFS) ReadDirectory(path string) (entries DirEntries, canonicalError error, originalError error) {
	path = mangleYarnPnPVirtualPath(path)

	entries, canonicalError, originalError = fs.inner.ReadDirectory(path)
	if canonicalError != syscall.ENOENT && canonicalError != syscall.ENOTDIR {
		return
	}

	// If the directory doesn't exist, try reading from an enclosing zip archive
	zip, pathTail := fs.checkForZip(path, DirEntry)
	if zip == nil {
		return
	}

	// Does the zip archive have this directory?
	dir, ok := zip.dirs[strings.ToLower(pathTail)]
	if !ok {
		return DirEntries{}, syscall.ENOENT, syscall.ENOENT
	}

	// Check whether it has already been converted
	dir.mutex.Lock()
	defer dir.mutex.Unlock()
	if dir.dirEntries.data != nil {
		return dir.dirEntries, nil, nil
	}

	// Otherwise, fill in the entries
	dir.dirEntries = DirEntries{dir: path, data: make(map[string]*Entry, len(dir.entries))}
	for name, kind := range dir.entries {
		dir.dirEntries.data[strings.ToLower(name)] = &Entry{
			dir:  path,
			base: name,
			kind: kind,
		}
	}

	return dir.dirEntries, nil, nil
}

func (fs *zipFS) ReadFile(path string) (contents string, canonicalError error, originalError error) {
	path = mangleYarnPnPVirtualPath(path)

	contents, canonicalError, originalError = fs.inner.ReadFile(path)
	if canonicalError != syscall.ENOENT {
		return
	}

	// If the file doesn't exist, try reading from an enclosing zip archive
	zip, pathTail := fs.checkForZip(path, FileEntry)
	if zip == nil {
		return
	}

	// Does the zip archive have this file?
	file, ok := zip.files[strings.ToLower(pathTail)]
	if !ok {
		return "", syscall.ENOENT, syscall.ENOENT
	}

	// Check whether it has already been read
	file.mutex.Lock()
	defer file.mutex.Unlock()
	if file.wasRead {
		return file.contents, file.err, file.err
	}
	file.wasRead = true

	// If not, try to open it
	reader, err := file.compressed.Open()
	if err != nil {
		file.err = err
		return "", err, err
	}
	defer reader.Close()

	// Then try to read it
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		file.err = err
		return "", err, err
	}

	file.contents = string(bytes)
	return file.contents, nil, nil
}

func (fs *zipFS) OpenFile(path string) (result OpenedFile, canonicalError error, originalError error) {
	path = mangleYarnPnPVirtualPath(path)

	result, canonicalError, originalError = fs.inner.OpenFile(path)
	return
}

func (fs *zipFS) ModKey(path string) (modKey ModKey, err error) {
	path = mangleYarnPnPVirtualPath(path)

	modKey, err = fs.inner.ModKey(path)
	return
}

func (fs *zipFS) IsAbs(path string) bool {
	return fs.inner.IsAbs(path)
}

func (fs *zipFS) Abs(path string) (string, bool) {
	return fs.inner.Abs(path)
}

func (fs *zipFS) Dir(path string) string {
	if prefix, suffix, ok := parseYarnPnPVirtualPath(path); ok && suffix == "" {
		return prefix
	}
	return fs.inner.Dir(path)
}

func (fs *zipFS) Base(path string) string {
	return fs.inner.Base(path)
}

func (fs *zipFS) Ext(path string) string {
	return fs.inner.Ext(path)
}

func (fs *zipFS) Join(parts ...string) string {
	return fs.inner.Join(parts...)
}

func (fs *zipFS) Cwd() string {
	return fs.inner.Cwd()
}

func (fs *zipFS) Rel(base string, target string) (string, bool) {
	return fs.inner.Rel(base, target)
}

func (fs *zipFS) kind(dir string, base string) (symlink string, kind EntryKind) {
	return fs.inner.kind(dir, base)
}

func (fs *zipFS) WatchData() WatchData {
	return fs.inner.WatchData()
}

func parseYarnPnPVirtualPath(path string) (string, string, bool) {
	i := 0

	for {
		start := i
		slash := strings.IndexAny(path[i:], "/\\")
		if slash == -1 {
			break
		}
		i += slash + 1

		// Replace the segments "__virtual__/<segment>/<n>" with N times the ".." operation
		if path[start:i-1] == "__virtual__" {
			if slash := strings.IndexAny(path[i:], "/\\"); slash != -1 {
				var count string
				var suffix string
				j := i + slash + 1

				// Find the range of the count
				if slash := strings.IndexAny(path[j:], "/\\"); slash != -1 {
					count = path[j : j+slash]
					suffix = path[j+slash:]
				} else {
					count = path[j:]
				}

				// Parse the count
				if n, err := strconv.ParseInt(count, 10, 64); err == nil {
					prefix := path[:start]

					// Apply N times the ".." operator
					for n > 0 && (strings.HasSuffix(prefix, "/") || strings.HasSuffix(prefix, "\\")) {
						slash := strings.LastIndexAny(prefix[:len(prefix)-1], "/\\")
						if slash == -1 {
							break
						}
						prefix = prefix[:slash+1]
						n--
					}

					// Make sure the prefix and suffix work well when joined together
					if suffix == "" && strings.IndexAny(prefix, "/\\") != strings.LastIndexAny(prefix, "/\\") {
						prefix = prefix[:len(prefix)-1]
					} else if prefix == "" {
						prefix = "."
					} else if strings.HasPrefix(suffix, "/") || strings.HasPrefix(suffix, "\\") {
						suffix = suffix[1:]
					}

					return prefix, suffix, true
				}
			}
		}
	}

	return "", "", false
}

func mangleYarnPnPVirtualPath(path string) string {
	if prefix, suffix, ok := parseYarnPnPVirtualPath(path); ok {
		return prefix + suffix
	}
	return path
}
