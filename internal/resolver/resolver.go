package resolver

import (
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/parser"
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
	log     logging.Log
	options ResolveOptions

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCacheMutex sync.RWMutex
	dirCache      map[string]*dirInfo
}

func NewResolver(fs fs.FS, log logging.Log, options ResolveOptions) Resolver {
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
		log:      log,
		options:  options,
		dirCache: make(map[string]*dirInfo),
	}
}

func (r *resolver) Resolve(sourcePath string, importPath string) (string, ResolveStatus) {
	absolute, status := r.resolveWithoutSymlinks(sourcePath, importPath)

	// If successful, resolve symlinks using the directory info cache
	if status == ResolveEnabled || status == ResolveDisabled {
		if dirInfo := r.dirInfoCached(r.fs.Dir(absolute)); dirInfo != nil {
			base := r.fs.Base(absolute)

			// Is this entry itself a symlink?
			if entry := dirInfo.entries[base]; entry.Symlink != "" {
				return entry.Symlink, status
			}

			// Is there at least one parent directory with a symlink?
			if dirInfo.absRealPath != "" {
				return r.fs.Join(dirInfo.absRealPath, base), status
			}
		}
	}

	return absolute, status
}

func (r *resolver) resolveWithoutSymlinks(sourcePath string, importPath string) (string, ResolveStatus) {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together
	result := ""

	// Get the cached information for this directory and all parent directories
	sourceDir := r.fs.Dir(sourcePath)

	if IsNonModulePath(importPath) {
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
			// Bail if the directory is missing for some reason
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
	if IsNonModulePath(importPath) {
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
	// The package.json format has two ways to specify the main file for the
	// package:
	//
	// * The "main" field. This is what node itself uses when you require() the
	//   package. It's usually (always?) in CommonJS format.
	//
	// * The "module" field. This is supposed to be in ES6 format. The idea is
	//   that "main" and "module" both have the same code but just in different
	//   formats. Then bundlers that support ES6 can prefer the "module" field
	//   over the "main" field for more efficient bundling. We support ES6 so
	//   we always prefer the "module" field over the "main" field.
	//
	absPathMain *string

	// Present if the "browser" field is present. This field is intended to be
	// used by bundlers and lets you redirect the paths of certain 3rd-party
	// modules that don't work in the browser to other modules that shim that
	// functionality. That way you don't have to rewrite the code for those 3rd-
	// party modules. For example, you might remap the native "util" node module
	// to something like https://www.npmjs.com/package/util so it works in the
	// browser.
	//
	// This field contains a mapping of absolute paths to absolute paths. Mapping
	// to an empty path indicates that the module is disabled. As far as I can
	// tell, the official spec is a GitHub repo hosted by a user account:
	// https://github.com/defunctzombie/package-browser-field-spec. The npm docs
	// say almost nothing: https://docs.npmjs.com/files/package.json.
	browserNonModuleMap map[string]*string
	browserModuleMap    map[string]*string
}

type tsConfigJson struct {
	absPathBaseUrl *string             // The absolute path of "compilerOptions.baseUrl"
	paths          map[string][]string // The absolute paths of "compilerOptions.paths"
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
	entries        map[string]fs.Entry
	hasNodeModules bool          // Is there a "node_modules" subdirectory?
	absPathIndex   *string       // Is there an "index.js" file?
	packageJson    *packageJson  // Is there a "package.json" file?
	tsConfigJson   *tsConfigJson // Is there a "tsconfig.json" file?
	absRealPath    string        // If non-empty, this is the real absolute path resolving any symlinks
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

func (r *resolver) parseJsTsConfig(file string, path string, info *dirInfo) {
	info.tsConfigJson = &tsConfigJson{}

	// Unfortunately "tsconfig.json" isn't actually JSON. It's some other
	// format that appears to be defined by the implementation details of the
	// TypeScript compiler.
	//
	// Attempt to parse it anyway by modifying the JSON parser, but just for
	// these particular files. This is likely not a completely accurate
	// emulation of what the TypeScript compiler does (e.g. string escape
	// behavior may also be different).
	options := parser.ParseJSONOptions{
		AllowComments:       true, // https://github.com/microsoft/TypeScript/issues/4987
		AllowTrailingCommas: true,
	}

	if json, ok := r.parseJSON(file, options); ok {
		if compilerOptionsJson, ok := getProperty(json, "compilerOptions"); ok {
			if baseUrlJson, ok := getProperty(compilerOptionsJson, "baseUrl"); ok {
				if baseUrl, ok := getString(baseUrlJson); ok {
					baseUrl = r.fs.Join(path, baseUrl)
					info.tsConfigJson.absPathBaseUrl = &baseUrl
				}
			}
			if pathsJson, ok := getProperty(compilerOptionsJson, "paths"); ok {
				if paths, ok := pathsJson.Data.(*ast.EObject); ok {
					info.tsConfigJson.paths = map[string][]string{}
					for _, prop := range paths.Properties {
						if key, ok := getString(prop.Key); ok {
							if value, ok := getProperty(pathsJson, key); ok {
								if array, ok := value.Data.(*ast.EArray); ok {
									for _, item := range array.Items {
										if str, ok := getString(item); ok {
											// If this is a string, it's a replacement module
											info.tsConfigJson.paths[key] = append(info.tsConfigJson.paths[key], str)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
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
		entries: entries,
	}

	// A "node_modules" directory isn't allowed to directly contain another "node_modules" directory
	base := r.fs.Base(path)
	info.hasNodeModules = base != "node_modules" && entries["node_modules"].Kind == fs.DirEntry

	// Propagate the browser scope into child directories
	if parentInfo != nil {
		info.enclosingBrowserScope = parentInfo.enclosingBrowserScope

		// Make sure "absRealPath" is the real path of the directory (resolving any symlinks)
		symlink := parentInfo.entries[base].Symlink
		if symlink != "" {
			info.absRealPath = symlink
		} else if parentInfo.absRealPath != "" {
			info.absRealPath = r.fs.Join(parentInfo.absRealPath, base)
		}
	}

	// Record if this directory has a package.json file
	if entries["package.json"].Kind == fs.FileEntry {
		info.packageJson = r.parsePackageJSON(path)

		// Propagate this browser scope into child directories
		if info.packageJson != nil && (info.packageJson.browserModuleMap != nil || info.packageJson.browserNonModuleMap != nil) {
			info.enclosingBrowserScope = info
		}
	}

	// Record if this directory has a tsconfig.json or jsconfig.json file
	if entries["tsconfig.json"].Kind == fs.FileEntry {
		r.parseJsTsConfig(r.fs.Join(path, "tsconfig.json"), path, info)
	} else if entries["jsconfig.json"].Kind == fs.FileEntry {
		r.parseJsTsConfig(r.fs.Join(path, "jsconfig.json"), path, info)
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

func (r *resolver) parsePackageJSON(path string) *packageJson {
	json, ok := r.parseJSON(r.fs.Join(path, "package.json"), parser.ParseJSONOptions{})
	if !ok {
		return nil
	}

	packageJson := &packageJson{}

	// Read the "module" property, or the "main" property as a fallback. We
	// prefer the "module" property because it's supposed to be ES6 while the
	// "main" property is supposed to be CommonJS, and ES6 helps us generate
	// better code.
	mainPath := ""
	if moduleJson, ok := getProperty(json, "module"); ok {
		if main, ok := getString(moduleJson); ok {
			mainPath = r.fs.Join(path, main)
		}
	} else if mainJson, ok := getProperty(json, "main"); ok {
		if main, ok := getString(mainJson); ok {
			mainPath = r.fs.Join(path, main)
		}
	}

	// Read the "browser" property, but only when targeting the browser
	if browserJson, ok := getProperty(json, "browser"); ok && r.options.Platform == PlatformBrowser {
		if browser, ok := getString(browserJson); ok {
			// If the value is a string, then we should just replace the main path.
			//
			// Note that this means if a package specifies "main", "module", and
			// "browser" then "browser" will win out over "module". This is the
			// same behavior as webpack: https://github.com/webpack/webpack/issues/4674.
			//
			// This is deliberate because the presence of the "browser" field is a
			// good signal that the "module" field may have non-browser stuff in it,
			// which will crash or fail to be bundled when targeting the browser.
			//
			// We both want the ability to have the option of CJS vs. ESM and the
			// option of having node vs. browser. The way to do this is to use the
			// object literal form of the "browser" field like this:
			//
			//   "main": "dist/index.node.cjs.js",
			//   "module": "dist/index.node.esm.js",
			//   "browser": {
			//     "./dist/index.node.cjs.js": "./dist/index.browser.cjs.js",
			//     "./dist/index.node.esm.js": "./dist/index.browser.esm.js"
			//   },
			//
			mainPath = r.fs.Join(path, browser)
		} else if browser, ok := browserJson.Data.(*ast.EObject); ok {
			// The value is an object
			browserModuleMap := make(map[string]*string)
			browserNonModuleMap := make(map[string]*string)

			// Remap all files in the browser field
			for _, prop := range browser.Properties {
				if key, ok := getString(prop.Key); ok && prop.Value != nil {
					isNonModulePath := IsNonModulePath(key)

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
		if entries[base].Kind == fs.FileEntry {
			return path, true
		}

		// Try the path with extensions
		for _, ext := range r.options.ExtensionOrder {
			if entries[base+ext].Kind == fs.FileEntry {
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
		if entries[base].Kind == fs.FileEntry {
			return r.fs.Join(path, base), true
		}
	}

	return "", false
}

func (r *resolver) parseJSON(path string, options parser.ParseJSONOptions) (ast.Expr, bool) {
	if contents, ok := r.fs.ReadFile(path); ok {
		source := logging.Source{
			AbsolutePath: path,
			PrettyPath:   r.PrettyPath(path),
			Contents:     contents,
		}
		return parser.ParseJSON(r.log, source, options)
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

func (r *resolver) resolvePathWithoutStar(from, path string) (string, error) {
	replaced := strings.Replace(path, "/*", "", -1)
	return r.fs.Join(from, replaced), nil
}

func (r *resolver) loadNodeModules(path string, dirInfo *dirInfo) (string, bool) {
	for {
		// Handle TypeScript base URLs for TypeScript code
		if dirInfo.tsConfigJson != nil {

			if dirInfo.tsConfigJson.absPathBaseUrl != nil {
				if dirInfo.tsConfigJson.paths != nil {
					for key, originalPaths := range dirInfo.tsConfigJson.paths {
						for _, originalPath := range originalPaths {
							if matched, err := regexp.MatchString("^"+key, path); matched && err == nil {
								if absoluteOriginalPath, err := r.resolvePathWithoutStar(*dirInfo.tsConfigJson.absPathBaseUrl, originalPath); err == nil {
									elements := strings.Split(path, "/")

									elements = elements[1:]

									resolved := append(strings.Split(absoluteOriginalPath, string(os.PathSeparator)), elements...)
									basePath := strings.Join(resolved, "/")
									if absolute, ok := r.loadAsFileOrDirectory(basePath); ok {
										return absolute, true
									}
								}
							}
						}

					}
				}
				basePath := r.fs.Join(*dirInfo.tsConfigJson.absPathBaseUrl, path)
				if absolute, ok := r.loadAsFileOrDirectory(basePath); ok {
					return absolute, true
				}

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

func IsNonModulePath(path string) bool {
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../") || path == "." || path == ".."
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
