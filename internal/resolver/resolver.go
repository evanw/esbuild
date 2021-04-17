package resolver

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_printer"
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

type SideEffectsData struct {
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

	DifferentCase *fs.DifferentCase

	// If present, any ES6 imports to this file can be considered to have no side
	// effects. This means they should be removed if unused.
	PrimarySideEffectsData *SideEffectsData

	IsExternal bool

	// If true, the class field transform should use Object.defineProperty().
	UseDefineForClassFieldsTS config.MaybeBool

	// If true, unused imports are retained in TypeScript code. This matches the
	// behavior of the "importsNotUsedAsValues" field in "tsconfig.json" when the
	// value is not "remove".
	PreserveUnusedImportsTS bool

	// This is the "type" field from "package.json"
	ModuleType config.ModuleType
}

type DebugMeta struct {
	notes             []logger.MsgData
	suggestionText    string
	suggestionMessage string
}

func (dm DebugMeta) LogErrorMsg(log logger.Log, source *logger.Source, r logger.Range, text string) {
	msg := logger.Msg{
		Kind:  logger.Error,
		Data:  logger.RangeData(source, r, text),
		Notes: dm.notes,
	}

	if source != nil && dm.suggestionMessage != "" {
		data := logger.RangeData(source, r, dm.suggestionMessage)
		data.Location.Suggestion = dm.suggestionText
		msg.Notes = append(msg.Notes, data)
	}

	log.AddMsg(msg)
}

type Resolver interface {
	Resolve(sourceDir string, importPath string, kind ast.ImportKind) (result *ResolveResult, debug DebugMeta)
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

type resolverQuery struct {
	*resolver
	debugLogs *debugLogs
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
	esmConditionsDefault := map[string]bool{"default": true}
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

func (rr *resolver) Resolve(sourceDir string, importPath string, kind ast.ImportKind) (*ResolveResult, DebugMeta) {
	r := resolverQuery{resolver: rr}
	if r.log.Level <= logger.LevelDebug {
		r.debugLogs = &debugLogs{what: fmt.Sprintf(
			"Resolving import %q in directory %q of type %q",
			importPath, sourceDir, kind.StringForMetafile())}
	}

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

		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this path as implicitly external")
		}

		r.flushDebugLogs(flushDueToSuccess)
		return &ResolveResult{
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
		}, DebugMeta{}
	}

	if parsed, ok := ParseDataURL(importPath); ok {
		// "import 'data:text/javascript,console.log(123)';"
		// "@import 'data:text/css,body{background:white}';"
		if parsed.DecodeMIMEType() != MIMETypeUnsupported {
			if r.debugLogs != nil {
				r.debugLogs.addNote("Putting this path in the \"dataurl\" namespace")
			}
			r.flushDebugLogs(flushDueToSuccess)
			return &ResolveResult{
				PathPair: PathPair{Primary: logger.Path{Text: importPath, Namespace: "dataurl"}},
			}, DebugMeta{}
		}

		// "background: url(data:image/png;base64,iVBORw0KGgo=);"
		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this data URL as external")
		}
		r.flushDebugLogs(flushDueToSuccess)
		return &ResolveResult{
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
		}, DebugMeta{}
	}

	// Fail now if there is no directory to resolve in. This can happen for
	// virtual modules (e.g. stdin) if a resolve directory is not specified.
	if sourceDir == "" {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Cannot resolve this path without a directory")
		}
		r.flushDebugLogs(flushDueToFailure)
		return nil, DebugMeta{}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	result, debug := r.resolveWithoutSymlinks(sourceDir, importPath, kind)
	if result == nil {
		// If resolution failed, try again with the URL query and/or hash removed
		suffix := strings.IndexAny(importPath, "?#")
		if suffix < 1 {
			r.flushDebugLogs(flushDueToFailure)
			return nil, debug
		}
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Retrying resolution after removing the suffix %q", importPath[suffix:]))
		}
		if result2, debug2 := r.resolveWithoutSymlinks(sourceDir, importPath[:suffix], kind); result2 == nil {
			r.flushDebugLogs(flushDueToFailure)
			return nil, debug
		} else {
			result = result2
			debug = debug2
			result.PathPair.Primary.IgnoredSuffix = importPath[suffix:]
			if result.PathPair.HasSecondary() {
				result.PathPair.Secondary.IgnoredSuffix = importPath[suffix:]
			}
		}
	}

	// If successful, resolve symlinks using the directory info cache
	r.finalizeResolve(result)
	r.flushDebugLogs(flushDueToSuccess)
	return result, debug
}

func (r resolverQuery) isExternalPattern(path string) bool {
	for _, pattern := range r.options.ExternalModules.Patterns {
		if len(path) >= len(pattern.Prefix)+len(pattern.Suffix) &&
			strings.HasPrefix(path, pattern.Prefix) &&
			strings.HasSuffix(path, pattern.Suffix) {
			return true
		}
	}
	return false
}

func (rr *resolver) ResolveAbs(absPath string) *ResolveResult {
	r := resolverQuery{resolver: rr}
	if r.log.Level <= logger.LevelDebug {
		r.debugLogs = &debugLogs{what: fmt.Sprintf("Getting metadata for absolute path %s", absPath)}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Just decorate the absolute path with information from parent directories
	result := &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}}
	r.finalizeResolve(result)
	r.flushDebugLogs(flushDueToSuccess)
	return result
}

func (rr *resolver) ProbeResolvePackageAsRelative(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult {
	r := resolverQuery{resolver: rr}
	absPath := r.fs.Join(sourceDir, importPath)

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if pair, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
		result := &ResolveResult{PathPair: pair, DifferentCase: diffCase}
		r.finalizeResolve(result)
		r.flushDebugLogs(flushDueToSuccess)
		return result
	}

	return nil
}

type debugLogs struct {
	what   string
	indent string
	notes  []logger.MsgData
}

func (d *debugLogs) addNote(text string) {
	if d.indent != "" {
		text = d.indent + text
	}
	d.notes = append(d.notes, logger.RangeData(nil, logger.Range{}, text))
}

func (d *debugLogs) increaseIndent() {
	d.indent += "  "
}

func (d *debugLogs) decreaseIndent() {
	d.indent = d.indent[2:]
}

type flushMode uint8

const (
	flushDueToFailure flushMode = iota
	flushDueToSuccess
)

func (r resolverQuery) flushDebugLogs(mode flushMode) {
	if r.debugLogs != nil {
		if mode == flushDueToFailure {
			r.log.AddDebugWithNotes(nil, logger.Loc{}, r.debugLogs.what, r.debugLogs.notes)
		} else if r.log.Level <= logger.LevelVerbose {
			r.log.AddVerboseWithNotes(nil, logger.Loc{}, r.debugLogs.what, r.debugLogs.notes)
		}
	}
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

func (r resolverQuery) finalizeResolve(result *ResolveResult) {
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
									if r.debugLogs != nil {
										r.debugLogs.addNote(fmt.Sprintf("Marking this file as having no side effects due to %q",
											info.packageJSON.source.KeyPath.Text))
									}
									result.PrimarySideEffectsData = info.packageJSON.sideEffectsData
								}
							}

							// Also copy over the "type" field
							result.ModuleType = info.packageJSON.moduleType
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

					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("This import is under the effect of %q",
							dirInfo.tsConfigJSON.AbsPath))
						if result.JSXFactory != nil {
							r.debugLogs.addNote(fmt.Sprintf("\"jsxFactory\" is %q due to %q",
								strings.Join(result.JSXFactory, "."),
								dirInfo.tsConfigJSON.AbsPath))
						}
						if result.JSXFragment != nil {
							r.debugLogs.addNote(fmt.Sprintf("\"jsxFragment\" is %q due to %q",
								strings.Join(result.JSXFragment, "."),
								dirInfo.tsConfigJSON.AbsPath))
						}
					}
				}

				if !r.options.PreserveSymlinks {
					if entry, _ := dirInfo.entries.Get(base); entry != nil {
						if symlink := entry.Symlink(r.fs); symlink != "" {
							// Is this entry itself a symlink?
							if r.debugLogs != nil {
								r.debugLogs.addNote(fmt.Sprintf("Resolved symlink %q to %q", path.Text, symlink))
							}
							path.Text = symlink
						} else if dirInfo.absRealPath != "" {
							// Is there at least one parent directory with a symlink?
							symlink := r.fs.Join(dirInfo.absRealPath, base)
							if r.debugLogs != nil {
								r.debugLogs.addNote(fmt.Sprintf("Resolved symlink %q to %q", path.Text, symlink))
							}
							path.Text = symlink
						}
					}
				}
			}
		}
	}

	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Primary path is %q in namespace %q", result.PathPair.Primary.Text, result.PathPair.Primary.Namespace))
		if result.PathPair.HasSecondary() {
			r.debugLogs.addNote(fmt.Sprintf("Secondary path is %q in namespace %q", result.PathPair.Secondary.Text, result.PathPair.Secondary.Namespace))
		}
	}
}

func (r resolverQuery) resolveWithoutSymlinks(sourceDir string, importPath string, kind ast.ImportKind) (*ResolveResult, DebugMeta) {
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
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("The import %q is being treated as an absolute path", importPath))
		}

		// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
		if dirInfo := r.dirInfoCached(sourceDir); dirInfo != nil && dirInfo.tsConfigJSON != nil && dirInfo.tsConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(dirInfo.tsConfigJSON, importPath, kind); ok {
				return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}, DebugMeta{}
			}
		}

		if r.options.ExternalModules.AbsPaths != nil && r.options.ExternalModules.AbsPaths[importPath] {
			// If the string literal in the source text is an absolute path and has
			// been marked as an external module, mark it as *not* an absolute path.
			// That way we preserve the literal text in the output and don't generate
			// a relative path from the output directory to that path.
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", importPath))
			}
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}, DebugMeta{}
		}

		// Run node's resolution rules (e.g. adding ".js")
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(importPath, kind); ok {
			return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}, DebugMeta{}
		}
		return nil, DebugMeta{}
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
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", absPath))
			}
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}, IsExternal: true}, DebugMeta{}
		}

		// Check the "browser" map for the first time (1 out of 2)
		if importDirInfo := r.dirInfoCached(r.fs.Dir(absPath)); importDirInfo != nil && importDirInfo.enclosingBrowserScope != nil {
			packageJSON := importDirInfo.enclosingBrowserScope.packageJSON
			if relPath, ok := r.fs.Rel(r.fs.Dir(packageJSON.source.KeyPath.Text), absPath); ok {
				if remapped, ok := r.checkBrowserMap(packageJSON, relPath); ok {
					if remapped == nil {
						return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}}, DebugMeta{}
					}
					if remappedResult, ok, diffCase, _ := r.resolveWithoutRemapping(importDirInfo.enclosingBrowserScope, *remapped, kind); ok {
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
				return nil, DebugMeta{}
			}
		}
	}

	if checkPackage {
		// Check for external packages first
		if r.options.ExternalModules.NodeModules != nil {
			query := importPath
			for {
				if r.options.ExternalModules.NodeModules[query] {
					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", query))
					}
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath}}, IsExternal: true}, DebugMeta{}
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
			return nil, DebugMeta{}
		}

		// Support remapping one package path to another via the "browser" field
		if sourceDirInfo.enclosingBrowserScope != nil {
			packageJSON := sourceDirInfo.enclosingBrowserScope.packageJSON
			if remapped, ok := r.checkBrowserMap(packageJSON, importPath); ok {
				if remapped == nil {
					// "browser": {"module": false}
					if absolute, ok, diffCase, _ := r.loadNodeModules(importPath, kind, sourceDirInfo); ok {
						absolute.Primary = logger.Path{Text: absolute.Primary.Text, Namespace: "file", Flags: logger.PathDisabled}
						if absolute.HasSecondary() {
							absolute.Secondary = logger.Path{Text: absolute.Secondary.Text, Namespace: "file", Flags: logger.PathDisabled}
						}
						return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}, DebugMeta{}
					} else {
						return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Flags: logger.PathDisabled}}, DifferentCase: diffCase}, DebugMeta{}
					}
				}

				// "browser": {"module": "./some-file"}
				// "browser": {"module": "another-module"}
				importPath = *remapped
				sourceDirInfo = sourceDirInfo.enclosingBrowserScope
			}
		}

		if absolute, ok, diffCase, debug := r.resolveWithoutRemapping(sourceDirInfo, importPath, kind); ok {
			result = ResolveResult{PathPair: absolute, DifferentCase: diffCase}
		} else {
			// Note: node's "self references" are not currently supported
			return nil, debug
		}
	}

	// Check the "browser" map for the second time (2 out of 2)
	for _, path := range result.PathPair.iter() {
		if resultDirInfo := r.dirInfoCached(r.fs.Dir(path.Text)); resultDirInfo != nil && resultDirInfo.enclosingBrowserScope != nil {
			packageJSON := resultDirInfo.enclosingBrowserScope.packageJSON
			if relPath, ok := r.fs.Rel(r.fs.Dir(packageJSON.source.KeyPath.Text), path.Text); ok {
				if remapped, ok := r.checkBrowserMap(packageJSON, relPath); ok {
					if remapped == nil {
						path.Flags |= logger.PathDisabled
					} else {
						if remappedResult, ok, _, _ := r.resolveWithoutRemapping(resultDirInfo.enclosingBrowserScope, *remapped, kind); ok {
							*path = remappedResult.Primary
						} else {
							return nil, DebugMeta{}
						}
					}
				}
			}
		}
	}

	return &result, DebugMeta{}
}

func (r resolverQuery) resolveWithoutRemapping(sourceDirInfo *dirInfo, importPath string, kind ast.ImportKind) (PathPair, bool, *fs.DifferentCase, DebugMeta) {
	if IsPackagePath(importPath) {
		return r.loadNodeModules(importPath, kind, sourceDirInfo)
	} else {
		pair, ok, diffCase := r.loadAsFileOrDirectory(r.fs.Join(sourceDirInfo.absPath, importPath), kind)
		return pair, ok, diffCase, DebugMeta{}
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
	packageJSON    *packageJSON  // Is there a "package.json" file?
	tsConfigJSON   *TSConfigJSON // Is there a "tsconfig.json" file in this directory or a parent directory?
	absRealPath    string        // If non-empty, this is the real absolute path resolving any symlinks
}

func (r resolverQuery) dirInfoCached(path string) *dirInfo {
	// First, check the cache
	cached, ok := r.dirCache[path]

	// Cache hit: stop now
	if !ok {
		// Cache miss: read the info
		cached = r.dirInfoUncached(path)

		// Update the cache unconditionally. Even if the read failed, we don't want to
		// retry again later. The directory is inaccessible so trying again is wasted.
		r.dirCache[path] = cached
	}

	if r.debugLogs != nil {
		if cached == nil {
			r.debugLogs.addNote(fmt.Sprintf("Failed to read directory %q", path))
		} else {
			count := cached.entries.Len()
			entries := "entries"
			if count == 1 {
				entries = "entry"
			}
			r.debugLogs.addNote(fmt.Sprintf("Read %d %s for directory %q", count, entries, path))
		}
	}

	return cached
}

var errParseErrorImportCycle = errors.New("(import cycle)")
var errParseErrorAlreadyLogged = errors.New("(error already logged)")

// This may return "parseErrorAlreadyLogged" in which case there was a syntax
// error, but it's already been reported. No further errors should be logged.
//
// Nested calls may also return "parseErrorImportCycle". In that case the
// caller is responsible for logging an appropriate error message.
func (r resolverQuery) parseTSConfig(file string, visited map[string]bool) (*TSConfigJSON, error) {
	// Don't infinite loop if a series of "extends" links forms a cycle
	if visited[file] {
		return nil, errParseErrorImportCycle
	}
	visited[file] = true

	contents, err, originalError := r.caches.FSCache.ReadFile(r.fs, file)
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read file %q: %s", file, originalError.Error()))
	}
	if err != nil {
		return nil, err
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("The file %q exists", file))
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
						} else if err == errParseErrorImportCycle {
							r.log.AddRangeWarning(&source, extendsRange,
								fmt.Sprintf("Base config file %q forms cycle", extends))
						} else if err != errParseErrorAlreadyLogged {
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
				} else if err == errParseErrorImportCycle {
					r.log.AddRangeWarning(&source, extendsRange,
						fmt.Sprintf("Base config file %q forms cycle", extends))
				} else if err != errParseErrorAlreadyLogged {
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
		return nil, errParseErrorAlreadyLogged
	}

	if result.BaseURL != nil && !r.fs.IsAbs(*result.BaseURL) {
		*result.BaseURL = r.fs.Join(fileDir, *result.BaseURL)
	}

	if result.Paths != nil && !r.fs.IsAbs(result.BaseURLForPaths) {
		result.BaseURLForPaths = r.fs.Join(fileDir, result.BaseURLForPaths)
	}

	return result, nil
}

func (r resolverQuery) dirInfoUncached(path string) *dirInfo {
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
	entries, err, originalError := r.fs.ReadDirectory(path)
	if err == syscall.EACCES {
		// Just pretend this directory is empty if we can't access it. This is the
		// case on Unix for directories that only have the execute permission bit
		// set. It means we will just pass through the empty directory and
		// continue to check the directories above it, which is now node behaves.
		entries = fs.MakeEmptyDirEntries(path)
		err = nil
	}
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read directory %q: %s", path, originalError.Error()))
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
					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("Resolved symlink %q to %q", path, symlink))
					}
					info.absRealPath = symlink
				} else if parentInfo.absRealPath != "" {
					symlink := r.fs.Join(parentInfo.absRealPath, base)
					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("Resolved symlink %q to %q", path, symlink))
					}
					info.absRealPath = symlink
				}
			}
		}
	}

	// Record if this directory has a package.json file
	if entry, _ := entries.Get("package.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
		info.packageJSON = r.parsePackageJSON(path)

		// Propagate this browser scope into child directories
		if info.packageJSON != nil && info.packageJSON.browserMap != nil {
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
				} else if err != errParseErrorAlreadyLogged {
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

	return info
}

func (r resolverQuery) loadAsFile(path string, extensionOrder []string) (string, bool, *fs.DifferentCase) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Attempting to load %q as a file", path))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	// Read the directory entries once to minimize locking
	dirPath := r.fs.Dir(path)
	entries, err, originalError := r.fs.ReadDirectory(dirPath)
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read directory %q: %s", dirPath, originalError.Error()))
	}
	if err != nil {
		if err != syscall.ENOENT {
			r.log.AddError(nil, logger.Loc{},
				fmt.Sprintf("  Cannot read directory %q: %s",
					r.PrettyPath(logger.Path{Text: dirPath, Namespace: "file"}), err.Error()))
		}
		return "", false, nil
	}

	base := r.fs.Base(path)

	// Try the plain path without any extensions
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Checking for file %q", base))
	}
	if entry, diffCase := entries.Get(base); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Found file %q", base))
		}
		return path, true, diffCase
	}

	// Try the path with extensions
	for _, ext := range extensionOrder {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Checking for file %q", base+ext))
		}
		if entry, diffCase := entries.Get(base + ext); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found file %q", base+ext))
			}
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
				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("Rewrote to %q", base[:lastDot]+ext))
				}
				return path[:len(path)-(len(base)-lastDot)] + ext, true, diffCase
			}
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Failed to rewrite to %q", base[:lastDot]+ext))
			}
		}
	}

	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to find file %q", base))
	}
	return "", false, nil
}

func (r resolverQuery) loadAsIndex(dirInfo *dirInfo, path string, extensionOrder []string) (PathPair, bool, *fs.DifferentCase) {
	// Try the "index" file with extensions
	for _, ext := range extensionOrder {
		base := "index" + ext
		if entry, diffCase := dirInfo.entries.Get(base); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found file %q", r.fs.Join(path, base)))
			}
			return PathPair{Primary: logger.Path{Text: r.fs.Join(path, base), Namespace: "file"}}, true, diffCase
		}
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Failed to find file %q", r.fs.Join(path, base)))
		}
	}

	return PathPair{}, false, nil
}

func (r resolverQuery) loadAsIndexWithBrowserRemapping(dirInfo *dirInfo, path string, extensionOrder []string) (PathPair, bool, *fs.DifferentCase) {
	// Potentially remap using the "browser" field
	if dirInfo.enclosingBrowserScope != nil {
		if remapped, ok := r.checkBrowserMap(dirInfo.enclosingBrowserScope.packageJSON, "index"); ok {
			if remapped == nil {
				return PathPair{Primary: logger.Path{Text: r.fs.Join(path, "index"), Namespace: "file", Flags: logger.PathDisabled}}, true, nil
			}
			remappedAbs := r.fs.Join(path, *remapped)

			// Is this a file?
			absolute, ok, diffCase := r.loadAsFile(remappedAbs, extensionOrder)
			if ok {
				return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true, diffCase
			}

			// Is it a directory with an index?
			if fieldDirInfo := r.dirInfoCached(remappedAbs); fieldDirInfo != nil {
				if absolute, ok, _ := r.loadAsIndex(fieldDirInfo, remappedAbs, extensionOrder); ok {
					return absolute, true, nil
				}
			}

			return PathPair{}, false, nil
		}
	}

	return r.loadAsIndex(dirInfo, path, extensionOrder)
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

func (r resolverQuery) loadAsFileOrDirectory(path string, kind ast.ImportKind) (PathPair, bool, *fs.DifferentCase) {
	// Use a special import order for CSS "@import" imports
	extensionOrder := r.options.ExtensionOrder
	if kind == ast.ImportAt || kind == ast.ImportAtConditional {
		extensionOrder = r.atImportExtensionOrder
	}

	// Is this a file?
	absolute, ok, diffCase := r.loadAsFile(path, extensionOrder)
	if ok {
		return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true, diffCase
	}

	// Is this a directory?
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Attempting to load %q as a directory", path))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}
	dirInfo := r.dirInfoCached(path)
	if dirInfo == nil {
		return PathPair{}, false, nil
	}

	// Try using the main field(s) from "package.json"
	if dirInfo.packageJSON != nil && dirInfo.packageJSON.mainFields != nil {
		mainFieldValues := dirInfo.packageJSON.mainFields
		mainFieldKeys := r.options.MainFields
		autoMain := false

		// If the user has not explicitly specified a "main" field order,
		// use a default one determined by the current platform target
		if mainFieldKeys == nil {
			mainFieldKeys = defaultMainFields[r.options.Platform]
			autoMain = true
		}

		loadMainField := func(fieldRelPath string, field string) (PathPair, bool, *fs.DifferentCase) {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found main field %q with path %q", field, fieldRelPath))
				r.debugLogs.increaseIndent()
				defer r.debugLogs.decreaseIndent()
			}

			// Potentially remap using the "browser" field
			if dirInfo.enclosingBrowserScope != nil {
				if remapped, ok := r.checkBrowserMap(dirInfo.enclosingBrowserScope.packageJSON, fieldRelPath); ok {
					if remapped == nil {
						return PathPair{Primary: logger.Path{Text: r.fs.Join(path, fieldRelPath), Namespace: "file", Flags: logger.PathDisabled}}, true, nil
					}
					fieldRelPath = *remapped
				}
			}
			fieldAbsPath := r.fs.Join(path, fieldRelPath)

			// Is this a file?
			absolute, ok, diffCase := r.loadAsFile(fieldAbsPath, extensionOrder)
			if ok {
				return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true, diffCase
			}

			// Is it a directory with an index?
			if fieldDirInfo := r.dirInfoCached(fieldAbsPath); fieldDirInfo != nil {
				if absolute, ok, _ := r.loadAsIndexWithBrowserRemapping(fieldDirInfo, fieldAbsPath, extensionOrder); ok {
					return absolute, true, nil
				}
			}

			return PathPair{}, false, nil
		}

		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Searching for main fields in %q", dirInfo.packageJSON.source.KeyPath.Text))
		}

		for _, key := range mainFieldKeys {
			fieldRelPath, ok := mainFieldValues[key]
			if !ok {
				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("Did not find main field %q", key))
				}
				continue
			}

			absolute, ok, diffCase := loadMainField(fieldRelPath, key)
			if !ok {
				continue
			}

			// If the user did not manually configure a "main" field order, then
			// use a special per-module automatic algorithm to decide whether to
			// use "module" or "main" based on whether the package is imported
			// using "import" or "require".
			if autoMain && key == "module" {
				var absoluteMain PathPair
				var okMain bool
				var diffCaseMain *fs.DifferentCase

				if mainRelPath, ok := mainFieldValues["main"]; ok {
					if absolute, ok, diffCase := loadMainField(mainRelPath, "main"); ok {
						absoluteMain = absolute
						okMain = true
						diffCaseMain = diffCase
					}
				} else {
					// Some packages have a "module" field without a "main" field but
					// still have an implicit "index.js" file. In that case, treat that
					// as the value for "main".
					if absolute, ok, diffCase := r.loadAsIndexWithBrowserRemapping(dirInfo, path, extensionOrder); ok {
						absoluteMain = absolute
						okMain = true
						diffCaseMain = diffCase
					}
				}

				if okMain {
					// If both the "main" and "module" fields exist, use "main" if the
					// path is for "require" and "module" if the path is for "import".
					// If we're using "module", return enough information to be able to
					// fall back to "main" later if something ended up using "require()"
					// with this same path. The goal of this code is to avoid having
					// both the "module" file and the "main" file in the bundle at the
					// same time.
					if kind != ast.ImportRequire {
						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("Resolved to %q using the \"module\" field in %q",
								absolute.Primary.Text, dirInfo.packageJSON.source.KeyPath.Text))
							r.debugLogs.addNote(fmt.Sprintf("The fallback path in case of \"require\" is %q",
								absoluteMain.Primary.Text))
						}
						return PathPair{
							// This is the whole point of the path pair
							Primary:   absolute.Primary,
							Secondary: absoluteMain.Primary,
						}, true, diffCase
					} else {
						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("Resolved to %q because of \"require\"", absoluteMain.Primary.Text))
						}
						return absoluteMain, true, diffCaseMain
					}
				}
			}

			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Resolved to %q using the %q field in %q",
					absolute.Primary.Text, key, dirInfo.packageJSON.source.KeyPath.Text))
			}
			return absolute, true, diffCase
		}
	}

	// Look for an "index" file with known extensions
	if absolute, ok, diffCase := r.loadAsIndexWithBrowserRemapping(dirInfo, path, extensionOrder); ok {
		return absolute, true, diffCase
	}

	return PathPair{}, false, nil
}

// This closely follows the behavior of "tryLoadModuleUsingPaths()" in the
// official TypeScript compiler
func (r resolverQuery) matchTSConfigPaths(tsConfigJSON *TSConfigJSON, path string, kind ast.ImportKind) (PathPair, bool, *fs.DifferentCase) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Matching %q against \"paths\" in %q", path, tsConfigJSON.AbsPath))
	}

	absBaseURL := tsConfigJSON.BaseURLForPaths

	// The explicit base URL should take precedence over the implicit base URL
	// if present. This matters when a tsconfig.json file overrides "baseUrl"
	// from another extended tsconfig.json file but doesn't override "paths".
	if tsConfigJSON.BaseURL != nil {
		absBaseURL = *tsConfigJSON.BaseURL
	}

	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Using %q as \"baseURL\"", absBaseURL))
	}

	// Check for exact matches first
	for key, originalPaths := range tsConfigJSON.Paths {
		if key == path {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found an exact match for %q in \"paths\"", key))
			}
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
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Found a fuzzy match for %q in \"paths\"", longestMatch.prefix+"*"+longestMatch.suffix))
		}

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

func (r resolverQuery) loadNodeModules(importPath string, kind ast.ImportKind, dirInfo *dirInfo) (PathPair, bool, *fs.DifferentCase, DebugMeta) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Searching for %q in \"node_modules\" directories starting from %q", importPath, dirInfo.absPath))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
	if dirInfo.tsConfigJSON != nil {
		// Try path substitutions first
		if dirInfo.tsConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(dirInfo.tsConfigJSON, importPath, kind); ok {
				return absolute, true, diffCase, DebugMeta{}
			}
		}

		// Try looking up the path relative to the base URL
		if dirInfo.tsConfigJSON.BaseURL != nil {
			basePath := r.fs.Join(*dirInfo.tsConfigJSON.BaseURL, importPath)
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(basePath, kind); ok {
				return absolute, true, diffCase, DebugMeta{}
			}
		}
	}

	esmPackageName, esmPackageSubpath, esmOK := esmParsePackageName(importPath)
	if r.debugLogs != nil && esmOK {
		r.debugLogs.addNote(fmt.Sprintf("Parsed package name %q and package subpath %q", esmPackageName, esmPackageSubpath))
	}

	// Then check for the package in any enclosing "node_modules" directories
	for {
		// Skip directories that are themselves called "node_modules", since we
		// don't ever want to search for "node_modules/node_modules"
		if dirInfo.hasNodeModules {
			absPath := r.fs.Join(dirInfo.absPath, "node_modules", importPath)
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Checking for a package in the directory %q", absPath))
			}

			// Check for an "exports" map in the package's package.json folder
			if esmOK {
				absPkgPath := r.fs.Join(dirInfo.absPath, "node_modules", esmPackageName)
				if pkgDirInfo := r.dirInfoCached(absPkgPath); pkgDirInfo != nil {
					// Check for an "exports" map in the package's package.json folder
					if packageJSON := pkgDirInfo.packageJSON; packageJSON != nil && packageJSON.exportsMap != nil {
						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("Looking for %q in \"exports\" map in %q", esmPackageSubpath, packageJSON.source.KeyPath.Text))
							r.debugLogs.increaseIndent()
							defer r.debugLogs.decreaseIndent()
						}

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
						resolvedPath, status, debug := r.esmPackageExportsResolveWithPostConditions("/", esmPackageSubpath, packageJSON.exportsMap.root, conditions)
						if (status == peStatusExact || status == peStatusInexact) && strings.HasPrefix(resolvedPath, "/") {
							absResolvedPath := r.fs.Join(absPkgPath, resolvedPath[1:])

							switch status {
							case peStatusExact:
								if r.debugLogs != nil {
									r.debugLogs.addNote(fmt.Sprintf("The resolved path %q is exact", absResolvedPath))
								}
								resolvedDirInfo := r.dirInfoCached(r.fs.Dir(absResolvedPath))
								if resolvedDirInfo == nil {
									status = peStatusModuleNotFound
								} else if entry, diffCase := resolvedDirInfo.entries.Get(r.fs.Base(absResolvedPath)); entry == nil {
									status = peStatusModuleNotFound
								} else if kind := entry.Kind(r.fs); kind == fs.DirEntry {
									if r.debugLogs != nil {
										r.debugLogs.addNote(fmt.Sprintf("The path %q is a directory, which is not allowed", absResolvedPath))
									}
									status = peStatusUnsupportedDirectoryImport
								} else if kind != fs.FileEntry {
									status = peStatusModuleNotFound
								} else {
									if r.debugLogs != nil {
										r.debugLogs.addNote(fmt.Sprintf("Resolved to %q", absResolvedPath))
									}
									return PathPair{Primary: logger.Path{Text: absResolvedPath, Namespace: "file"}}, true, diffCase, DebugMeta{}
								}

							case peStatusInexact:
								// If this was resolved against an expansion key ending in a "/"
								// instead of a "*", we need to try CommonJS-style implicit
								// extension and/or directory detection.
								if r.debugLogs != nil {
									r.debugLogs.addNote(fmt.Sprintf("The resolved path %q is inexact", absResolvedPath))
								}
								if absolute, ok, diffCase := r.loadAsFileOrDirectory(absResolvedPath, kind); ok {
									return absolute, true, diffCase, DebugMeta{}
								}
								status = peStatusModuleNotFound
							}
						}

						var debugMeta DebugMeta
						if strings.HasPrefix(resolvedPath, "/") {
							resolvedPath = "." + resolvedPath
						}

						// Provide additional details about the failure to help with debugging
						switch status {
						case peStatusInvalidModuleSpecifier:
							debugMeta.notes = []logger.MsgData{logger.RangeData(&packageJSON.source, debug.token,
								fmt.Sprintf("The module specifier %q is invalid", resolvedPath))}

						case peStatusInvalidPackageConfiguration:
							debugMeta.notes = []logger.MsgData{logger.RangeData(&packageJSON.source, debug.token,
								"The package configuration has an invalid value here")}

						case peStatusInvalidPackageTarget:
							why := fmt.Sprintf("The package target %q is invalid", resolvedPath)
							if resolvedPath == "" {
								// "PACKAGE_TARGET_RESOLVE" is specified to throw an "Invalid
								// Package Target" error for what is actually an invalid package
								// configuration error
								why = "The package configuration has an invalid value here"
							}
							debugMeta.notes = []logger.MsgData{logger.RangeData(&packageJSON.source, debug.token, why)}

						case peStatusPackagePathNotExported:
							debugMeta.notes = []logger.MsgData{logger.RangeData(&packageJSON.source, debug.token,
								fmt.Sprintf("The path %q is not exported by package %q", esmPackageSubpath, esmPackageName))}

							// If this fails, try to resolve it using the old algorithm
							if absolute, ok, _ := r.loadAsFileOrDirectory(absPath, kind); ok && absolute.Primary.Namespace == "file" {
								if relPath, ok := r.fs.Rel(absPkgPath, absolute.Primary.Text); ok {
									query := "." + path.Join("/", strings.ReplaceAll(relPath, "\\", "/"))

									// If that succeeds, try to do a reverse lookup using the
									// "exports" map for the currently-active set of conditions
									if ok, subpath, token := r.esmPackageExportsReverseResolve(
										query, pkgDirInfo.packageJSON.exportsMap.root, conditions); ok {
										debugMeta.notes = append(debugMeta.notes, logger.RangeData(&pkgDirInfo.packageJSON.source, token,
											fmt.Sprintf("The file %q is exported at path %q", query, subpath)))

										// Provide an inline suggestion message with the correct import path
										actualImportPath := path.Join(esmPackageName, subpath)
										debugMeta.suggestionText = string(js_printer.QuoteForJSON(actualImportPath, false))
										debugMeta.suggestionMessage = fmt.Sprintf("Import from %q to get the file %q",
											actualImportPath, r.PrettyPath(absolute.Primary))
									}
								}
							}

						case peStatusModuleNotFound:
							debugMeta.notes = []logger.MsgData{logger.RangeData(&packageJSON.source, debug.token,
								fmt.Sprintf("The module %q was not found on the file system", resolvedPath))}

						case peStatusUnsupportedDirectoryImport:
							debugMeta.notes = []logger.MsgData{logger.RangeData(&packageJSON.source, debug.token,
								fmt.Sprintf("Importing the directory %q is not supported", resolvedPath))}

						case peStatusUndefinedNoConditionsMatch:
							prettyPrintConditions := func(conditions []string) string {
								quoted := make([]string, len(conditions))
								for i, condition := range conditions {
									quoted[i] = fmt.Sprintf("%q", condition)
								}
								return strings.Join(quoted, ", ")
							}
							keys := make([]string, 0, len(conditions))
							for key := range conditions {
								keys = append(keys, key)
							}
							sort.Strings(keys)
							debugMeta.notes = []logger.MsgData{
								logger.RangeData(&packageJSON.source, packageJSON.exportsMap.root.firstToken,
									fmt.Sprintf("The path %q is not currently exported by package %q",
										esmPackageSubpath, esmPackageName)),
								logger.RangeData(&packageJSON.source, debug.token,
									fmt.Sprintf("None of the conditions provided (%s) match any of the currently active conditions (%s)",
										prettyPrintConditions(debug.unmatchedConditions),
										prettyPrintConditions(keys),
									))}
							for _, key := range debug.unmatchedConditions {
								if key == "import" && (kind == ast.ImportRequire || kind == ast.ImportRequireResolve) {
									debugMeta.suggestionMessage = "Consider using an \"import\" statement to import this file"
								} else if key == "require" && (kind == ast.ImportStmt || kind == ast.ImportDynamic) {
									debugMeta.suggestionMessage = "Consider using a \"require()\" call to import this file"
								}
							}
						}

						return PathPair{}, false, nil, debugMeta
					}

					// Check the "browser" map
					if pkgDirInfo.enclosingBrowserScope != nil {
						packageJSON := pkgDirInfo.enclosingBrowserScope.packageJSON
						if relPath, ok := r.fs.Rel(r.fs.Dir(packageJSON.source.KeyPath.Text), absPath); ok {
							if remapped, ok := r.checkBrowserMap(packageJSON, relPath); ok {
								if remapped == nil {
									return PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil, DebugMeta{}
								}
								if remappedResult, ok, diffCase, notes := r.resolveWithoutRemapping(pkgDirInfo.enclosingBrowserScope, *remapped, kind); ok {
									return remappedResult, true, diffCase, notes
								}
							}
						}
					}
				}
			}

			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
				return absolute, true, diffCase, DebugMeta{}
			}
		}

		// Go to the parent directory, stopping at the file system root
		dirInfo = dirInfo.parent
		if dirInfo == nil {
			break
		}
	}

	// Then check the global "NODE_PATH" environment variable.
	//
	// Note: This is a deviation from node's published module resolution
	// algorithm. The published algorithm says "NODE_PATH" must take precedence
	// over "node_modules" paths, but it appears that the published algorithm is
	// incorrect. We follow node's actual behavior instead of following the
	// published algorithm. See also: https://github.com/nodejs/node/issues/38128.
	for _, absDir := range r.options.AbsNodePaths {
		absPath := r.fs.Join(absDir, importPath)
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath, kind); ok {
			return absolute, true, diffCase, DebugMeta{}
		}
	}

	return PathPair{}, false, nil, DebugMeta{}
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
