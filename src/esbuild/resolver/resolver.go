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

type ResolveStatus uint8

const (
	ResolveMissing ResolveStatus = iota
	ResolveEnabled
	ResolveDisabled
	ResolveExternal
)

type Resolver interface {
	Resolve(sourcePath string, importPath string) (string, ResolveStatus)
	Read(path string) (string, bool)
	PrettyPath(path string) string
}

type Platform uint8

const (
	PlatformBrowser Platform = iota
	PlatformNode
)

type ResolveOptions struct {
	ExtensionOrder  []string
	Platform        Platform
	ExternalModules map[string]bool
}

type resolver struct {
	fs      fs.FS
	options ResolveOptions

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCacheMutex sync.RWMutex
	dirCache      map[string]*dirInfo
}

func NewResolver(fs fs.FS, options ResolveOptions) Resolver {
	// Bundling for node implies allowing node's builtin modules
	if options.Platform == PlatformNode {
		externalModules := make(map[string]bool)
		if options.ExternalModules != nil {
			for name, _ := range options.ExternalModules {
				externalModules[name] = true
			}
		}
		for _, name := range externalModulesForNode {
			externalModules[name] = true
		}
		options.ExternalModules = externalModules
	}

	return &resolver{
		fs:       fs,
		options:  options,
		dirCache: make(map[string]*dirInfo),
	}
}

func (r *resolver) Resolve(sourcePath string, importPath string) (string, ResolveStatus) {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together
	result := ""

	// Get the cached information for this directory and all parent directories
	sourceDir := r.fs.Dir(sourcePath)

	if isNonModulePath(importPath) {
		if absolute, ok := r.loadAsFileOrDirectory(r.fs.Join(sourceDir, importPath)); ok {
			result = absolute
		} else {
			return "", ResolveMissing
		}
	} else {
		// Check for external modules first
		if r.options.ExternalModules != nil && r.options.ExternalModules[importPath] {
			return "", ResolveExternal
		}

		sourceDirInfo := r.dirInfoCached(sourceDir)
		if sourceDirInfo == nil {
			// Bail no if the directory is missing for some reason
			return "", ResolveMissing
		}

		// Support remapping one module path to another via the "browser" field
		if sourceDirInfo.enclosingBrowserScope != nil {
			packageJson := sourceDirInfo.enclosingBrowserScope.packageJson
			if packageJson.browserModuleMap != nil {
				if remapped, ok := packageJson.browserModuleMap[importPath]; ok {
					if remapped == nil {
						// "browser": {"module": false}
						if absolute, ok := r.loadNodeModules(importPath, sourceDirInfo); ok {
							return absolute, ResolveDisabled
						} else {
							return "", ResolveMissing
						}
					} else {
						// "browser": {"module": "./some-file"}
						// "browser": {"module": "another-module"}
						importPath = *remapped
						sourceDirInfo = sourceDirInfo.enclosingBrowserScope
					}
				}
			}
		}

		if absolute, ok := r.resolveWithoutRemapping(sourceDirInfo, importPath); ok {
			result = absolute
		} else {
			// Note: node's "self references" are not currently supported
			return "", ResolveMissing
		}
	}

	// Check the directory that contains this file
	resultDir := r.fs.Dir(result)
	resultDirInfo := r.dirInfoCached(resultDir)

	// Support remapping one non-module path to another via the "browser" field
	if resultDirInfo != nil && resultDirInfo.enclosingBrowserScope != nil {
		packageJson := resultDirInfo.enclosingBrowserScope.packageJson
		if packageJson.browserNonModuleMap != nil {
			if remapped, ok := packageJson.browserNonModuleMap[result]; ok {
				if remapped == nil {
					return result, ResolveDisabled
				}
				result, ok = r.resolveWithoutRemapping(resultDirInfo.enclosingBrowserScope, *remapped)
				if !ok {
					return "", ResolveMissing
				}
			}
		}
	}

	return result, ResolveEnabled
}

func (r *resolver) resolveWithoutRemapping(sourceDirInfo *dirInfo, importPath string) (string, bool) {
	if isNonModulePath(importPath) {
		return r.loadAsFileOrDirectory(r.fs.Join(sourceDirInfo.absPath, importPath))
	} else {
		return r.loadNodeModules(importPath, sourceDirInfo)
	}
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
	// The absolute path of the "main" entry point
	absPathMain *string

	// Present if the "browser" field is present. This contains a mapping of
	// absolute paths to absolute paths. Mapping to an empty path indicates that
	// the module is disabled. As far as I can tell, the official spec is a random
	// GitHub repo: https://github.com/defunctzombie/package-browser-field-spec.
	// The npm docs say almost nothing: https://docs.npmjs.com/files/package.json.
	browserNonModuleMap map[string]*string
	browserModuleMap    map[string]*string
}

type tsConfigJson struct {
	absPathBaseUrl *string // The absolute path of "compilerOptions.baseUrl"
}

type dirInfo struct {
	// These objects are immutable, so we can just point to the parent directory
	// and avoid having to lock the cache again
	parent *dirInfo

	// A pointer to the enclosing dirInfo with a valid "browser" field in
	// package.json. We need this to remap paths after they have been resolved.
	enclosingBrowserScope *dirInfo

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

	// Propagate the browser scope into child directories
	if parentInfo != nil {
		info.enclosingBrowserScope = parentInfo.enclosingBrowserScope
	}

	// A "node_modules" directory isn't allowed to directly contain another "node_modules" directory
	info.hasNodeModules = entries["node_modules"] == fs.DirEntry && r.fs.Base(path) != "node_modules"

	// Record if this directory has a package.json file
	if entries["package.json"] == fs.FileEntry {
		info.packageJson = r.parsePackageJson(path)

		// Propagate this browser scope into child directories
		if info.packageJson != nil && (info.packageJson.browserModuleMap != nil || info.packageJson.browserNonModuleMap != nil) {
			info.enclosingBrowserScope = info
		}
	}

	// Record if this directory has a tsconfig.json file
	if entries["tsconfig.json"] == fs.FileEntry {
		info.tsConfigJson = &tsConfigJson{}
		if json, ok := r.parseJson(r.fs.Join(path, "tsconfig.json")); ok {
			if compilerOptionsJson, ok := getProperty(json, "compilerOptions"); ok {
				if baseUrlJson, ok := getProperty(compilerOptionsJson, "baseUrl"); ok {
					if baseUrl, ok := getString(baseUrlJson); ok {
						baseUrl := r.fs.Join(path, baseUrl)
						info.tsConfigJson.absPathBaseUrl = &baseUrl
					}
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

func (r *resolver) parsePackageJson(path string) *packageJson {
	json, ok := r.parseJson(r.fs.Join(path, "package.json"))
	if !ok {
		return nil
	}

	packageJson := &packageJson{}

	// Read the "main" property
	mainPath := ""
	if mainJson, ok := getProperty(json, "main"); ok {
		if main, ok := getString(mainJson); ok {
			mainPath = r.fs.Join(path, main)
		}
	}

	// Read the "browser" property, but only when targeting the browser
	if browserJson, ok := getProperty(json, "browser"); ok && r.options.Platform == PlatformBrowser {
		if browser, ok := getString(browserJson); ok {
			// The value is a string
			mainPath = r.fs.Join(path, browser)
		} else if browser, ok := browserJson.Data.(*ast.EObject); ok {
			// The value is an object
			browserModuleMap := make(map[string]*string)
			browserNonModuleMap := make(map[string]*string)

			// Remap all files in the browser field
			for _, prop := range browser.Properties {
				if key, ok := getString(prop.Key); ok && prop.Value != nil {
					isNonModulePath := isNonModulePath(key)

					// Make this an absolute path if it's not a module
					if isNonModulePath {
						key = r.fs.Join(path, key)
					}

					if value, ok := getString(*prop.Value); ok {
						// If this is a string, it's a replacement module
						if isNonModulePath {
							browserNonModuleMap[key] = &value
						} else {
							browserModuleMap[key] = &value
						}
					} else if value, ok := getBool(*prop.Value); ok && !value {
						// If this is false, it means the module is disabled
						if isNonModulePath {
							browserNonModuleMap[key] = nil
						} else {
							browserModuleMap[key] = nil
						}
					}
				}
			}

			packageJson.browserModuleMap = browserModuleMap
			packageJson.browserNonModuleMap = browserNonModuleMap
		}
	}

	// Delay parsing "main" into an absolute path in case "browser" replaces it
	if mainPath != "" {
		// Is it a file?
		if absolute, ok := r.loadAsFile(mainPath); ok {
			packageJson.absPathMain = &absolute
		} else {
			// Is it a directory?
			if mainEntries := r.fs.ReadDirectory(mainPath); mainEntries != nil {
				// Look for an "index" file with known extensions
				if absolute, ok = r.loadAsIndex(mainPath, mainEntries); ok {
					packageJson.absPathMain = &absolute
				}
			}
		}
	}

	return packageJson
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
		for _, ext := range r.options.ExtensionOrder {
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
	for _, ext := range r.options.ExtensionOrder {
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
			if key, ok := prop.Key.Data.(*ast.EString); ok && key.Value != nil &&
				len(key.Value) == len(name) && lexer.UTF16ToString(key.Value) == name {
				return *prop.Value, true
			}
		}
	}
	return ast.Expr{}, false
}

func getString(json ast.Expr) (string, bool) {
	if value, ok := json.Data.(*ast.EString); ok {
		return lexer.UTF16ToString(value.Value), true
	}
	return "", false
}

func getBool(json ast.Expr) (bool, bool) {
	if value, ok := json.Data.(*ast.EBoolean); ok {
		return value.Value, true
	}
	return false, false
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

func isNonModulePath(path string) bool {
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || path == "."
}

var externalModulesForNode = []string{
	"assert",
	"async_hooks",
	"buffer",
	"child_process",
	"cluster",
	"console",
	"constants",
	"crypto",
	"dgram",
	"dns",
	"domain",
	"events",
	"fs",
	"http",
	"http2",
	"https",
	"inspector",
	"module",
	"net",
	"os",
	"path",
	"perf_hooks",
	"process",
	"punycode",
	"querystring",
	"readline",
	"repl",
	"stream",
	"string_decoder",
	"sys",
	"timers",
	"tls",
	"trace_events",
	"tty",
	"url",
	"util",
	"v8",
	"vm",
	"worker_threads",
	"zlib",
}
