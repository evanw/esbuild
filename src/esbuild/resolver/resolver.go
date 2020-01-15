package resolver

import (
	"esbuild/ast"
	"esbuild/lexer"
	"esbuild/logging"
	"esbuild/parser"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Resolver interface {
	Resolve(sourcePath string, importPath string) (string, bool)
	Read(path string) (string, bool)
	FileExists(path string) bool
	PrettyPath(path string) string
}

////////////////////////////////////////////////////////////////////////////////
// This is a dummy Resolver just for tests

type mapResolver struct {
	files map[string]string
}

func CreateMapResolver(files map[string]string) Resolver {
	return &mapResolver{files}
}

func (r *mapResolver) Resolve(sourcePath string, importPath string) (string, bool) {
	return commonResolve(r, sourcePath, importPath)
}

func (r *mapResolver) Read(path string) (string, bool) {
	contents, ok := r.files[path]
	return contents, ok
}

func (r *mapResolver) FileExists(path string) bool {
	_, ok := r.files[path]
	return ok
}

func (r *mapResolver) PrettyPath(path string) string {
	return path
}

////////////////////////////////////////////////////////////////////////////////
// This is the Resolver that uses the actual file system

type fileSystemResolver struct {
	// Stores the contents of files we've read before
	fileContentsMutex sync.Mutex
	fileContents      map[string]*string

	// Stores the file entries for directories we've listed before
	fileEntriesMutex sync.Mutex
	fileEntries      map[string]map[string]bool
}

func CreateFileSystemResolver() Resolver {
	return &fileSystemResolver{
		sync.Mutex{},
		make(map[string]*string),
		sync.Mutex{},
		make(map[string]map[string]bool),
	}
}

func (r *fileSystemResolver) Resolve(sourcePath string, importPath string) (string, bool) {
	return commonResolve(r, sourcePath, importPath)
}

func (r *fileSystemResolver) Read(path string) (string, bool) {
	// First, check the cache
	cached, ok := func() (*string, bool) {
		r.fileContentsMutex.Lock()
		defer r.fileContentsMutex.Unlock()
		cached, ok := r.fileContents[path]
		return cached, ok
	}()

	// Cache hit: stop now
	if ok {
		if cached == nil {
			return "", false
		}
		return *cached, true
	}

	// Cache miss: read the file
	buffer, err := ioutil.ReadFile(path)

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The file is inaccessible so trying again is wasted.
	r.fileContentsMutex.Lock()
	defer r.fileContentsMutex.Unlock()
	if err != nil {
		r.fileContents[path] = nil
		return "", false
	}
	contents := string(buffer)
	r.fileContents[path] = &contents
	return contents, true
}

func readdir(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

func (r *fileSystemResolver) FileExists(path string) bool {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Assume the root of the file system exists
	if path == dir {
		return true
	}

	// First, check the cache
	cached, ok := func() (map[string]bool, bool) {
		r.fileEntriesMutex.Lock()
		defer r.fileEntriesMutex.Unlock()
		cached, ok := r.fileEntries[dir]
		return cached, ok
	}()

	// Cache hit: stop now
	if ok {
		if cached == nil {
			return false
		}
		return cached[base]
	}

	// Cache miss: read the directory entries
	names, err := readdir(dir)
	fileEntries := make(map[string]bool)
	if err == nil {
		for _, name := range names {
			// Use "stat", not "lstat", because we want to follow symbolic links
			if stat, err := os.Stat(filepath.Join(dir, name)); err == nil && !stat.IsDir() {
				fileEntries[name] = true
			}
		}
	}

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	r.fileEntriesMutex.Lock()
	defer r.fileEntriesMutex.Unlock()
	if err != nil {
		r.fileEntries[dir] = nil
		return false
	}
	r.fileEntries[dir] = fileEntries
	return fileEntries[base]
}

func (*fileSystemResolver) PrettyPath(path string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil {
			return rel
		}
	}
	return path
}

////////////////////////////////////////////////////////////////////////////////

var extensionOrder = []string{".jsx", ".js", ".json"}

func loadAsFile(r Resolver, path string) (string, bool) {
	if r.FileExists(path) {
		return path, true
	}

	for _, ext := range extensionOrder {
		extPath := path + ext
		if r.FileExists(extPath) {
			return extPath, true
		}
	}

	return "", false
}

func loadAsIndex(r Resolver, path string) (string, bool) {
	jsPath := filepath.Join(path, "index.js")
	if r.FileExists(jsPath) {
		return jsPath, true
	}
	jsonPath := filepath.Join(path, "index.json")
	if r.FileExists(jsonPath) {
		return jsonPath, true
	}
	return "", false
}

func parseMainFromJson(r Resolver, path string) (result string, found bool) {
	// Read the file
	contents, ok := r.Read(path)
	if ok {
		// Parse the JSON
		log, _ := logging.NewDeferLog()
		source := logging.Source{Contents: contents}
		parsed, ok := parser.ParseJson(log, source)
		if ok {
			// Check for a top-level object
			if obj, ok := parsed.Data.(*ast.EObject); ok {
				for _, prop := range obj.Properties {
					// Find the key that says "main"
					if key, ok := prop.Key.Data.(*ast.EString); ok && len(key.Value) == 4 && lexer.UTF16ToString(key.Value) == "main" {
						if value, ok := prop.Value.Data.(*ast.EString); ok {
							// Return the value for this key if it's a string
							result = lexer.UTF16ToString(value.Value)
							found = true
						}
					}
				}
			}
		}
	}
	return
}

func loadAsFileOrDirectory(r Resolver, path string) (string, bool) {
	absolute, ok := loadAsFile(r, path)
	if ok {
		return absolute, true
	}

	packageJson := filepath.Join(path, "package.json")
	if r.FileExists(packageJson) {
		if main, ok := parseMainFromJson(r, packageJson); ok {
			mainPath := filepath.Join(path, main)

			absolute, ok := loadAsFile(r, mainPath)
			if ok {
				return absolute, true
			}

			absolute, ok = loadAsIndex(r, mainPath)
			if ok {
				return absolute, true
			}
		}
	}

	return loadAsIndex(r, path)
}

func loadNodeModules(r Resolver, path string, start string) (string, bool) {
	for {
		// Skip "node_modules" folders
		if filepath.Base(start) != "node_modules" {
			absolute, ok := loadAsFileOrDirectory(r, filepath.Join(start, "node_modules", path))
			if ok {
				return absolute, true
			}
		}

		// Go to the parent directory, stopping at the file system root
		dir := filepath.Dir(start)
		if start == dir {
			break
		}
		start = dir
	}

	return "", false
}

func loadSelfReference(r Resolver, path string, start string) (string, bool) {
	// Note: this is modified from how node's resolution algorithm works. Instead
	// of just checking the closest enclosing directory with a "package.json"
	// file, it checks all enclosing directories.

	for {
		// Check this directory
		absolute, ok := loadAsFileOrDirectory(r, filepath.Join(start, path))
		if ok {
			return absolute, true
		}

		// Go to the parent directory, stopping at the file system root
		dir := filepath.Dir(start)
		if start == dir {
			break
		}
		start = dir
	}

	return "", false
}

func commonResolve(r Resolver, sourcePath string, importPath string) (string, bool) {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together

	sourceDir := filepath.Dir(sourcePath)
	startsWithSlash := strings.HasPrefix(importPath, "/")

	if startsWithSlash {
		sourceDir = "/"
	}

	if startsWithSlash || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return loadAsFileOrDirectory(r, filepath.Join(sourceDir, importPath))
	}

	absolute, ok := loadNodeModules(r, importPath, sourceDir)
	if ok {
		return absolute, true
	}

	return loadSelfReference(r, importPath, sourceDir)
}
