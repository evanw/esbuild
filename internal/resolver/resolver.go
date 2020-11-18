package resolver

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

// This namespace is used when a module has been disabled by being mapped to
// "false" using the "browser" field of "package.json".
const BrowserFalseNamespace = "empty"

var defaultMainFields = map[config.Platform][]string{
	// Note that this means if a package specifies "main", "module", and
	// "browser" then "browser" will win out over "module". This is the
	// same behavior as webpack: https://github.com/webpack/webpack/issues/4674.
	//
	// This is deliberate because the presence of the "browser" field is a
	// good signal that the "module" field may have non-browser stuff in it,
	// which will crash or fail to be bundled when targeting the browser.
	config.PlatformBrowser: []string{"browser", "module", "main"},

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
	config.PlatformNode: []string{"main", "module"},
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

	// If not empty, these should override the default values
	JSXFactory  []string // Default if empty: "React.createElement"
	JSXFragment []string // Default if empty: "React.Fragment"

	IsExternal bool

	// If true, any ES6 imports to this file can be considered to have no side
	// effects. This means they should be removed if unused.
	IgnorePrimaryIfUnused *IgnoreIfUnusedData

	// If true, the class field transform should use Object.defineProperty().
	UseDefineForClassFieldsTS bool

	// If true, unused imports are retained in TypeScript code. This matches the
	// behavior of the "importsNotUsedAsValues" field in "tsconfig.json" when the
	// value is not "remove".
	PreserveUnusedImportsTS bool

	// This is true if the file is inside a "node_modules" directory
	SuppressWarningsAboutWeirdCode bool
}

type Resolver interface {
	Resolve(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult
	ResolveAbs(absPath string) *ResolveResult
	PrettyPath(path logger.Path) string
}

type resolver struct {
	fs      fs.FS
	log     logger.Log
	options config.Options
	mutex   sync.Mutex

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

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCache map[string]*dirInfo
}

func NewResolver(fs fs.FS, log logger.Log, options config.Options) Resolver {
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

	return &resolver{
		fs:                     fs,
		log:                    log,
		options:                options,
		dirCache:               make(map[string]*dirInfo),
		atImportExtensionOrder: atImportExtensionOrder,
	}
}

func (r *resolver) Resolve(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult {
	// Certain types of URLs default to being external for convenience
	if
	// "fill: url(#filter);"
	(kind.IsFromCSS() && strings.HasPrefix(importPath, "#")) ||

		// "background: url(http://example.com/images/image.png);"
		strings.HasPrefix(importPath, "http://") ||

		// "background: url(https://example.com/images/image.png);"
		strings.HasPrefix(importPath, "https://") ||

		// "background: url(data:image/png;base64,iVBORw0KGgo=);"
		strings.HasPrefix(importPath, "data:") ||

		// "background: url(//example.com/images/image.png);"
		strings.HasPrefix(importPath, "//") {

		return &ResolveResult{
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
		}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	result := r.resolveWithoutSymlinks(sourceDir, importPath, kind)
	if result == nil {
		return nil
	}

	// If successful, resolve symlinks using the directory info cache
	return r.finalizeResolve(*result)
}

func (r *resolver) ResolveAbs(absPath string) *ResolveResult {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Just decorate the absolute path with information from parent directories
	return r.finalizeResolve(ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}})
}

func isInsideNodeModules(fs fs.FS, path string) bool {
	dir := fs.Dir(path)
	for {
		if fs.Base(dir) == "node_modules" {
			return true
		}
		parent := fs.Dir(dir)
		if dir == parent {
			return false
		}
		dir = parent
	}
}

func (r *resolver) finalizeResolve(result ResolveResult) *ResolveResult {
	for _, path := range result.PathPair.iter() {
		if path.Namespace == "file" {
			if dirInfo := r.dirInfoCached(r.fs.Dir(path.Text)); dirInfo != nil {
				base := r.fs.Base(path.Text)

				// Don't emit warnings for code inside a "node_modules" directory
				if isInsideNodeModules(r.fs, path.Text) {
					result.SuppressWarningsAboutWeirdCode = true
				}

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

				if entry, ok := dirInfo.entries[base]; ok {
					if symlink := entry.Symlink(); symlink != "" {
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

	return &result
}

func (r *resolver) resolveWithoutSymlinks(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together
	var result PathPair

	// Return early if this is already an absolute path
	if r.fs.IsAbs(importPath) {
		// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
		if dirInfo := r.dirInfoCached(sourceDir); dirInfo != nil && dirInfo.tsConfigJSON != nil &&
			dirInfo.tsConfigJSON.BaseURL != nil && dirInfo.tsConfigJSON.Paths != nil {
			if absolute, ok := r.matchTSConfigPaths(dirInfo.tsConfigJSON, importPath, kind); ok {
				return &ResolveResult{PathPair: absolute}
			}
		}

		if r.options.ExternalModules.AbsPaths != nil && r.options.ExternalModules.AbsPaths[importPath] {
			// If the string literal in the source text is an absolute path and has
			// been marked as an external module, mark it as *not* an absolute path.
			// That way we preserve the literal text in the output and don't generate
			// a relative path from the output directory to that path.
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}
		}

		return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Namespace: "file"}}}
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
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}, IsExternal: true}
		}

		if absolute, ok := r.loadAsFileOrDirectory(absPath, kind); ok {
			checkPackage = false
			result = absolute
		} else if !checkPackage {
			return nil
		}
	}

	if checkPackage {
		// Check for external packages first
		if r.options.ExternalModules.NodeModules != nil {
			query := importPath
			for {
				if r.options.ExternalModules.NodeModules[query] {
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}
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
			return nil
		}

		// Support remapping one package path to another via the "browser" field
		if sourceDirInfo.enclosingBrowserScope != nil {
			packageJSON := sourceDirInfo.enclosingBrowserScope.packageJSON
			if packageJSON.browserPackageMap != nil {
				if remapped, ok := packageJSON.browserPackageMap[importPath]; ok {
					if remapped == nil {
						// "browser": {"module": false}
						if absolute, ok := r.loadNodeModules(importPath, kind, sourceDirInfo); ok {
							absolute.Primary = logger.Path{Text: absolute.Primary.Text, Namespace: BrowserFalseNamespace}
							if absolute.HasSecondary() {
								absolute.Secondary = logger.Path{Text: absolute.Secondary.Text, Namespace: BrowserFalseNamespace}
							}
							return &ResolveResult{PathPair: absolute}
						} else {
							return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Namespace: BrowserFalseNamespace}}}
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

		if absolute, ok := r.resolveWithoutRemapping(sourceDirInfo, importPath, kind); ok {
			result = absolute
		} else {
			// Note: node's "self references" are not currently supported
			return nil
		}
	}

	// Check the directory that contains this file
	for _, path := range result.iter() {
		resultDir := r.fs.Dir(path.Text)
		resultDirInfo := r.dirInfoCached(resultDir)

		// Support remapping one non-module path to another via the "browser" field
		if resultDirInfo != nil && resultDirInfo.enclosingBrowserScope != nil {
			packageJSON := resultDirInfo.enclosingBrowserScope.packageJSON
			if packageJSON.browserNonPackageMap != nil {
				if remapped, ok := packageJSON.browserNonPackageMap[path.Text]; ok {
					if remapped == nil {
						path.Namespace = BrowserFalseNamespace
					} else if remappedResult, ok := r.resolveWithoutRemapping(resultDirInfo.enclosingBrowserScope, *remapped, kind); ok {
						*path = remappedResult.Primary
					} else {
						return nil
					}
				}
			}
		}
	}

	return &ResolveResult{PathPair: result}
}

func (r *resolver) resolveWithoutRemapping(sourceDirInfo *dirInfo, importPath string, kind ast.ImportKind) (PathPair, bool) {
	if IsPackagePath(importPath) {
		return r.loadNodeModules(importPath, kind, sourceDirInfo)
	} else {
		return r.loadAsFileOrDirectory(r.fs.Join(sourceDirInfo.absPath, importPath), kind)
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
		return strings.ReplaceAll(path.Text, "\\", "/")
	}

	if path.Namespace != "" {
		return fmt.Sprintf("%s:%s", path.Namespace, path.Text)
	}

	return path.Text
}

////////////////////////////////////////////////////////////////////////////////

type packageJSON struct {
	absMainFields map[string]string

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
	browserNonPackageMap map[string]*string
	browserPackageMap    map[string]*string

	// If this is non-nil, each entry in this map is the absolute path of a file
	// with side effects. Any entry not in this map should be considered to have
	// no side effects, which means import statements for these files can be
	// removed if none of the imports are used. This is a convention from Webpack:
	// https://webpack.js.org/guides/tree-shaking/.
	//
	// Note that if a file is included, all statements that can't be proven to be
	// free of side effects must be included. This convention does not say
	// anything about whether any statements within the file have side effects or
	// not.
	sideEffectsMap     map[string]bool
	sideEffectsRegexps []*regexp.Regexp
	ignoreIfUnusedData *IgnoreIfUnusedData
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
	entries        map[string]*fs.Entry
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

	contents, err := r.fs.ReadFile(file)
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

	result := ParseTSConfigJSON(r.log, source, func(extends string, extendsRange logger.Range) *TSConfigJSON {
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
			extendsFile := r.fs.Join(fileDir, extends)
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
		if !isInsideNodeModules(r.fs, file) {
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
	if err != nil {
		if err != syscall.ENOENT {
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
		if entry, ok := entries["node_modules"]; ok {
			info.hasNodeModules = entry.Kind() == fs.DirEntry
		}
	}

	// Propagate the browser scope into child directories
	if parentInfo != nil {
		info.enclosingBrowserScope = parentInfo.enclosingBrowserScope

		// Make sure "absRealPath" is the real path of the directory (resolving any symlinks)
		if entry, ok := parentInfo.entries[base]; ok {
			if symlink := entry.Symlink(); symlink != "" {
				info.absRealPath = symlink
			} else if parentInfo.absRealPath != "" {
				info.absRealPath = r.fs.Join(parentInfo.absRealPath, base)
			}
		}
	}

	// Record if this directory has a package.json file
	if entry, ok := entries["package.json"]; ok && entry.Kind() == fs.FileEntry {
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
			if entry, ok := entries["tsconfig.json"]; ok && entry.Kind() == fs.FileEntry {
				tsConfigPath = r.fs.Join(path, "tsconfig.json")
			} else if entry, ok := entries["jsconfig.json"]; ok && entry.Kind() == fs.FileEntry {
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
	if absolute, ok := r.loadAsIndex(path, entries); ok {
		info.absPathIndex = &absolute
	}

	return info
}

func (r *resolver) parsePackageJSON(path string) *packageJSON {
	packageJsonPath := r.fs.Join(path, "package.json")
	contents, err := r.fs.ReadFile(packageJsonPath)
	if err != nil {
		r.log.AddError(nil, logger.Loc{},
			fmt.Sprintf("Cannot read file %q: %s",
				r.PrettyPath(logger.Path{Text: packageJsonPath, Namespace: "file"}), err.Error()))
		return nil
	}

	keyPath := logger.Path{Text: packageJsonPath, Namespace: "file"}
	jsonSource := logger.Source{
		KeyPath:    keyPath,
		PrettyPath: r.PrettyPath(keyPath),
		Contents:   contents,
	}

	json, ok := js_parser.ParseJSON(r.log, jsonSource, js_parser.ParseJSONOptions{})
	if !ok {
		return nil
	}

	toAbsPath := func(pathText string, pathRange logger.Range) *string {
		// Is it a file?
		if absolute, ok := r.loadAsFile(pathText, r.options.ExtensionOrder); ok {
			return &absolute
		}

		// Is it a directory?
		if mainEntries, err := r.fs.ReadDirectory(pathText); err == nil {
			// Look for an "index" file with known extensions
			if absolute, ok := r.loadAsIndex(pathText, mainEntries); ok {
				return &absolute
			}
		} else if err != syscall.ENOENT {
			r.log.AddRangeError(&jsonSource, pathRange,
				fmt.Sprintf("Cannot read directory %q: %s",
					r.PrettyPath(logger.Path{Text: pathText, Namespace: "file"}), err.Error()))
		}
		return nil
	}

	packageJSON := &packageJSON{}

	// Read the "main" fields
	mainFields := r.options.MainFields
	if mainFields == nil {
		mainFields = defaultMainFields[r.options.Platform]
	}
	for _, field := range mainFields {
		if mainJson, _, ok := getProperty(json, field); ok {
			if main, ok := getString(mainJson); ok {
				if packageJSON.absMainFields == nil {
					packageJSON.absMainFields = make(map[string]string)
				}
				if absPath := toAbsPath(r.fs.Join(path, main), jsonSource.RangeOfString(mainJson.Loc)); absPath != nil {
					packageJSON.absMainFields[field] = *absPath
				}
			}
		}
	}

	// Read the "browser" property, but only when targeting the browser
	if browserJson, _, ok := getProperty(json, "browser"); ok && r.options.Platform == config.PlatformBrowser {
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
		if browser, ok := browserJson.Data.(*js_ast.EObject); ok {
			// The value is an object
			browserPackageMap := make(map[string]*string)
			browserNonPackageMap := make(map[string]*string)

			// Remap all files in the browser field
			for _, prop := range browser.Properties {
				if key, ok := getString(prop.Key); ok && prop.Value != nil {
					isPackagePath := IsPackagePath(key)

					// Make this an absolute path if it's not a package
					if !isPackagePath {
						key = r.fs.Join(path, key)
					}

					if value, ok := getString(*prop.Value); ok {
						// If this is a string, it's a replacement package
						if isPackagePath {
							browserPackageMap[key] = &value
						} else {
							browserNonPackageMap[key] = &value
						}
					} else if value, ok := getBool(*prop.Value); ok && !value {
						// If this is false, it means the package is disabled
						if isPackagePath {
							browserPackageMap[key] = nil
						} else {
							browserNonPackageMap[key] = nil
						}
					}
				}
			}

			packageJSON.browserPackageMap = browserPackageMap
			packageJSON.browserNonPackageMap = browserNonPackageMap
		}
	}

	// Read the "sideEffects" property
	if sideEffectsJson, sideEffectsLoc, ok := getProperty(json, "sideEffects"); ok {
		switch data := sideEffectsJson.Data.(type) {
		case *js_ast.EBoolean:
			if !data.Value {
				// Make an empty map for "sideEffects: false", which indicates all
				// files in this module can be considered to not have side effects.
				packageJSON.sideEffectsMap = make(map[string]bool)
				packageJSON.ignoreIfUnusedData = &IgnoreIfUnusedData{
					IsSideEffectsArrayInJSON: false,
					Source:                   &jsonSource,
					Range:                    jsonSource.RangeOfString(sideEffectsLoc),
				}
			}

		case *js_ast.EArray:
			// The "sideEffects: []" format means all files in this module but not in
			// the array can be considered to not have side effects.
			packageJSON.sideEffectsMap = make(map[string]bool)
			packageJSON.ignoreIfUnusedData = &IgnoreIfUnusedData{
				IsSideEffectsArrayInJSON: true,
				Source:                   &jsonSource,
				Range:                    jsonSource.RangeOfString(sideEffectsLoc),
			}
			for _, itemJson := range data.Items {
				item, ok := itemJson.Data.(*js_ast.EString)
				if !ok || item.Value == nil {
					r.log.AddWarning(&jsonSource, itemJson.Loc,
						"Expected string in array for \"sideEffects\"")
					continue
				}

				absPattern := r.fs.Join(path, js_lexer.UTF16ToString(item.Value))
				re, hadWildcard := globToEscapedRegexp(absPattern)

				// Wildcard patterns require more expensive matching
				if hadWildcard {
					packageJSON.sideEffectsRegexps = append(packageJSON.sideEffectsRegexps, regexp.MustCompile(re))
					continue
				}

				// Normal strings can be matched with a map lookup
				packageJSON.sideEffectsMap[absPattern] = true
			}

		default:
			r.log.AddWarning(&jsonSource, sideEffectsJson.Loc,
				"The value for \"sideEffects\" must be a boolean or an array")
		}
	}

	return packageJSON
}

func globToEscapedRegexp(glob string) (string, bool) {
	sb := strings.Builder{}
	sb.WriteByte('^')
	hadWildcard := false

	for _, c := range glob {
		switch c {
		case '\\', '^', '$', '.', '+', '|', '(', ')', '[', ']', '{', '}':
			sb.WriteByte('\\')
			sb.WriteRune(c)

		case '*':
			sb.WriteString(".*")
			hadWildcard = true

		case '?':
			sb.WriteByte('.')
			hadWildcard = true

		default:
			sb.WriteRune(c)
		}
	}

	sb.WriteByte('$')
	return sb.String(), hadWildcard
}

func (r *resolver) loadAsFile(path string, extensionOrder []string) (string, bool) {
	// Read the directory entries once to minimize locking
	dirPath := r.fs.Dir(path)
	entries, err := r.fs.ReadDirectory(dirPath)
	if err != nil {
		if err != syscall.ENOENT {
			r.log.AddError(nil, logger.Loc{},
				fmt.Sprintf("Cannot read directory %q: %s",
					r.PrettyPath(logger.Path{Text: dirPath, Namespace: "file"}), err.Error()))
		}
		return "", false
	}

	base := r.fs.Base(path)

	// Try the plain path without any extensions
	if entry, ok := entries[base]; ok && entry.Kind() == fs.FileEntry {
		return path, true
	}

	// Try the path with extensions
	for _, ext := range extensionOrder {
		if entry, ok := entries[base+ext]; ok && entry.Kind() == fs.FileEntry {
			return path + ext, true
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
			if entry, ok := entries[base[:lastDot]+ext]; ok && entry.Kind() == fs.FileEntry {
				return path[:len(path)-(len(base)-lastDot)] + ext, true
			}
		}
	}

	return "", false
}

// We want to minimize the number of times directory contents are listed. For
// this reason, the directory entries are computed by the caller and then
// passed down to us.
func (r *resolver) loadAsIndex(path string, entries map[string]*fs.Entry) (string, bool) {
	// Try the "index" file with extensions
	for _, ext := range r.options.ExtensionOrder {
		base := "index" + ext
		if entry, ok := entries[base]; ok && entry.Kind() == fs.FileEntry {
			return r.fs.Join(path, base), true
		}
	}

	return "", false
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

func (r *resolver) loadAsFileOrDirectory(path string, kind ast.ImportKind) (PathPair, bool) {
	// Use a special import order for CSS "@import" imports
	extensionOrder := r.options.ExtensionOrder
	if kind == ast.ImportAt {
		extensionOrder = r.atImportExtensionOrder
	}

	// Is this a file?
	absolute, ok := r.loadAsFile(path, extensionOrder)
	if ok {
		return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true
	}

	// Is this a directory?
	dirInfo := r.dirInfoCached(path)
	if dirInfo == nil {
		return PathPair{}, false
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
					if absoluteMain, ok := absMainFields["main"]; ok {
						// If both the "main" and "module" fields exist, use "main" if the path is
						// for "require" and "module" if the path is for "import". If we're using
						// "module", return enough information to be able to fall back to "main"
						// later if that decision was incorrect.
						if kind != ast.ImportRequire {
							return PathPair{
								// This is the whole point of the path pair
								Primary:   logger.Path{Text: absolute, Namespace: "file"},
								Secondary: logger.Path{Text: absoluteMain, Namespace: "file"},
							}, true
						} else {
							return PathPair{Primary: logger.Path{Text: absoluteMain, Namespace: "file"}}, true
						}
					}
				}

				return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true
			}
		}
	}

	// Return the "index.js" file
	if dirInfo.absPathIndex != nil {
		return PathPair{Primary: logger.Path{Text: *dirInfo.absPathIndex, Namespace: "file"}}, true
	}

	return PathPair{}, false
}

// This closely follows the behavior of "tryLoadModuleUsingPaths()" in the
// official TypeScript compiler
func (r *resolver) matchTSConfigPaths(tsConfigJSON *TSConfigJSON, path string, kind ast.ImportKind) (PathPair, bool) {
	// Check for exact matches first
	for key, originalPaths := range tsConfigJSON.Paths {
		if key == path {
			for _, originalPath := range originalPaths {
				// Load the original path relative to the "baseUrl" from tsconfig.json
				absoluteOriginalPath := r.fs.Join(*tsConfigJSON.BaseURL, originalPath)
				if absolute, ok := r.loadAsFileOrDirectory(absoluteOriginalPath, kind); ok {
					return absolute, true
				}
			}
			return PathPair{}, false
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
			absoluteOriginalPath := r.fs.Join(*tsConfigJSON.BaseURL, originalPath)
			if absolute, ok := r.loadAsFileOrDirectory(absoluteOriginalPath, kind); ok {
				return absolute, true
			}
		}
	}

	return PathPair{}, false
}

func (r *resolver) loadNodeModules(path string, kind ast.ImportKind, dirInfo *dirInfo) (PathPair, bool) {
	// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
	if dirInfo.tsConfigJSON != nil && dirInfo.tsConfigJSON.BaseURL != nil {
		// Try path substitutions first
		if dirInfo.tsConfigJSON.Paths != nil {
			if absolute, ok := r.matchTSConfigPaths(dirInfo.tsConfigJSON, path, kind); ok {
				return absolute, true
			}
		}

		// Try looking up the path relative to the base URL
		basePath := r.fs.Join(*dirInfo.tsConfigJSON.BaseURL, path)
		if absolute, ok := r.loadAsFileOrDirectory(basePath, kind); ok {
			return absolute, true
		}
	}

	// Then check for the package in any enclosing "node_modules" directories
	for {
		// Skip directories that are themselves called "node_modules", since we
		// don't ever want to search for "node_modules/node_modules"
		if dirInfo.hasNodeModules {
			absolute, ok := r.loadAsFileOrDirectory(r.fs.Join(dirInfo.absPath, "node_modules", path), kind)
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

	return PathPair{}, false
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
