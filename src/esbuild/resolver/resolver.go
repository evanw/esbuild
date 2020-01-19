package resolver

import (
	"esbuild/ast"
	"esbuild/fs"
	"esbuild/lexer"
	"esbuild/logging"
	"esbuild/parser"
	"strings"
	"sync"
)

type Resolver interface {
	Resolve(sourcePath string, importPath string) (string, bool)
	Read(path string) (string, bool)
	PrettyPath(path string) string
}

type resolver struct {
	fs             fs.FS
	extensionOrder []string

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCacheMutex sync.RWMutex
	dirCache      map[string]*dirInfo
}

func NewResolver(fs fs.FS, extensionOrder []string) Resolver {
	return &resolver{
		fs:             fs,
		extensionOrder: extensionOrder,
		dirCache:       make(map[string]*dirInfo),
	}
}

func (r *resolver) Resolve(sourcePath string, importPath string) (string, bool) {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together

	sourceDir := r.fs.Dir(sourcePath)
	startsWithSlash := strings.HasPrefix(importPath, "/")

	if startsWithSlash {
		sourceDir = "/"
	}

	if startsWithSlash || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return r.loadAsFileOrDirectory(r.fs.Join(sourceDir, importPath))
	}

	// Get the cached information for this directory and all parent directories
	sourceInfo := r.dirInfoCached(sourceDir)
	if sourceInfo == nil {
		return "", false
	}

	absolute, ok := r.loadNodeModules(importPath, sourceInfo)
	if ok {
		return absolute, true
	}

	// Note: node's "self references" are not currently supported
	return "", false
}

func (r *resolver) Read(path string) (string, bool) {
	contents, ok := r.fs.ReadFile(path)
	return contents, ok
}

func (r *resolver) PrettyPath(path string) string {
	if rel, ok := r.fs.RelativeToCwd(path); ok {
		return rel
	}
	return path
}

////////////////////////////////////////////////////////////////////////////////

type packageJson struct {
	absPathMain *string // The absolute path of the "main" entry point
}

type tsConfigJson struct {
	absPathBaseUrl *string // The absolute path of "compilerOptions.baseUrl"
}

type dirInfo struct {
	// These objects are immutable, so we can just point to the parent directory
	// and avoid having to lock the cache again
	parent *dirInfo

	// All relevant information about this directory
	absPath        string
	hasNodeModules bool         // Is there a "node_modules" subdirectory?
	absPathIndex   *string      // Is there an "index.js" file?
	packageJson    *packageJson // Is there a "package.json" file?
	tsConfigJson   *tsConfigJson
}

func (r *resolver) dirInfoCached(path string) *dirInfo {
	// First, check the cache
	cached, ok := func() (*dirInfo, bool) {
		r.dirCacheMutex.RLock()
		defer r.dirCacheMutex.RUnlock()
		cached, ok := r.dirCache[path]
		return cached, ok
	}()

	// Cache hit: stop now
	if ok {
		return cached
	}

	// Cache miss: read the info
	info := r.dirInfoUncached(path)

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	r.dirCacheMutex.Lock()
	defer r.dirCacheMutex.Unlock()
	r.dirCache[path] = info
	return info
}

func (r *resolver) dirInfoUncached(path string) *dirInfo {
	// Get the info for the parent directory
	var parentInfo *dirInfo
	parentDir := r.fs.Dir(path)
	if parentDir != path {
		parentInfo = r.dirInfoCached(parentDir)

		// Stop now if the parent directory doesn't exist
		if parentInfo == nil {
			return nil
		}
	}

	// List the directories
	entries := r.fs.ReadDirectory(path)
	info := &dirInfo{
		absPath: path,
		parent:  parentInfo,
	}

	// A "node_modules" directory isn't allowed to directly contain another "node_modules" directory
	info.hasNodeModules = entries["node_modules"] == fs.DirEntry && r.fs.Base(path) != "node_modules"

	// Record if this directory has a package.json file
	if entries["package.json"] == fs.FileEntry {
		info.packageJson = &packageJson{}
		if json, ok := r.parseJson(r.fs.Join(path, "package.json")); ok {
			if main, ok := getStringProperty(json, "main"); ok {
				mainPath := r.fs.Join(path, main)

				// Is it a file?
				if absolute, ok := r.loadAsFile(mainPath); ok {
					info.packageJson.absPathMain = &absolute
				} else {
					// Is it a directory?
					if mainEntries := r.fs.ReadDirectory(mainPath); mainEntries != nil {
						// Look for an "index" file with known extensions
						if absolute, ok = r.loadAsIndex(mainPath, mainEntries); ok {
							info.packageJson.absPathMain = &absolute
						}
					}
				}
			}
		}
	}

	// Record if this directory has a tsconfig.json file
	if entries["tsconfig.json"] == fs.FileEntry {
		info.tsConfigJson = &tsConfigJson{}
		if json, ok := r.parseJson(r.fs.Join(path, "tsconfig.json")); ok {
			if compilerOptions, ok := getProperty(json, "compilerOptions"); ok {
				if baseUrl, ok := getStringProperty(compilerOptions, "baseUrl"); ok {
					baseUrl := r.fs.Join(path, baseUrl)
					info.tsConfigJson.absPathBaseUrl = &baseUrl
				}
			}
		}
	}

	// Is the "main" field from "package.json" missing?
	if info.packageJson == nil || info.packageJson.absPathMain == nil {
		// Look for an "index" file with known extensions
		if absolute, ok := r.loadAsIndex(path, entries); ok {
			info.absPathIndex = &absolute
		}
	}

	return info
}

func (r *resolver) loadAsFile(path string) (string, bool) {
	// Read the directory entries once to minimize locking
	entries := r.fs.ReadDirectory(r.fs.Dir(path))

	if entries != nil {
		base := r.fs.Base(path)

		// Try the plain path without any extensions
		if entries[base] == fs.FileEntry {
			return path, true
		}

		// Try the path with extensions
		for _, ext := range r.extensionOrder {
			if entries[base+ext] == fs.FileEntry {
				return path + ext, true
			}
		}
	}

	return "", false
}

// We want to minimize the number of times directory contents are listed. For
// this reason, the directory entries are computed by the caller and then
// passed down to us.
func (r *resolver) loadAsIndex(path string, entries map[string]fs.Entry) (string, bool) {
	// Try the "index" file with extensions
	for _, ext := range r.extensionOrder {
		base := "index" + ext
		if entries[base] == fs.FileEntry {
			return r.fs.Join(path, base), true
		}
	}

	return "", false
}

func (r *resolver) parseJson(path string) (ast.Expr, bool) {
	if contents, ok := r.fs.ReadFile(path); ok {
		log, _ := logging.NewDeferLog()
		source := logging.Source{Contents: contents}
		return parser.ParseJson(log, source)
	}
	return ast.Expr{}, false
}

func getProperty(json ast.Expr, name string) (ast.Expr, bool) {
	if obj, ok := json.Data.(*ast.EObject); ok {
		for _, prop := range obj.Properties {
			if key, ok := prop.Key.Data.(*ast.EString); ok && len(key.Value) == len(name) && lexer.UTF16ToString(key.Value) == name {
				return prop.Value, true
			}
		}
	}
	return ast.Expr{}, false
}

func getStringProperty(json ast.Expr, name string) (string, bool) {
	if prop, ok := getProperty(json, name); ok {
		if value, ok := prop.Data.(*ast.EString); ok {
			return lexer.UTF16ToString(value.Value), true
		}
	}
	return "", false
}

func (r *resolver) loadAsFileOrDirectory(path string) (string, bool) {
	// Is this a file?
	absolute, ok := r.loadAsFile(path)
	if ok {
		return absolute, true
	}

	// Is this a directory?
	dirInfo := r.dirInfoCached(path)
	if dirInfo == nil {
		return "", false
	}

	// Return the "main" field from "package.json"
	if dirInfo.packageJson != nil && dirInfo.packageJson.absPathMain != nil {
		return *dirInfo.packageJson.absPathMain, true
	}

	// Return the "index.js" file
	if dirInfo.absPathIndex != nil {
		return *dirInfo.absPathIndex, true
	}

	return "", false
}

func (r *resolver) loadNodeModules(path string, dirInfo *dirInfo) (string, bool) {
	for {
		// Handle TypeScript base URLs for TypeScript code
		if dirInfo.tsConfigJson != nil && dirInfo.tsConfigJson.absPathBaseUrl != nil {
			basePath := r.fs.Join(*dirInfo.tsConfigJson.absPathBaseUrl, path)
			if absolute, ok := r.loadAsFileOrDirectory(basePath); ok {
				return absolute, true
			}
		}

		// Skip "node_modules" folders
		if dirInfo.hasNodeModules {
			absolute, ok := r.loadAsFileOrDirectory(r.fs.Join(dirInfo.absPath, "node_modules", path))
			if ok {
				return absolute, true
			}
		}

		// Go to the parent directory, stopping at the file system root
		dirInfo = dirInfo.parent
		if dirInfo == nil {
			break
		}
	}

	return "", false
}
