package resolver

import (
	"errors"
	"fmt"
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

type ResolveResult struct {
	PathPair   PathPair
	IsExternal bool

	// If not empty, these should override the default values
	JSXFactory  []string // Default if empty: "React.createElement"
	JSXFragment []string // Default if empty: "React.Fragment"

	// If true, any ES6 imports to this file can be considered to have no side
	// effects. This means they should be removed if unused.
	IgnoreIfUnused bool

	// If true, the class field transform should use Object.defineProperty().
	StrictClassFields bool

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

	return &resolver{
		fs:       fs,
		log:      log,
		options:  options,
		dirCache: make(map[string]*dirInfo),
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
				// directory with a "package.json" file
				for info := dirInfo; info != nil; info = info.parent {
					if info.packageJson != nil {
						if info.packageJson.sideEffectsMap != nil {
							result.IgnoreIfUnused = !info.packageJson.sideEffectsMap[path.Text]
						}
						break
					}
				}

				// Copy various fields from the nearest enclosing "tsconfig.json" file if present
				if path == &result.PathPair.Primary && dirInfo.tsConfigJson != nil {
					result.JSXFactory = dirInfo.tsConfigJson.jsxFactory
					result.JSXFragment = dirInfo.tsConfigJson.jsxFragmentFactory
					result.StrictClassFields = dirInfo.tsConfigJson.useDefineForClassFields
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
		if r.options.ExternalModules.AbsPaths != nil && r.options.ExternalModules.AbsPaths[importPath] {
			// If the string literal in the source text is an absolute path and has
			// been marked as an external module, mark it as *not* an absolute path.
			// That way we preserve the literal text in the output and don't generate
			// a relative path from the output directory to that path.
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}
		}

		return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Namespace: "file"}}}
	}

	if !IsPackagePath(importPath) {
		absPath := r.fs.Join(sourceDir, importPath)

		// Check for external packages first
		if r.options.ExternalModules.AbsPaths != nil && r.options.ExternalModules.AbsPaths[absPath] {
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}, IsExternal: true}
		}

		if absolute, ok := r.loadAsFileOrDirectory(absPath, kind); ok {
			result = absolute
		} else {
			return nil
		}
	} else {
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
			packageJson := sourceDirInfo.enclosingBrowserScope.packageJson
			if packageJson.browserPackageMap != nil {
				if remapped, ok := packageJson.browserPackageMap[importPath]; ok {
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
			packageJson := resultDirInfo.enclosingBrowserScope.packageJson
			if packageJson.browserNonPackageMap != nil {
				if remapped, ok := packageJson.browserNonPackageMap[path.Text]; ok {
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

type packageJson struct {
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
	sideEffectsMap map[string]bool
}

type tsConfigJson struct {
	// The absolute path of "compilerOptions.baseUrl"
	absPathBaseUrl *string

	// The verbatim values of "compilerOptions.paths". The keys are patterns to
	// match and the values are arrays of fallback paths to search. Each key and
	// each fallback path can optionally have a single "*" wildcard character.
	// If both the key and the value have a wildcard, the substring matched by
	// the wildcard is substituted into the fallback path. The keys represent
	// module-style path names and the fallback paths are relative to the
	// "baseUrl" value in the "tsconfig.json" file.
	paths map[string][]string

	jsxFactory              []string
	jsxFragmentFactory      []string
	useDefineForClassFields bool
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
	packageJson    *packageJson  // Is there a "package.json" file?
	tsConfigJson   *tsConfigJson // Is there a "tsconfig.json" file in this directory or a parent directory?
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

func (r *resolver) parseMemberExpressionForJSX(source logger.Source, loc logger.Loc, text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !js_lexer.IsIdentifier(part) {
			warnRange := source.RangeOfString(loc)
			r.log.AddRangeWarning(&source, warnRange, fmt.Sprintf("Invalid JSX member expression: %q", text))
			return nil
		}
	}
	return parts
}

var parseErrorImportCycle = errors.New("(import cycle)")

// This may return "parseErrorAlreadyLogged" in which case there was a syntax
// error, but it's already been reported. No further errors should be logged.
//
// Nested calls may also return "parseErrorImportCycle". In that case the
// caller is responsible for logging an appropriate error message.
func (r *resolver) parseJsTsConfig(file string, visited map[string]bool) (*tsConfigJson, error) {
	// Don't infinite loop if a series of "extends" links forms a cycle
	if visited[file] {
		return nil, parseErrorImportCycle
	}
	visited[file] = true
	filePath := r.fs.Dir(file)

	// Unfortunately "tsconfig.json" isn't actually JSON. It's some other
	// format that appears to be defined by the implementation details of the
	// TypeScript compiler.
	//
	// Attempt to parse it anyway by modifying the JSON parser, but just for
	// these particular files. This is likely not a completely accurate
	// emulation of what the TypeScript compiler does (e.g. string escape
	// behavior may also be different).
	json, tsConfigSource, err := r.parseJSON(file, js_parser.ParseJSONOptions{
		AllowComments:       true, // https://github.com/microsoft/TypeScript/issues/4987
		AllowTrailingCommas: true,
	})
	if err != nil {
		return nil, err
	}

	var result tsConfigJson

	// Parse "extends"
	if extendsJson, _, ok := getProperty(json, "extends"); ok {
		if extends, ok := getString(extendsJson); ok {
			warnRange := tsConfigSource.RangeOfString(extendsJson.Loc)
			found := false

			if IsPackagePath(extends) {
				// If this is a package path, try to resolve it to a "node_modules"
				// folder. This doesn't use the normal node module resolution algorithm
				// both because it's different (e.g. we don't want to match a directory)
				// and because it would deadlock since we're currently in the middle of
				// populating the directory info cache.
				current := filePath
				for !found {
					// Skip "node_modules" folders
					if r.fs.Base(current) != "node_modules" {
						join := r.fs.Join(current, "node_modules", extends)
						filesToCheck := []string{r.fs.Join(join, "tsconfig.json"), join, join + ".json"}
						for _, fileToCheck := range filesToCheck {
							base, err := r.parseJsTsConfig(fileToCheck, visited)
							if err == nil {
								result = *base
							} else if err == syscall.ENOENT {
								continue
							} else if err == parseErrorImportCycle {
								r.log.AddRangeWarning(&tsConfigSource, warnRange,
									fmt.Sprintf("Base config file %q forms cycle", extends))
							} else if err != parseErrorAlreadyLogged {
								r.log.AddRangeError(&tsConfigSource, warnRange,
									fmt.Sprintf("Cannot read file %q: %s",
										r.PrettyPath(logger.Path{Text: fileToCheck, Namespace: "file"}), err.Error()))
							}
							found = true
							break
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
				extendsFile := r.fs.Join(filePath, extends)
				for _, fileToCheck := range []string{extendsFile, extendsFile + ".json"} {
					base, err := r.parseJsTsConfig(fileToCheck, visited)
					if err == nil {
						result = *base
					} else if err == syscall.ENOENT {
						continue
					} else if err == parseErrorImportCycle {
						r.log.AddRangeWarning(&tsConfigSource, warnRange,
							fmt.Sprintf("Base config file %q forms cycle", extends))
					} else if err != parseErrorAlreadyLogged {
						r.log.AddRangeError(&tsConfigSource, warnRange,
							fmt.Sprintf("Cannot read file %q: %s",
								r.PrettyPath(logger.Path{Text: fileToCheck, Namespace: "file"}), err.Error()))
					}
					found = true
					break
				}
			}

			// Suppress warnings about missing base config files inside "node_modules"
			if !found && !isInsideNodeModules(r.fs, file) {
				r.log.AddRangeWarning(&tsConfigSource, warnRange,
					fmt.Sprintf("Cannot find base config file %q", extends))
			}
		}
	}

	// Parse "compilerOptions"
	if compilerOptionsJson, _, ok := getProperty(json, "compilerOptions"); ok {
		// Parse "baseUrl"
		if baseUrlJson, _, ok := getProperty(compilerOptionsJson, "baseUrl"); ok {
			if baseUrl, ok := getString(baseUrlJson); ok {
				baseUrl = r.fs.Join(filePath, baseUrl)
				result.absPathBaseUrl = &baseUrl
			}
		}

		// Parse "jsxFactory"
		if jsxFactoryJson, _, ok := getProperty(compilerOptionsJson, "jsxFactory"); ok {
			if jsxFactory, ok := getString(jsxFactoryJson); ok {
				result.jsxFactory = r.parseMemberExpressionForJSX(tsConfigSource, jsxFactoryJson.Loc, jsxFactory)
			}
		}

		// Parse "jsxFragmentFactory"
		if jsxFragmentFactoryJson, _, ok := getProperty(compilerOptionsJson, "jsxFragmentFactory"); ok {
			if jsxFragmentFactory, ok := getString(jsxFragmentFactoryJson); ok {
				result.jsxFragmentFactory = r.parseMemberExpressionForJSX(tsConfigSource, jsxFragmentFactoryJson.Loc, jsxFragmentFactory)
			}
		}

		// Parse "useDefineForClassFields"
		if useDefineForClassFieldsJson, _, ok := getProperty(compilerOptionsJson, "useDefineForClassFields"); ok {
			if useDefineForClassFields, ok := getBool(useDefineForClassFieldsJson); ok {
				result.useDefineForClassFields = useDefineForClassFields
			}
		}

		// Parse "paths"
		if pathsJson, pathsKeyLoc, ok := getProperty(compilerOptionsJson, "paths"); ok {
			if result.absPathBaseUrl == nil {
				warnRange := tsConfigSource.RangeOfString(pathsKeyLoc)
				r.log.AddRangeWarning(&tsConfigSource, warnRange,
					"Cannot use the \"paths\" property without the \"baseUrl\" property")
			} else if paths, ok := pathsJson.Data.(*js_ast.EObject); ok {
				result.paths = make(map[string][]string)
				for _, prop := range paths.Properties {
					if key, ok := getString(prop.Key); ok {
						if !isValidTSConfigPathPattern(key, r.log, tsConfigSource, prop.Key.Loc) {
							continue
						}

						// The "paths" field is an object which maps a pattern to an
						// array of remapping patterns to try, in priority order. See
						// the documentation for examples of how this is used:
						// https://www.typescriptlang.org/docs/handbook/module-resolution.html#path-mapping.
						//
						// One particular example:
						//
						//   {
						//     "compilerOptions": {
						//       "baseUrl": "projectRoot",
						//       "paths": {
						//         "*": [
						//           "*",
						//           "generated/*"
						//         ]
						//       }
						//     }
						//   }
						//
						// Matching "folder1/file2" should first check "projectRoot/folder1/file2"
						// and then, if that didn't work, also check "projectRoot/generated/folder1/file2".
						if array, ok := prop.Value.Data.(*js_ast.EArray); ok {
							for _, item := range array.Items {
								if str, ok := getString(item); ok {
									if isValidTSConfigPathPattern(str, r.log, tsConfigSource, item.Loc) {
										result.paths[key] = append(result.paths[key], str)
									}
								}
							}
						} else {
							warnRange := tsConfigSource.RangeOfString(prop.Value.Loc)
							r.log.AddRangeWarning(&tsConfigSource, warnRange, fmt.Sprintf(
								"Substitutions for pattern %q should be an array", key))
						}
					}
				}
			}
		}
	}

	return &result, nil
}

func isValidTSConfigPathPattern(text string, log logger.Log, source logger.Source, loc logger.Loc) bool {
	foundAsterisk := false
	for i := 0; i < len(text); i++ {
		if text[i] == '*' {
			if foundAsterisk {
				r := source.RangeOfString(loc)
				log.AddRangeWarning(&source, r, fmt.Sprintf(
					"Invalid pattern %q, must have at most one \"*\" character", text))
				return false
			}
			foundAsterisk = true
		}
	}
	return true
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
		info.packageJson = r.parsePackageJSON(path)

		// Propagate this browser scope into child directories
		if info.packageJson != nil && (info.packageJson.browserPackageMap != nil || info.packageJson.browserNonPackageMap != nil) {
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
			info.tsConfigJson, err = r.parseJsTsConfig(tsConfigPath, make(map[string]bool))
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
	if info.tsConfigJson == nil && parentInfo != nil {
		info.tsConfigJson = parentInfo.tsConfigJson
	}

	// Are all main fields from "package.json" missing?
	if info.packageJson == nil || info.packageJson.absMainFields == nil {
		// Look for an "index" file with known extensions
		if absolute, ok := r.loadAsIndex(path, entries); ok {
			info.absPathIndex = &absolute
		}
	}

	return info
}

func (r *resolver) parsePackageJSON(path string) *packageJson {
	packageJsonPath := r.fs.Join(path, "package.json")
	json, jsonSource, err := r.parseJSON(packageJsonPath, js_parser.ParseJSONOptions{})
	if err != nil {
		if err != parseErrorAlreadyLogged {
			r.log.AddError(nil, logger.Loc{},
				fmt.Sprintf("Cannot read file %q: %s",
					r.PrettyPath(logger.Path{Text: packageJsonPath, Namespace: "file"}), err.Error()))
		}
		return nil
	}

	toAbsPath := func(pathText string, pathRange logger.Range) *string {
		// Is it a file?
		if absolute, ok := r.loadAsFile(pathText); ok {
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

	packageJson := &packageJson{}

	// Read the "main" fields
	mainFields := r.options.MainFields
	if mainFields == nil {
		mainFields = defaultMainFields[r.options.Platform]
	}
	for _, field := range mainFields {
		if mainJson, _, ok := getProperty(json, field); ok {
			if main, ok := getString(mainJson); ok {
				if packageJson.absMainFields == nil {
					packageJson.absMainFields = make(map[string]string)
				}
				if absPath := toAbsPath(r.fs.Join(path, main), jsonSource.RangeOfString(mainJson.Loc)); absPath != nil {
					packageJson.absMainFields[field] = *absPath
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

			packageJson.browserPackageMap = browserPackageMap
			packageJson.browserNonPackageMap = browserNonPackageMap
		}
	}

	// Read the "sideEffects" property
	if sideEffectsJson, _, ok := getProperty(json, "sideEffects"); ok {
		switch data := sideEffectsJson.Data.(type) {
		case *js_ast.EBoolean:
			if !data.Value {
				// Make an empty map for "sideEffects: false", which indicates all
				// files in this module can be considered to not have side effects.
				packageJson.sideEffectsMap = make(map[string]bool)
			}

		case *js_ast.EArray:
			// The "sideEffects: []" format means all files in this module but not in
			// the array can be considered to not have side effects.
			packageJson.sideEffectsMap = make(map[string]bool)
			for _, itemJson := range data.Items {
				if item, ok := itemJson.Data.(*js_ast.EString); ok && item.Value != nil {
					absolute := r.fs.Join(path, js_lexer.UTF16ToString(item.Value))
					packageJson.sideEffectsMap[absolute] = true
				} else {
					r.log.AddWarning(&jsonSource, itemJson.Loc,
						"Expected string in array for \"sideEffects\"")
				}
			}

		default:
			r.log.AddWarning(&jsonSource, sideEffectsJson.Loc,
				"Invalid value for \"sideEffects\"")
		}
	}

	return packageJson
}

func (r *resolver) loadAsFile(path string) (string, bool) {
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
	for _, ext := range r.options.ExtensionOrder {
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

var parseErrorAlreadyLogged = errors.New("(error already logged)")

// This may return "parseErrorAlreadyLogged" in which case there was a syntax
// error, but it's already been reported. No further errors should be logged.
func (r *resolver) parseJSON(path string, options js_parser.ParseJSONOptions) (js_ast.Expr, logger.Source, error) {
	contents, err := r.fs.ReadFile(path)
	if err != nil {
		return js_ast.Expr{}, logger.Source{}, err
	}
	keyPath := logger.Path{Text: path, Namespace: "file"}
	source := logger.Source{
		KeyPath:    keyPath,
		PrettyPath: r.PrettyPath(keyPath),
		Contents:   contents,
	}
	result, ok := js_parser.ParseJSON(r.log, source, options)
	if !ok {
		return js_ast.Expr{}, logger.Source{}, parseErrorAlreadyLogged
	}
	return result, source, nil
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
	// Is this a file?
	absolute, ok := r.loadAsFile(path)
	if ok {
		return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true
	}

	// Is this a directory?
	dirInfo := r.dirInfoCached(path)
	if dirInfo == nil {
		return PathPair{}, false
	}

	// Try using the main field(s) from "package.json"
	if dirInfo.packageJson != nil && dirInfo.packageJson.absMainFields != nil {
		absMainFields := dirInfo.packageJson.absMainFields
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
func (r *resolver) matchTSConfigPaths(tsConfigJson *tsConfigJson, path string, kind ast.ImportKind) (PathPair, bool) {
	// Check for exact matches first
	for key, originalPaths := range tsConfigJson.paths {
		if key == path {
			for _, originalPath := range originalPaths {
				// Load the original path relative to the "baseUrl" from tsconfig.json
				absoluteOriginalPath := r.fs.Join(*tsConfigJson.absPathBaseUrl, originalPath)
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
	for key, originalPaths := range tsConfigJson.paths {
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
			absoluteOriginalPath := r.fs.Join(*tsConfigJson.absPathBaseUrl, originalPath)
			if absolute, ok := r.loadAsFileOrDirectory(absoluteOriginalPath, kind); ok {
				return absolute, true
			}
		}
	}

	return PathPair{}, false
}

func (r *resolver) loadNodeModules(path string, kind ast.ImportKind, dirInfo *dirInfo) (PathPair, bool) {
	// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
	if dirInfo.tsConfigJson != nil && dirInfo.tsConfigJson.absPathBaseUrl != nil {
		// Try path substitutions first
		if dirInfo.tsConfigJson.paths != nil {
			if absolute, ok := r.matchTSConfigPaths(dirInfo.tsConfigJson, path, kind); ok {
				return absolute, true
			}
		}

		// Try looking up the path relative to the base URL
		basePath := r.fs.Join(*dirInfo.tsConfigJson.absPathBaseUrl, path)
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
