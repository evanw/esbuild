package resolver

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

var defaultMainFields = map[config.Platform][]string{
	// Note that this means if a package specifies "main", "module", and
	// "browser" then "browser" will win out over "module". This is the
	// same behavior as webpack: https://github.com/webpack/webpack/issues/4674.
	//
	// This is deliberate because the presence of the "browser" field is a
	// good signal that the "module" field may have non-browser stuff in it,
	// which will crash or fail to be bundled when targeting the browser.
	config.PlatformBrowser: {"browser", "module", "main"},

	// Note that this means if a package specifies "module" and "main", the ES6
	// module will not be selected. This means tree shaking will not work when
	// targeting node environments.
	//
	// This is unfortunately necessary for compatibility. Some packages
	// incorrectly treat the "module" field as "code for the browser". It
	// actually means "code for ES6 environments" which includes both node
	// and the browser.
	//
	// For example, the package "@firebase/app" prints a warning on startup about
	// the bundler incorrectly using code meant for the browser if the bundler
	// selects the "module" field instead of the "main" field.
	//
	// If you want to enable tree shaking when targeting node, you will have to
	// configure the main fields to be "module" and then "main". Keep in mind
	// that some packages may break if you do this.
	config.PlatformNode: {"main", "module"},

	// The neutral platform is for people that don't want esbuild to try to
	// pick good defaults for their platform. In that case, the list of main
	// fields is empty by default. You must explicitly configure it yourself.
	config.PlatformNeutral: {},
}

// Path resolution is a mess. One tricky issue is the "module" override for the
// "main" field in "package.json" files. Bundlers generally prefer "module" over
// "main" but that breaks packages that export a function in "main" for use with
// "require()", since resolving to "module" means an object will be returned. We
// attempt to handle this automatically by having import statements resolve to
// "module" but switch that out later for "main" if "require()" is used too.
type PathPair struct {
	// Either secondary will be empty, or primary will be "module" and secondary
	// will be "main"
	Primary   logger.Path
	Secondary logger.Path
}

func (pp *PathPair) iter() []*logger.Path {
	result := []*logger.Path{&pp.Primary, &pp.Secondary}
	if !pp.HasSecondary() {
		result = result[:1]
	}
	return result
}

func (pp *PathPair) HasSecondary() bool {
	return pp.Secondary.Text != ""
}

type IgnoreIfUnusedData struct {
	Source *logger.Source
	Range  logger.Range

	// If true, "sideEffects" was an array. If false, "sideEffects" was false.
	IsSideEffectsArrayInJSON bool
}

type ResolveResult struct {
	PathPair PathPair

	// If this was resolved by a plugin, the plugin gets to store its data here
	PluginData interface{}

	// If not empty, these should override the default values
	JSXFactory  []string // Default if empty: "React.createElement"
	JSXFragment []string // Default if empty: "React.Fragment"

	IsExternal    bool
	DifferentCase *fs.DifferentCase

	// If true, any ES6 imports to this file can be considered to have no side
	// effects. This means they should be removed if unused.
	IgnorePrimaryIfUnused *IgnoreIfUnusedData

	// If true, the class field transform should use Object.defineProperty().
	UseDefineForClassFieldsTS bool

	// If true, unused imports are retained in TypeScript code. This matches the
	// behavior of the "importsNotUsedAsValues" field in "tsconfig.json" when the
	// value is not "remove".
	PreserveUnusedImportsTS bool
}

type Resolver interface {
	Resolve(sourceDir string, importPath string, kind ast.ImportKind) (result *ResolveResult, notes []logger.MsgData)
	ResolveAbs(absPath string) *ResolveResult
	PrettyPath(path logger.Path) string

	// This tries to run "Resolve" on a package path as a relative path. If
	// successful, the user just forgot a leading "./" in front of the path.
	ProbeResolvePackageAsRelative(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult
}

type resolver struct {
	fs      fs.FS
	log     logger.Log
	caches  *cache.CacheSet
	options config.Options

	// These are sets that represent various conditions for the "exports" field
	// in package.json.
	esmConditionsDefault map[string]bool
	esmConditionsImport  map[string]bool
	esmConditionsRequire map[string]bool

	// A special filtered import order for CSS "@import" imports.
	//
	// The "resolve extensions" setting determines the order of implicit
	// extensions to try when resolving imports with the extension omitted.
	// Sometimes people create a JavaScript/TypeScript file and a CSS file with
	// the same name when they create a component. At a high level, users expect
	// implicit extensions to resolve to the JS file when being imported from JS
	// and to resolve to the CSS file when being imported from CSS.
	//
	// Different bundlers handle this in different ways. Parcel handles this by
	// having the resolver prefer the same extension as the importing file in
	// front of the configured "resolve extensions" order. Webpack's "css-loader"
	// plugin just explicitly configures a special "resolve extensions" order
	// consisting of only ".css" for CSS files.
	//
	// It's unclear what behavior is best here. What we currently do is to create
	// a special filtered version of the configured "resolve extensions" order
	// for CSS files that filters out any extension that has been explicitly
	// configured with a non-CSS loader. This still gives users control over the
	// order but avoids the scenario where we match an import in a CSS file to a
	// JavaScript-related file. It's probably not perfect with plugins in the
	// picture but it's better than some alternatives and probably pretty good.
	atImportExtensionOrder []string

	// This mutex serves two purposes. First of all, it guards access to "dirCache"
	// which is potentially mutated during path resolution. But this mutex is also
	// necessary for performance. The "React admin" benchmark mysteriously runs
	// twice as fast when this mutex is locked around the whole resolve operation
	// instead of around individual accesses to "dirCache". For some reason,
	// reducing parallelism in the resolver helps the rest of the bundler go
	// faster. I'm not sure why this is but please don't change this unless you
	// do a lot of testing with various benchmarks and there aren't any regressions.
	mutex sync.Mutex

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCache map[string]*dirInfo
}

func NewResolver(fs fs.FS, log logger.Log, caches *cache.CacheSet, options config.Options) Resolver {
	// Bundling for node implies allowing node's builtin modules
	if options.Platform == config.PlatformNode {
		externalNodeModules := make(map[string]bool)
		if options.ExternalModules.NodeModules != nil {
			for name := range options.ExternalModules.NodeModules {
				externalNodeModules[name] = true
			}
		}
		for name := range BuiltInNodeModules {
			externalNodeModules[name] = true
		}
		options.ExternalModules.NodeModules = externalNodeModules
	}

	// Filter out non-CSS extensions for CSS "@import" imports
	atImportExtensionOrder := make([]string, 0, len(options.ExtensionOrder))
	for _, ext := range options.ExtensionOrder {
		if loader, ok := options.ExtensionToLoader[ext]; ok && loader != config.LoaderCSS {
			continue
		}
		atImportExtensionOrder = append(atImportExtensionOrder, ext)
	}

	// Generate the condition sets for interpreting the "exports" field
	esmConditionsDefault := map[string]bool{}
	esmConditionsImport := map[string]bool{"import": true}
	esmConditionsRequire := map[string]bool{"require": true}
	for _, condition := range options.Conditions {
		esmConditionsDefault[condition] = true
	}
	switch options.Platform {
	case config.PlatformBrowser:
		esmConditionsDefault["browser"] = true
	case config.PlatformNode:
		esmConditionsDefault["node"] = true
	}
	for key := range esmConditionsDefault {
		esmConditionsImport[key] = true
		esmConditionsRequire[key] = true
	}

	return &resolver{
		fs:                     fs,
		log:                    log,
		options:                options,
		caches:                 caches,
		dirCache:               make(map[string]*dirInfo),
		atImportExtensionOrder: atImportExtensionOrder,
		esmConditionsDefault:   esmConditionsDefault,
		esmConditionsImport:    esmConditionsImport,
		esmConditionsRequire:   esmConditionsRequire,
	}
}

func (r *resolver) Resolve(sourceDir string, importPath string, kind ast.ImportKind) (*ResolveResult, []logger.MsgData) {
	// Certain types of URLs default to being external for convenience
	if r.isExternalPattern(importPath) ||

		// "fill: url(#filter);"
		(kind.IsFromCSS() && strings.HasPrefix(importPath, "#")) ||

		// "background: url(http://example.com/images/image.png);"
		strings.HasPrefix(importPath, "http://") ||

		// "background: url(https://example.com/images/image.png);"
		strings.HasPrefix(importPath, "https://") ||

		// "background: url(//example.com/images/image.png);"
		strings.HasPrefix(importPath, "//") {

		return &ResolveResult{
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
		}, nil
	}

	if parsed, ok := ParseDataURL(importPath); ok {
		// "import 'data:text/javascript,console.log(123)';"
		// "@import 'data:text/css,body{background:white}';"
		if parsed.DecodeMIMEType() != MIMETypeUnsupported {
			return &ResolveResult{
				PathPair: PathPair{Primary: logger.Path{Text: importPath, Namespace: "dataurl"}},
			}, nil
		}

		// "background: url(data:image/png;base64,iVBORw0KGgo=);"
		return &ResolveResult{
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
		}, nil
	}

	// Fail now if there is no directory to resolve in. This can happen for
	// virtual modules (e.g. stdin) if a resolve directory is not specified.
	if sourceDir == "" {
		return nil, nil
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	result, notes := r.resolveWithoutSymlinks(sourceDir, importPath, kind)
	if result == nil {
		// If resolution failed, try again with the URL query and/or hash removed
		suffix := strings.IndexAny(importPath, "?#")
		if suffix < 1 {
			return nil, notes
		}
		if result2, notes2 := r.resolveWithoutSymlinks(sourceDir, importPath[:suffix], kind); result2 == nil {
			return nil, notes
		} else {
			result = result2
			notes = notes2
			result.PathPair.Primary.IgnoredSuffix = importPath[suffix:]
			if result.PathPair.HasSecondary() {
				result.PathPair.Secondary.IgnoredSuffix = importPath[suffix:]
			}
		}
	}

	// If successful, resolve symlinks using the directory info cache
	r.finalizeResolve(result)
	return result, notes
}

func (r *resolver) isExternalPattern(path string) bool {
	for _, pattern := range r.options.ExternalModules.Patterns {
		if len(path) >= len(pattern.Prefix)+len(pattern.Suffix) &&
			strings.HasPrefix(path, pattern.Prefix) &&
			strings.HasSuffix(path, pattern.Suffix) {
			return true
		}
	}
	return false
}

func (r *resolver) ResolveAbs(absPath string) *ResolveResult {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Just decorate the absolute path with information from parent directories
	result := &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}}
	r.finalizeResolve(result)
	return result
}

func (r *resolver) ProbeResolvePackageAsRelative(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult {
	absPath := r.fs.Join(sourceDir, importPath)

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if pair, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
		result := &ResolveResult{PathPair: pair, DifferentCase: diffCase}
		r.finalizeResolve(result)
		return result
	}
	return nil
}

func IsInsideNodeModules(path string) bool {
	for {
		// This is written in a platform-independent manner because it's run on
		// user-specified paths which can be arbitrary non-file-system things. So
		// for example Windows paths may end up being used on Unix or URLs may end
		// up being used on Windows. Be consistently agnostic to which kind of
		// slash is used on all platforms.
		slash := strings.LastIndexAny(path, "/\\")
		if slash == -1 {
			return false
		}
		dir, base := path[:slash], path[slash+1:]
		if base == "node_modules" {
			return true
		}
		path = dir
	}
}

func (r *resolver) finalizeResolve(result *ResolveResult) {
	for _, path := range result.PathPair.iter() {
		if path.Namespace == "file" {
			if dirInfo := r.dirInfoCached(r.fs.Dir(path.Text)); dirInfo != nil {
				base := r.fs.Base(path.Text)

				// Look up this file in the "sideEffects" map in the nearest enclosing
				// directory with a "package.json" file.
				//
				// Only do this for the primary path. Some packages have the primary
				// path marked as having side effects and the secondary path marked
				// as not having side effects. This is likely a bug in the package
				// definition but we don't want to consider the primary path as not
				// having side effects just because the secondary path is marked as
				// not having side effects.
				if *path == result.PathPair.Primary {
					for info := dirInfo; info != nil; info = info.parent {
						if info.packageJSON != nil {
							if info.packageJSON.sideEffectsMap != nil {
								hasSideEffects := false
								if info.packageJSON.sideEffectsMap[path.Text] {
									// Fast path: map lookup
									hasSideEffects = true
								} else {
									// Slow path: glob tests
									for _, re := range info.packageJSON.sideEffectsRegexps {
										if re.MatchString(path.Text) {
											hasSideEffects = true
											break
										}
									}
								}
								if !hasSideEffects {
									result.IgnorePrimaryIfUnused = info.packageJSON.ignoreIfUnusedData
								}
							}
							break
						}
					}
				}

				// Copy various fields from the nearest enclosing "tsconfig.json" file if present
				if path == &result.PathPair.Primary && dirInfo.tsConfigJSON != nil {
					result.JSXFactory = dirInfo.tsConfigJSON.JSXFactory
					result.JSXFragment = dirInfo.tsConfigJSON.JSXFragmentFactory
					result.UseDefineForClassFieldsTS = dirInfo.tsConfigJSON.UseDefineForClassFields
					result.PreserveUnusedImportsTS = dirInfo.tsConfigJSON.PreserveImportsNotUsedAsValues
				}

				if !r.options.PreserveSymlinks {
					if entry, _ := dirInfo.entries.Get(base); entry != nil {
						if symlink := entry.Symlink(r.fs); symlink != "" {
							// Is this entry itself a symlink?
							path.Text = symlink
						} else if dirInfo.absRealPath != "" {
							// Is there at least one parent directory with a symlink?
							path.Text = r.fs.Join(dirInfo.absRealPath, base)
						}
					}
				}
			}
		}
	}
}

func (r *resolver) resolveWithoutSymlinks(sourceDir string, importPath string, kind ast.ImportKind) (*ResolveResult, []logger.MsgData) {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together
	var result ResolveResult

	// Return early if this is already an absolute path. In addition to asking
	// the file system whether this is an absolute path, we also explicitly check
	// whether it starts with a "/" and consider that an absolute path too. This
	// is because relative paths can technically start with a "/" on Windows
	// because it's not an absolute path on Windows. Then people might write code
	// with imports that start with a "/" that works fine on Windows only to
	// experience unexpected build failures later on other operating systems.
	// Treating these paths as absolute paths on all platforms means Windows
	// users will not be able to accidentally make use of these paths.
	if strings.HasPrefix(importPath, "/") || r.fs.IsAbs(importPath) {
		// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
		if dirInfo := r.dirInfoCached(sourceDir); dirInfo != nil && dirInfo.tsConfigJSON != nil && dirInfo.tsConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(dirInfo.tsConfigJSON, importPath, kind); ok {
				return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}, nil
			}
		}

		if r.options.ExternalModules.AbsPaths != nil && r.options.ExternalModules.AbsPaths[importPath] {
			// If the string literal in the source text is an absolute path and has
			// been marked as an external module, mark it as *not* an absolute path.
			// That way we preserve the literal text in the output and don't generate
			// a relative path from the output directory to that path.
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}, nil
		}

		// Run node's resolution rules (e.g. adding ".js")
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(importPath, kind); ok {
			return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}, nil
		}
		return nil, nil
	}

	// Check both relative and package paths for CSS URL tokens, with relative
	// paths taking precedence over package paths to match Webpack behavior.
	isPackagePath := IsPackagePath(importPath)
	checkRelative := !isPackagePath || kind == ast.ImportURL
	checkPackage := isPackagePath

	if checkRelative {
		absPath := r.fs.Join(sourceDir, importPath)

		// Check for external packages first
		if r.options.ExternalModules.AbsPaths != nil && r.options.ExternalModules.AbsPaths[absPath] {
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}, IsExternal: true}, nil
		}

		// Check the non-package "browser" map for the first time (1 out of 2)
		importDirInfo := r.dirInfoCached(r.fs.Dir(absPath))
		if importDirInfo != nil && importDirInfo.enclosingBrowserScope != nil {
			if packageJSON := importDirInfo.enclosingBrowserScope.packageJSON; packageJSON.browserNonPackageMap != nil {
				if remapped, ok := packageJSON.browserNonPackageMap[absPath]; ok {
					if remapped == nil {
						return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}}, nil
					} else if remappedResult, ok, diffCase, _ := r.resolveWithoutRemapping(importDirInfo.enclosingBrowserScope, *remapped, kind); ok {
						result = ResolveResult{PathPair: remappedResult, DifferentCase: diffCase}
						checkRelative = false
						checkPackage = false
					}
				}
			}
		}

		if checkRelative {
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
				checkPackage = false
				result = ResolveResult{PathPair: absolute, DifferentCase: diffCase}
			} else if !checkPackage {
				return nil, nil
			}
		}
	}

	if checkPackage {
		// Check for external packages first
		if r.options.ExternalModules.NodeModules != nil {
			query := importPath
			for {
				if r.options.ExternalModules.NodeModules[query] {
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}, nil
				}

				// If the module "foo" has been marked as external, we also want to treat
				// paths into that module such as "foo/bar" as external too.
				slash := strings.LastIndexByte(query, '/')
				if slash == -1 {
					break
				}
				query = query[:slash]
			}
		}

		sourceDirInfo := r.dirInfoCached(sourceDir)
		if sourceDirInfo == nil {
			// Bail if the directory is missing for some reason
			return nil, nil
		}

		// Support remapping one package path to another via the "browser" field
		if sourceDirInfo.enclosingBrowserScope != nil {
			packageJSON := sourceDirInfo.enclosingBrowserScope.packageJSON
			if packageJSON.browserPackageMap != nil {
				if remapped, ok := packageJSON.browserPackageMap[importPath]; ok {
					if remapped == nil {
						// "browser": {"module": false}
						if absolute, ok, diffCase, _ := r.loadNodeModules(importPath, kind, sourceDirInfo); ok {
							absolute.Primary = logger.Path{Text: absolute.Primary.Text, Namespace: "file", Flags: logger.PathDisabled}
							if absolute.HasSecondary() {
								absolute.Secondary = logger.Path{Text: absolute.Secondary.Text, Namespace: "file", Flags: logger.PathDisabled}
							}
							return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}, nil
						} else {
							return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Flags: logger.PathDisabled}}, DifferentCase: diffCase}, nil
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

		if absolute, ok, diffCase, notes := r.resolveWithoutRemapping(sourceDirInfo, importPath, kind); ok {
			result = ResolveResult{PathPair: absolute, DifferentCase: diffCase}
		} else {
			// Note: node's "self references" are not currently supported
			return nil, notes
		}
	}

	// Check the directory that contains this file
	for _, path := range result.PathPair.iter() {
		resultDir := r.fs.Dir(path.Text)
		resultDirInfo := r.dirInfoCached(resultDir)

		// Check the non-package "browser" map for the second time (2 out of 2)
		if resultDirInfo != nil && resultDirInfo.enclosingBrowserScope != nil {
			packageJSON := resultDirInfo.enclosingBrowserScope.packageJSON
			if packageJSON.browserNonPackageMap != nil {
				if remapped, ok := packageJSON.browserNonPackageMap[path.Text]; ok {
					if remapped == nil {
						path.Flags |= logger.PathDisabled
					} else if remappedResult, ok, _, _ := r.resolveWithoutRemapping(resultDirInfo.enclosingBrowserScope, *remapped, kind); ok {
						*path = remappedResult.Primary
					} else {
						return nil, nil
					}
				}
			}
		}
	}

	return &result, nil
}

func (r *resolver) resolveWithoutRemapping(sourceDirInfo *dirInfo, importPath string, kind ast.ImportKind) (PathPair, bool, *fs.DifferentCase, []logger.MsgData) {
	if IsPackagePath(importPath) {
		return r.loadNodeModules(importPath, kind, sourceDirInfo)
	} else {
		pair, ok, diffCase := r.loadAsFileOrDirectory(r.fs.Join(sourceDirInfo.absPath, importPath), kind)
		return pair, ok, diffCase, nil
	}
}

func (r *resolver) PrettyPath(path logger.Path) string {
	if path.Namespace == "file" {
		if rel, ok := r.fs.Rel(r.fs.Cwd(), path.Text); ok {
			path.Text = rel
		}

		// These human-readable paths are used in error messages, comments in output
		// files, source names in source maps, and paths in the metadata JSON file.
		// These should be platform-independent so our output doesn't depend on which
		// operating system it was run. Replace Windows backward slashes with standard
		// forward slashes.
		path.Text = strings.ReplaceAll(path.Text, "\\", "/")
	} else if path.Namespace != "" {
		path.Text = fmt.Sprintf("%s:%s", path.Namespace, path.Text)
	}

	if path.IsDisabled() {
		path.Text = "(disabled):" + path.Text
	}

	return path.Text + path.IgnoredSuffix
}

////////////////////////////////////////////////////////////////////////////////

type dirInfo struct {
	// These objects are immutable, so we can just point to the parent directory
	// and avoid having to lock the cache again
	parent *dirInfo

	// A pointer to the enclosing dirInfo with a valid "browser" field in
	// package.json. We need this to remap paths after they have been resolved.
	enclosingBrowserScope *dirInfo

	// All relevant information about this directory
	absPath        string
	entries        fs.DirEntries
	hasNodeModules bool          // Is there a "node_modules" subdirectory?
	absPathIndex   *string       // Is there an "index.js" file?
	packageJSON    *packageJSON  // Is there a "package.json" file?
	tsConfigJSON   *TSConfigJSON // Is there a "tsconfig.json" file in this directory or a parent directory?
	absRealPath    string        // If non-empty, this is the real absolute path resolving any symlinks
}

func (r *resolver) dirInfoCached(path string) *dirInfo {
	// First, check the cache
	cached, ok := r.dirCache[path]

	// Cache hit: stop now
	if ok {
		return cached
	}

	// Cache miss: read the info
	info := r.dirInfoUncached(path)

	// Update the cache unconditionally. Even if the read failed, we don't want to
	// retry again later. The directory is inaccessible so trying again is wasted.
	r.dirCache[path] = info
	return info
}

var parseErrorImportCycle = errors.New("(import cycle)")
var parseErrorAlreadyLogged = errors.New("(error already logged)")

// This may return "parseErrorAlreadyLogged" in which case there was a syntax
// error, but it's already been reported. No further errors should be logged.
//
// Nested calls may also return "parseErrorImportCycle". In that case the
// caller is responsible for logging an appropriate error message.
func (r *resolver) parseTSConfig(file string, visited map[string]bool) (*TSConfigJSON, error) {
	// Don't infinite loop if a series of "extends" links forms a cycle
	if visited[file] {
		return nil, parseErrorImportCycle
	}
	visited[file] = true

	contents, err := r.caches.FSCache.ReadFile(r.fs, file)
	if err != nil {
		return nil, err
	}

	keyPath := logger.Path{Text: file, Namespace: "file"}
	source := logger.Source{
		KeyPath:    keyPath,
		PrettyPath: r.PrettyPath(keyPath),
		Contents:   contents,
	}
	fileDir := r.fs.Dir(file)

	result := ParseTSConfigJSON(r.log, source, &r.caches.JSONCache, func(extends string, extendsRange logger.Range) *TSConfigJSON {
		if IsPackagePath(extends) {
			// If this is a package path, try to resolve it to a "node_modules"
			// folder. This doesn't use the normal node module resolution algorithm
			// both because it's different (e.g. we don't want to match a directory)
			// and because it would deadlock since we're currently in the middle of
			// populating the directory info cache.
			current := fileDir
			for {
				// Skip "node_modules" folders
				if r.fs.Base(current) != "node_modules" {
					join := r.fs.Join(current, "node_modules", extends)
					filesToCheck := []string{r.fs.Join(join, "tsconfig.json"), join, join + ".json"}
					for _, fileToCheck := range filesToCheck {
						base, err := r.parseTSConfig(fileToCheck, visited)
						if err == nil {
							return base
						} else if err == syscall.ENOENT {
							continue
						} else if err == parseErrorImportCycle {
							r.log.AddRangeWarning(&source, extendsRange,
								fmt.Sprintf("Base config file %q forms cycle", extends))
						} else if err != parseErrorAlreadyLogged {
							r.log.AddRangeError(&source, extendsRange,
								fmt.Sprintf("Cannot read file %q: %s",
									r.PrettyPath(logger.Path{Text: fileToCheck, Namespace: "file"}), err.Error()))
						}
						return nil
					}
				}

				// Go to the parent directory, stopping at the file system root
				next := r.fs.Dir(current)
				if current == next {
					break
				}
				current = next
			}
		} else {
			// If this is a regular path, search relative to the enclosing directory
			extendsFile := extends
			if !r.fs.IsAbs(extends) {
				extendsFile = r.fs.Join(fileDir, extends)
			}
			for _, fileToCheck := range []string{extendsFile, extendsFile + ".json"} {
				base, err := r.parseTSConfig(fileToCheck, visited)
				if err == nil {
					return base
				} else if err == syscall.ENOENT {
					continue
				} else if err == parseErrorImportCycle {
					r.log.AddRangeWarning(&source, extendsRange,
						fmt.Sprintf("Base config file %q forms cycle", extends))
				} else if err != parseErrorAlreadyLogged {
					r.log.AddRangeError(&source, extendsRange,
						fmt.Sprintf("Cannot read file %q: %s",
							r.PrettyPath(logger.Path{Text: fileToCheck, Namespace: "file"}), err.Error()))
				}
				return nil
			}
		}

		// Suppress warnings about missing base config files inside "node_modules"
		if !IsInsideNodeModules(file) {
			r.log.AddRangeWarning(&source, extendsRange,
				fmt.Sprintf("Cannot find base config file %q", extends))
		}

		return nil
	})

	if result == nil {
		return nil, parseErrorAlreadyLogged
	}

	if result.BaseURL != nil && !r.fs.IsAbs(*result.BaseURL) {
		*result.BaseURL = r.fs.Join(fileDir, *result.BaseURL)
	}

	if result.Paths != nil && !r.fs.IsAbs(result.BaseURLForPaths) {
		result.BaseURLForPaths = r.fs.Join(fileDir, result.BaseURLForPaths)
	}

	return result, nil
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
	entries, err := r.fs.ReadDirectory(path)
	if err == syscall.EACCES {
		// Just pretend this directory is empty if we can't access it. This is the
		// case on Unix for directories that only have the execute permission bit
		// set. It means we will just pass through the empty directory and
		// continue to check the directories above it, which is now node behaves.
		entries = fs.MakeEmptyDirEntries(path)
		err = nil
	}
	if err != nil {
		// Ignore "ENOTDIR" here so that calling "ReadDirectory" on a file behaves
		// as if there is nothing there at all instead of causing an error due to
		// the directory actually being a file. This is a workaround for situations
		// where people try to import from a path containing a file as a parent
		// directory. The "pnpm" package manager generates a faulty "NODE_PATH"
		// list which contains such paths and treating them as missing means we just
		// ignore them during path resolution.
		if err != syscall.ENOENT && err != syscall.ENOTDIR {
			r.log.AddError(nil, logger.Loc{},
				fmt.Sprintf("Cannot read directory %q: %s",
					r.PrettyPath(logger.Path{Text: path, Namespace: "file"}), err.Error()))
		}
		return nil
	}
	info := &dirInfo{
		absPath: path,
		parent:  parentInfo,
		entries: entries,
	}

	// A "node_modules" directory isn't allowed to directly contain another "node_modules" directory
	base := r.fs.Base(path)
	if base != "node_modules" {
		if entry, _ := entries.Get("node_modules"); entry != nil {
			info.hasNodeModules = entry.Kind(r.fs) == fs.DirEntry
		}
	}

	// Propagate the browser scope into child directories
	if parentInfo != nil {
		info.enclosingBrowserScope = parentInfo.enclosingBrowserScope

		// Make sure "absRealPath" is the real path of the directory (resolving any symlinks)
		if !r.options.PreserveSymlinks {
			if entry, _ := parentInfo.entries.Get(base); entry != nil {
				if symlink := entry.Symlink(r.fs); symlink != "" {
					info.absRealPath = symlink
				} else if parentInfo.absRealPath != "" {
					info.absRealPath = r.fs.Join(parentInfo.absRealPath, base)
				}
			}
		}
	}

	// Record if this directory has a package.json file
	if entry, _ := entries.Get("package.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
		info.packageJSON = r.parsePackageJSON(path)

		// Propagate this browser scope into child directories
		if info.packageJSON != nil && (info.packageJSON.browserPackageMap != nil || info.packageJSON.browserNonPackageMap != nil) {
			info.enclosingBrowserScope = info
		}
	}

	// Record if this directory has a tsconfig.json or jsconfig.json file
	{
		var tsConfigPath string
		if forceTsConfig := r.options.TsConfigOverride; forceTsConfig == "" {
			if entry, _ := entries.Get("tsconfig.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
				tsConfigPath = r.fs.Join(path, "tsconfig.json")
			} else if entry, _ := entries.Get("jsconfig.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
				tsConfigPath = r.fs.Join(path, "jsconfig.json")
			}
		} else if parentInfo == nil {
			// If there is a tsconfig.json override, mount it at the root directory
			tsConfigPath = forceTsConfig
		}
		if tsConfigPath != "" {
			var err error
			info.tsConfigJSON, err = r.parseTSConfig(tsConfigPath, make(map[string]bool))
			if err != nil {
				if err == syscall.ENOENT {
					r.log.AddError(nil, logger.Loc{}, fmt.Sprintf("Cannot find tsconfig file %q",
						r.PrettyPath(logger.Path{Text: tsConfigPath, Namespace: "file"})))
				} else if err != parseErrorAlreadyLogged {
					r.log.AddError(nil, logger.Loc{},
						fmt.Sprintf("Cannot read file %q: %s",
							r.PrettyPath(logger.Path{Text: tsConfigPath, Namespace: "file"}), err.Error()))
				}
			}
		}
	}

	// Propagate the enclosing tsconfig.json from the parent directory
	if info.tsConfigJSON == nil && parentInfo != nil {
		info.tsConfigJSON = parentInfo.tsConfigJSON
	}

	// Look for an "index" file with known extensions
	if absolute, ok, _ := r.loadAsIndex(path, entries); ok {
		info.absPathIndex = &absolute
	}

	return info
}

func (r *resolver) loadAsFile(path string, extensionOrder []string) (string, bool, *fs.DifferentCase) {
	// Read the directory entries once to minimize locking
	dirPath := r.fs.Dir(path)
	entries, err := r.fs.ReadDirectory(dirPath)
	if err != nil {
		if err != syscall.ENOENT {
			r.log.AddError(nil, logger.Loc{},
				fmt.Sprintf("Cannot read directory %q: %s",
					r.PrettyPath(logger.Path{Text: dirPath, Namespace: "file"}), err.Error()))
		}
		return "", false, nil
	}

	base := r.fs.Base(path)

	// Try the plain path without any extensions
	if entry, diffCase := entries.Get(base); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
		return path, true, diffCase
	}

	// Try the path with extensions
	for _, ext := range extensionOrder {
		if entry, diffCase := entries.Get(base + ext); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			return path + ext, true, diffCase
		}
	}

	// TypeScript-specific behavior: if the extension is ".js" or ".jsx", try
	// replacing it with ".ts" or ".tsx". At the time of writing this specific
	// behavior comes from the function "loadModuleFromFile()" in the file
	// "moduleNameResolver.ts" in the TypeScript compiler source code. It
	// contains this comment:
	//
	//   If that didn't work, try stripping a ".js" or ".jsx" extension and
	//   replacing it with a TypeScript one; e.g. "./foo.js" can be matched
	//   by "./foo.ts" or "./foo.d.ts"
	//
	// We don't care about ".d.ts" files because we can't do anything with
	// those, so we ignore that part of the behavior.
	//
	// See the discussion here for more historical context:
	// https://github.com/microsoft/TypeScript/issues/4595
	if strings.HasSuffix(base, ".js") || strings.HasSuffix(base, ".jsx") {
		lastDot := strings.LastIndexByte(base, '.')
		// Note that the official compiler code always tries ".ts" before
		// ".tsx" even if the original extension was ".jsx".
		for _, ext := range []string{".ts", ".tsx"} {
			if entry, diffCase := entries.Get(base[:lastDot] + ext); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
				return path[:len(path)-(len(base)-lastDot)] + ext, true, diffCase
			}
		}
	}

	return "", false, nil
}

// We want to minimize the number of times directory contents are listed. For
// this reason, the directory entries are computed by the caller and then
// passed down to us.
func (r *resolver) loadAsIndex(path string, entries fs.DirEntries) (string, bool, *fs.DifferentCase) {
	// Try the "index" file with extensions
	for _, ext := range r.options.ExtensionOrder {
		base := "index" + ext
		if entry, diffCase := entries.Get(base); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			return r.fs.Join(path, base), true, diffCase
		}
	}

	return "", false, nil
}

func getProperty(json js_ast.Expr, name string) (js_ast.Expr, logger.Loc, bool) {
	if obj, ok := json.Data.(*js_ast.EObject); ok {
		for _, prop := range obj.Properties {
			if key, ok := prop.Key.Data.(*js_ast.EString); ok && key.Value != nil &&
				len(key.Value) == len(name) && js_lexer.UTF16ToString(key.Value) == name {
				return *prop.Value, prop.Key.Loc, true
			}
		}
	}
	return js_ast.Expr{}, logger.Loc{}, false
}

func getString(json js_ast.Expr) (string, bool) {
	if value, ok := json.Data.(*js_ast.EString); ok {
		return js_lexer.UTF16ToString(value.Value), true
	}
	return "", false
}

func getBool(json js_ast.Expr) (bool, bool) {
	if value, ok := json.Data.(*js_ast.EBoolean); ok {
		return value.Value, true
	}
	return false, false
}

func (r *resolver) loadAsFileOrDirectory(path string, kind ast.ImportKind) (PathPair, bool, *fs.DifferentCase) {
	// Use a special import order for CSS "@import" imports
	extensionOrder := r.options.ExtensionOrder
	if kind == ast.ImportAt {
		extensionOrder = r.atImportExtensionOrder
	}

	// Is this a file?
	absolute, ok, diffCase := r.loadAsFile(path, extensionOrder)
	if ok {
		return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true, diffCase
	}

	// Is this a directory?
	dirInfo := r.dirInfoCached(path)
	if dirInfo == nil {
		return PathPair{}, false, nil
	}

	// Try using the main field(s) from "package.json"
	if dirInfo.packageJSON != nil && dirInfo.packageJSON.absMainFields != nil {
		absMainFields := dirInfo.packageJSON.absMainFields
		mainFields := r.options.MainFields
		autoMain := false

		// If the user has not explicitly specified a "main" field order,
		// use a default one determined by the current platform target
		if mainFields == nil {
			mainFields = defaultMainFields[r.options.Platform]
			autoMain = true
		}

		for _, field := range mainFields {
			if absolute, ok := absMainFields[field]; ok {
				// If the user did not manually configure a "main" field order, then
				// use a special per-module automatic algorithm to decide whether to
				// use "module" or "main" based on whether the package is imported
				// using "import" or "require".
				if autoMain && field == "module" {
					absoluteMain, ok := absMainFields["main"]

					// Some packages have a "module" field without a "main" field but
					// still have an implicit "index.js" file. In that case, treat that
					// as the value for "main".
					if !ok && dirInfo.absPathIndex != nil {
						absoluteMain = *dirInfo.absPathIndex
						ok = true
					}

					if ok {
						// If both the "main" and "module" fields exist, use "main" if the
						// path is for "require" and "module" if the path is for "import".
						// If we're using "module", return enough information to be able to
						// fall back to "main" later if something ended up using "require()"
						// with this same path. The goal of this code is to avoid having
						// both the "module" file and the "main" file in the bundle at the
						// same time.
						if kind != ast.ImportRequire {
							return PathPair{
								// This is the whole point of the path pair
								Primary:   logger.Path{Text: absolute, Namespace: "file"},
								Secondary: logger.Path{Text: absoluteMain, Namespace: "file"},
							}, true, nil
						} else {
							return PathPair{Primary: logger.Path{Text: absoluteMain, Namespace: "file"}}, true, nil
						}
					}
				}

				return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true, nil
			}
		}
	}

	// Return the "index.js" file
	if dirInfo.absPathIndex != nil {
		return PathPair{Primary: logger.Path{Text: *dirInfo.absPathIndex, Namespace: "file"}}, true, nil
	}

	return PathPair{}, false, nil
}

// This closely follows the behavior of "tryLoadModuleUsingPaths()" in the
// official TypeScript compiler
func (r *resolver) matchTSConfigPaths(tsConfigJSON *TSConfigJSON, path string, kind ast.ImportKind) (PathPair, bool, *fs.DifferentCase) {
	absBaseURL := tsConfigJSON.BaseURLForPaths

	// The explicit base URL should take precedence over the implicit base URL
	// if present. This matters when a tsconfig.json file overrides "baseUrl"
	// from another extended tsconfig.json file but doesn't override "paths".
	if tsConfigJSON.BaseURL != nil {
		absBaseURL = *tsConfigJSON.BaseURL
	}

	// Check for exact matches first
	for key, originalPaths := range tsConfigJSON.Paths {
		if key == path {
			for _, originalPath := range originalPaths {
				// Load the original path relative to the "baseUrl" from tsconfig.json
				absoluteOriginalPath := originalPath
				if !r.fs.IsAbs(originalPath) {
					absoluteOriginalPath = r.fs.Join(absBaseURL, originalPath)
				}
				if absolute, ok, diffCase := r.loadAsFileOrDirectory(absoluteOriginalPath, kind); ok {
					return absolute, true, diffCase
				}
			}
			return PathPair{}, false, nil
		}
	}

	type match struct {
		prefix        string
		suffix        string
		originalPaths []string
	}

	// Check for pattern matches next
	longestMatchPrefixLength := -1
	longestMatchSuffixLength := -1
	var longestMatch match
	for key, originalPaths := range tsConfigJSON.Paths {
		if starIndex := strings.IndexByte(key, '*'); starIndex != -1 {
			prefix, suffix := key[:starIndex], key[starIndex+1:]

			// Find the match with the longest prefix. If two matches have the same
			// prefix length, pick the one with the longest suffix. This second edge
			// case isn't handled by the TypeScript compiler, but we handle it
			// because we want the output to always be deterministic and Go map
			// iteration order is deliberately non-deterministic.
			if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) && (len(prefix) > longestMatchPrefixLength ||
				(len(prefix) == longestMatchPrefixLength && len(suffix) > longestMatchSuffixLength)) {
				longestMatchPrefixLength = len(prefix)
				longestMatchSuffixLength = len(suffix)
				longestMatch = match{
					prefix:        prefix,
					suffix:        suffix,
					originalPaths: originalPaths,
				}
			}
		}
	}

	// If there is at least one match, only consider the one with the longest
	// prefix. This matches the behavior of the TypeScript compiler.
	if longestMatchPrefixLength != -1 {
		for _, originalPath := range longestMatch.originalPaths {
			// Swap out the "*" in the original path for whatever the "*" matched
			matchedText := path[len(longestMatch.prefix) : len(path)-len(longestMatch.suffix)]
			originalPath = strings.Replace(originalPath, "*", matchedText, 1)

			// Load the original path relative to the "baseUrl" from tsconfig.json
			absoluteOriginalPath := originalPath
			if !r.fs.IsAbs(originalPath) {
				absoluteOriginalPath = r.fs.Join(absBaseURL, originalPath)
			}
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absoluteOriginalPath, kind); ok {
				return absolute, true, diffCase
			}
		}
	}

	return PathPair{}, false, nil
}

func (r *resolver) loadNodeModules(path string, kind ast.ImportKind, dirInfo *dirInfo) (PathPair, bool, *fs.DifferentCase, []logger.MsgData) {
	// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
	if dirInfo.tsConfigJSON != nil {
		// Try path substitutions first
		if dirInfo.tsConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(dirInfo.tsConfigJSON, path, kind); ok {
				return absolute, true, diffCase, nil
			}
		}

		// Try looking up the path relative to the base URL
		if dirInfo.tsConfigJSON.BaseURL != nil {
			basePath := r.fs.Join(*dirInfo.tsConfigJSON.BaseURL, path)
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(basePath, kind); ok {
				return absolute, true, diffCase, nil
			}
		}
	}

	// Then check the global "NODE_PATH" environment variable
	for _, absDir := range r.options.AbsNodePaths {
		absPath := r.fs.Join(absDir, path)
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
			return absolute, true, diffCase, nil
		}
	}

	esmPackageName, esmPackageSubpath, esmOK := esmParsePackageName(path)

	// Then check for the package in any enclosing "node_modules" directories
	for {
		// Skip directories that are themselves called "node_modules", since we
		// don't ever want to search for "node_modules/node_modules"
		if dirInfo.hasNodeModules {
			absPath := r.fs.Join(dirInfo.absPath, "node_modules", path)

			// Check for an "exports" map in the package's package.json folder
			if esmOK {
				absPkgPath := r.fs.Join(dirInfo.absPath, "node_modules", esmPackageName)
				if pkgDirInfo := r.dirInfoCached(absPkgPath); pkgDirInfo != nil {
					if pkgJSON := pkgDirInfo.packageJSON; pkgJSON != nil && pkgJSON.exportsMap != nil {
						// The condition set is determined by the kind of import
						conditions := r.esmConditionsDefault
						switch kind {
						case ast.ImportStmt, ast.ImportDynamic:
							conditions = r.esmConditionsImport
						case ast.ImportRequire, ast.ImportRequireResolve:
							conditions = r.esmConditionsRequire
						}

						// Resolve against the path "/", then join it with the absolute
						// directory path. This is done because ESM package resolution uses
						// URLs while our path resolution uses file system paths. We don't
						// want problems due to Windows paths, which are very unlike URL
						// paths. We also want to avoid any "%" characters in the absolute
						// directory path accidentally being interpreted as URL escapes.
						resolvedPath, status, token := esmPackageExportsResolveWithPostConditions("/", esmPackageSubpath, pkgJSON.exportsMap.root, conditions)
						if (status == peStatusExact || status == peStatusInexact) && strings.HasPrefix(resolvedPath, "/") {
							absResolvedPath := r.fs.Join(absPkgPath, resolvedPath[1:])

							switch status {
							case peStatusExact:
								resolvedDirInfo := r.dirInfoCached(r.fs.Dir(absResolvedPath))
								if resolvedDirInfo == nil {
									status = peStatusModuleNotFound
								} else if entry, diffCase := resolvedDirInfo.entries.Get(r.fs.Base(absResolvedPath)); entry == nil {
									status = peStatusModuleNotFound
								} else if kind := entry.Kind(r.fs); kind == fs.DirEntry {
									status = peStatusUnsupportedDirectoryImport
								} else if kind != fs.FileEntry {
									status = peStatusModuleNotFound
								} else {
									return PathPair{Primary: logger.Path{Text: absResolvedPath, Namespace: "file"}}, true, diffCase, nil
								}

							case peStatusInexact:
								// If this was resolved against an expansion key ending in a "/"
								// instead of a "*", we need to try CommonJS-style implicit
								// extension and/or directory detection.
								if absolute, ok, diffCase := r.loadAsFileOrDirectory(absResolvedPath, kind); ok {
									return absolute, true, diffCase, nil
								}
								status = peStatusModuleNotFound
							}
						}

						var notes []logger.MsgData
						if strings.HasPrefix(resolvedPath, "/") {
							resolvedPath = "." + resolvedPath
						}

						// Provide additional details about the failure to help with debugging
						switch status {
						case peStatusInvalidModuleSpecifier:
							notes = []logger.MsgData{logger.RangeData(&pkgJSON.source, token,
								fmt.Sprintf("The module specifier %q is invalid", resolvedPath))}

						case peStatusInvalidPackageConfiguration:
							notes = []logger.MsgData{logger.RangeData(&pkgJSON.source, token,
								"The package configuration has an invalid value here")}

						case peStatusInvalidPackageTarget:
							why := fmt.Sprintf("The package target %q is invalid", resolvedPath)
							if resolvedPath == "" {
								// "PACKAGE_TARGET_RESOLVE" is specified to throw an "Invalid
								// Package Target" error for what is actually an invalid package
								// configuration error
								why = "The package configuration has an invalid value here"
							}
							notes = []logger.MsgData{logger.RangeData(&pkgJSON.source, token, why)}

						case peStatusPackagePathNotExported:
							notes = []logger.MsgData{logger.RangeData(&pkgJSON.source, token,
								fmt.Sprintf("The path %q is not exported by %q", esmPackageSubpath, esmPackageName))}

						case peStatusModuleNotFound:
							notes = []logger.MsgData{logger.RangeData(&pkgJSON.source, token,
								fmt.Sprintf("The module %q was not found on the file system", resolvedPath))}

						case peStatusUnsupportedDirectoryImport:
							notes = []logger.MsgData{logger.RangeData(&pkgJSON.source, token,
								fmt.Sprintf("Importing the directory %q is not supported", resolvedPath))}
						}

						return PathPair{}, false, nil, notes
					}
				}
			}

			// Check the non-package "browser" map for the first time (1 out of 2)
			importDirInfo := r.dirInfoCached(r.fs.Dir(absPath))
			if importDirInfo != nil && importDirInfo.enclosingBrowserScope != nil {
				if packageJSON := importDirInfo.enclosingBrowserScope.packageJSON; packageJSON.browserNonPackageMap != nil {
					if remapped, ok := packageJSON.browserNonPackageMap[absPath]; ok {
						if remapped == nil {
							return PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil, nil
						} else if remappedResult, ok, diffCase, notes := r.resolveWithoutRemapping(importDirInfo.enclosingBrowserScope, *remapped, kind); ok {
							return remappedResult, true, diffCase, notes
						}
					}
				}
			}

			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
				return absolute, true, diffCase, nil
			}
		}

		// Go to the parent directory, stopping at the file system root
		dirInfo = dirInfo.parent
		if dirInfo == nil {
			break
		}
	}

	return PathPair{}, false, nil, nil
}

// Package paths are loaded from a "node_modules" directory. Non-package paths
// are relative or absolute paths.
func IsPackagePath(path string) bool {
	return !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "./") &&
		!strings.HasPrefix(path, "../") && path != "." && path != ".."
}

var BuiltInNodeModules = map[string]bool{
	"assert":         true,
	"async_hooks":    true,
	"buffer":         true,
	"child_process":  true,
	"cluster":        true,
	"console":        true,
	"constants":      true,
	"crypto":         true,
	"dgram":          true,
	"dns":            true,
	"domain":         true,
	"events":         true,
	"fs":             true,
	"http":           true,
	"http2":          true,
	"https":          true,
	"inspector":      true,
	"module":         true,
	"net":            true,
	"os":             true,
	"path":           true,
	"perf_hooks":     true,
	"process":        true,
	"punycode":       true,
	"querystring":    true,
	"readline":       true,
	"repl":           true,
	"stream":         true,
	"string_decoder": true,
	"sys":            true,
	"timers":         true,
	"tls":            true,
	"trace_events":   true,
	"tty":            true,
	"url":            true,
	"util":           true,
	"v8":             true,
	"vm":             true,
	"worker_threads": true,
	"zlib":           true,
}
