package resolver

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
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

// These are the main fields to use when the "main fields" setting is configured
// to something unusual, such as something without the "main" field.
var mainFieldsForFailure = []string{"main", "module"}

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

	IsExternal bool
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

	// If non-empty, this false value came from a plugin
	PluginName string

	Range logger.Range

	// If true, "sideEffects" was an array. If false, "sideEffects" was false.
	IsSideEffectsArrayInJSON bool
}

type ResolveResult struct {
	PathPair PathPair

	// If this was resolved by a plugin, the plugin gets to store its data here
	PluginData interface{}

	DifferentCase *fs.DifferentCase

	// If present, any ES6 imports to this file can be considered to have no side
	// effects. This means they should be removed if unused.
	PrimarySideEffectsData *SideEffectsData

	// These are from "tsconfig.json"
	TSConfigJSX    config.TSConfigJSX
	TSConfig       *config.TSConfig
	TSAlwaysStrict *config.TSAlwaysStrict

	// This is the "type" field from "package.json"
	ModuleTypeData js_ast.ModuleTypeData
}

type suggestionRange uint8

const (
	suggestionRangeFull suggestionRange = iota
	suggestionRangeEnd
)

type DebugMeta struct {
	notes              []logger.MsgData
	suggestionText     string
	suggestionMessage  string
	suggestionRange    suggestionRange
	ModifiedImportPath string
}

func (dm DebugMeta) LogErrorMsg(log logger.Log, source *logger.Source, r logger.Range, text string, suggestion string, notes []logger.MsgData) {
	tracker := logger.MakeLineColumnTracker(source)

	if source != nil && dm.suggestionMessage != "" {
		suggestionRange := r
		if dm.suggestionRange == suggestionRangeEnd {
			suggestionRange = logger.Range{Loc: logger.Loc{Start: r.End() - 1}}
		}
		data := tracker.MsgData(suggestionRange, dm.suggestionMessage)
		data.Location.Suggestion = dm.suggestionText
		dm.notes = append(dm.notes, data)
	}

	msg := logger.Msg{
		Kind:  logger.Error,
		Data:  tracker.MsgData(r, text),
		Notes: append(dm.notes, notes...),
	}

	if msg.Data.Location != nil && suggestion != "" {
		msg.Data.Location.Suggestion = suggestion
	}

	log.AddMsg(msg)
}

type Resolver struct {
	fs     fs.FS
	log    logger.Log
	caches *cache.CacheSet

	tsConfigOverride *TSConfigJSON

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
	cssExtensionOrder []string

	// A special sorted import order for imports inside packages.
	//
	// The "resolve extensions" setting determines the order of implicit
	// extensions to try when resolving imports with the extension omitted.
	// Sometimes people author a package using TypeScript and publish both the
	// compiled JavaScript and the original TypeScript. The compiled JavaScript
	// depends on the "tsconfig.json" settings that were passed to "tsc" when
	// it was compiled, and we don't know what they are (they may even be
	// unknowable if the "tsconfig.json" file wasn't published).
	//
	// To work around this, we sort TypeScript file extensions after JavaScript
	// file extensions (but only within packages) so that esbuild doesn't load
	// the original source code in these scenarios. Instead we should load the
	// compiled code, which is what will be loaded by node at run-time.
	nodeModulesExtensionOrder []string

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCache map[string]*dirInfo

	pnpManifestWasChecked bool
	pnpManifest           *pnpData

	options config.Options

	// This mutex serves two purposes. First of all, it guards access to "dirCache"
	// which is potentially mutated during path resolution. But this mutex is also
	// necessary for performance. The "React admin" benchmark mysteriously runs
	// twice as fast when this mutex is locked around the whole resolve operation
	// instead of around individual accesses to "dirCache". For some reason,
	// reducing parallelism in the resolver helps the rest of the bundler go
	// faster. I'm not sure why this is but please don't change this unless you
	// do a lot of testing with various benchmarks and there aren't any regressions.
	mutex sync.Mutex
}

type resolverQuery struct {
	*Resolver
	debugMeta *DebugMeta
	debugLogs *debugLogs
	kind      ast.ImportKind
}

func NewResolver(call config.APICall, fs fs.FS, log logger.Log, caches *cache.CacheSet, options *config.Options) *Resolver {
	// Filter out non-CSS extensions for CSS "@import" imports
	cssExtensionOrder := make([]string, 0, len(options.ExtensionOrder))
	for _, ext := range options.ExtensionOrder {
		if loader := config.LoaderFromFileExtension(options.ExtensionToLoader, ext); loader == config.LoaderNone || loader.IsCSS() {
			cssExtensionOrder = append(cssExtensionOrder, ext)
		}
	}

	// Sort all TypeScript file extensions after all JavaScript file extensions
	// for imports of files inside of "node_modules" directories. But insert
	// the TypeScript file extensions right after the last JavaScript file
	// extension instead of at the end so that they might come before the
	// first CSS file extension, which is important to people that publish
	// TypeScript and CSS code to npm with the same file names for both.
	nodeModulesExtensionOrder := make([]string, 0, len(options.ExtensionOrder))
	split := 0
	for i, ext := range options.ExtensionOrder {
		if loader := config.LoaderFromFileExtension(options.ExtensionToLoader, ext); loader == config.LoaderJS || loader == config.LoaderJSX {
			split = i + 1 // Split after the last JavaScript extension
		}
	}
	if split != 0 { // Only do this if there are any JavaScript extensions
		for _, ext := range options.ExtensionOrder[:split] { // Non-TypeScript extensions before the split
			if loader := config.LoaderFromFileExtension(options.ExtensionToLoader, ext); !loader.IsTypeScript() {
				nodeModulesExtensionOrder = append(nodeModulesExtensionOrder, ext)
			}
		}
		for _, ext := range options.ExtensionOrder { // All TypeScript extensions
			if loader := config.LoaderFromFileExtension(options.ExtensionToLoader, ext); loader.IsTypeScript() {
				nodeModulesExtensionOrder = append(nodeModulesExtensionOrder, ext)
			}
		}
		for _, ext := range options.ExtensionOrder[split:] { // Non-TypeScript extensions after the split
			if loader := config.LoaderFromFileExtension(options.ExtensionToLoader, ext); !loader.IsTypeScript() {
				nodeModulesExtensionOrder = append(nodeModulesExtensionOrder, ext)
			}
		}
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

	fs.Cwd()

	res := &Resolver{
		fs:                        fs,
		log:                       log,
		options:                   *options,
		caches:                    caches,
		dirCache:                  make(map[string]*dirInfo),
		cssExtensionOrder:         cssExtensionOrder,
		nodeModulesExtensionOrder: nodeModulesExtensionOrder,
		esmConditionsDefault:      esmConditionsDefault,
		esmConditionsImport:       esmConditionsImport,
		esmConditionsRequire:      esmConditionsRequire,
	}

	// Handle the "tsconfig.json" override when the resolver is created. This
	// isn't done when we validate the build options both because the code for
	// "tsconfig.json" handling is already in the resolver, and because we want
	// watch mode to pick up changes to "tsconfig.json" and rebuild.
	var debugMeta DebugMeta
	if options.TSConfigPath != "" || options.TSConfigRaw != "" {
		r := resolverQuery{
			Resolver:  res,
			debugMeta: &debugMeta,
		}
		var visited map[string]bool
		var err error
		if call == config.BuildCall {
			visited = make(map[string]bool)
		}
		if options.TSConfigPath != "" {
			if r.log.Level <= logger.LevelDebug {
				r.debugLogs = &debugLogs{what: fmt.Sprintf("Resolving tsconfig file %q", options.TSConfigPath)}
			}
			res.tsConfigOverride, err = r.parseTSConfig(options.TSConfigPath, visited, fs.Dir(options.TSConfigPath))
		} else {
			source := logger.Source{
				KeyPath:    logger.Path{Text: fs.Join(fs.Cwd(), "<tsconfig.json>"), Namespace: "file"},
				PrettyPath: "<tsconfig.json>",
				Contents:   options.TSConfigRaw,
			}
			res.tsConfigOverride, err = r.parseTSConfigFromSource(source, visited, fs.Cwd())
		}
		if err != nil {
			if err == syscall.ENOENT {
				r.log.AddError(nil, logger.Range{}, fmt.Sprintf("Cannot find tsconfig file %q",
					PrettyPath(r.fs, logger.Path{Text: options.TSConfigPath, Namespace: "file"})))
			} else if err != errParseErrorAlreadyLogged {
				r.log.AddError(nil, logger.Range{}, fmt.Sprintf("Cannot read file %q: %s",
					PrettyPath(r.fs, logger.Path{Text: options.TSConfigPath, Namespace: "file"}), err.Error()))
			}
		} else {
			r.flushDebugLogs(flushDueToSuccess)
		}
	}

	// Mutate the provided options by settings from "tsconfig.json" if present
	if res.tsConfigOverride != nil {
		options.TS.Config = res.tsConfigOverride.Settings
		res.tsConfigOverride.JSXSettings.ApplyTo(&options.JSX)
		options.TSAlwaysStrict = res.tsConfigOverride.TSAlwaysStrictOrStrict()
	}

	return res
}

func (res *Resolver) Resolve(sourceDir string, importPath string, kind ast.ImportKind) (*ResolveResult, DebugMeta) {
	var debugMeta DebugMeta
	r := resolverQuery{
		Resolver:  res,
		debugMeta: &debugMeta,
		kind:      kind,
	}
	if r.log.Level <= logger.LevelDebug {
		r.debugLogs = &debugLogs{what: fmt.Sprintf(
			"Resolving import %q in directory %q of type %q",
			importPath, sourceDir, kind.StringForMetafile())}
	}

	// Apply package alias substitutions first
	if r.options.PackageAliases != nil && IsPackagePath(importPath) {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Checking for package alias matches")
		}
		longestKey := ""
		longestValue := ""

		for key, value := range r.options.PackageAliases {
			if len(key) > len(longestKey) && strings.HasPrefix(importPath, key) && (len(importPath) == len(key) || importPath[len(key)] == '/') {
				longestKey = key
				longestValue = value
			}
		}

		if longestKey != "" {
			debugMeta.ModifiedImportPath = longestValue
			if tail := importPath[len(longestKey):]; tail != "/" {
				// Don't include the trailing characters if they are equal to a
				// single slash. This comes up because you can abuse this quirk of
				// node's path resolution to force node to load the package from the
				// file system instead of as a built-in module. For example, "util"
				// is node's built-in module while "util/" is one on the file system.
				// Leaving the trailing slash in place causes problems for people:
				// https://github.com/evanw/esbuild/issues/2730. It should be ok to
				// always strip the trailing slash even when using the alias feature
				// to swap one package for another (except when you swap a reference
				// to one built-in node module with another but really why would you
				// do that).
				debugMeta.ModifiedImportPath += tail
			}
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("  Matched with alias from %q to %q", longestKey, longestValue))
				r.debugLogs.addNote(fmt.Sprintf("  Modified import path from %q to %q", importPath, debugMeta.ModifiedImportPath))
			}
			importPath = debugMeta.ModifiedImportPath

			// Resolve the package using the current path instead of the original
			// path. This is trying to resolve the substitute in the top-level
			// package instead of the nested package, which lets the top-level
			// package control the version of the substitution. It's also critical
			// when using Yarn PnP because Yarn PnP doesn't allow nested packages
			// to "reach outside" of their normal dependency lists.
			sourceDir = r.fs.Cwd()
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("  Changed resolve directory to %q", sourceDir))
			}
		} else if r.debugLogs != nil {
			r.debugLogs.addNote("  Failed to find any package alias matches")
		}
	}

	// Certain types of URLs default to being external for convenience
	if isExplicitlyExternal := r.isExternal(r.options.ExternalSettings.PreResolve, importPath, kind); isExplicitlyExternal ||

		// "fill: url(#filter);"
		(kind == ast.ImportURL && strings.HasPrefix(importPath, "#")) ||

		// "background: url(http://example.com/images/image.png);"
		strings.HasPrefix(importPath, "http://") ||

		// "background: url(https://example.com/images/image.png);"
		strings.HasPrefix(importPath, "https://") ||

		// "background: url(//example.com/images/image.png);"
		strings.HasPrefix(importPath, "//") {

		if r.debugLogs != nil {
			if isExplicitlyExternal {
				r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", importPath))
			} else {
				r.debugLogs.addNote("Marking this path as implicitly external")
			}
		}

		r.flushDebugLogs(flushDueToSuccess)
		return &ResolveResult{
			PathPair: PathPair{Primary: logger.Path{Text: importPath}, IsExternal: true},
		}, debugMeta
	}

	if pathPair, ok, sideEffects := r.checkForBuiltInNodeModules(importPath); ok {
		r.flushDebugLogs(flushDueToSuccess)
		return &ResolveResult{
			PathPair:               pathPair,
			PrimarySideEffectsData: sideEffects,
		}, debugMeta
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
			}, debugMeta
		}

		// "background: url(data:image/png;base64,iVBORw0KGgo=);"
		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this data URL as external")
		}
		r.flushDebugLogs(flushDueToSuccess)
		return &ResolveResult{
			PathPair: PathPair{Primary: logger.Path{Text: importPath}, IsExternal: true},
		}, debugMeta
	}

	// Fail now if there is no directory to resolve in. This can happen for
	// virtual modules (e.g. stdin) if a resolve directory is not specified.
	if sourceDir == "" {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Cannot resolve this path without a directory")
		}
		r.flushDebugLogs(flushDueToFailure)
		return nil, debugMeta
	}

	// Glob imports only work in a multi-path context
	if strings.ContainsRune(importPath, '*') {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Cannot resolve a path containing a wildcard character in a single-path context")
		}
		r.flushDebugLogs(flushDueToFailure)
		return nil, debugMeta
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check for the Yarn PnP manifest if it hasn't already been checked for
	if !r.pnpManifestWasChecked {
		r.pnpManifestWasChecked = true

		// Use the current working directory to find the Yarn PnP manifest. We
		// can't necessarily use the entry point locations because the entry
		// point locations aren't necessarily file paths. For example, they could
		// be HTTP URLs that will be handled by a plugin.
		for dirInfo := r.dirInfoCached(r.fs.Cwd()); dirInfo != nil; dirInfo = dirInfo.parent {
			if absPath := dirInfo.pnpManifestAbsPath; absPath != "" {
				if strings.HasSuffix(absPath, ".json") {
					if json, source := r.extractYarnPnPDataFromJSON(absPath, pnpReportErrorsAboutMissingFiles); json.Data != nil {
						r.pnpManifest = compileYarnPnPData(absPath, r.fs.Dir(absPath), json, source)
					}
				} else {
					if json, source := r.tryToExtractYarnPnPDataFromJS(absPath, pnpReportErrorsAboutMissingFiles); json.Data != nil {
						r.pnpManifest = compileYarnPnPData(absPath, r.fs.Dir(absPath), json, source)
					}
				}
				if r.debugLogs != nil && r.pnpManifest != nil && r.pnpManifest.invalidIgnorePatternData != "" {
					r.debugLogs.addNote("  Invalid Go regular expression for \"ignorePatternData\": " + r.pnpManifest.invalidIgnorePatternData)
				}
				break
			}
		}
	}

	sourceDirInfo := r.dirInfoCached(sourceDir)
	if sourceDirInfo == nil {
		// Bail if the directory is missing for some reason
		return nil, debugMeta
	}

	result := r.resolveWithoutSymlinks(sourceDir, sourceDirInfo, importPath)
	if result == nil {
		// If resolution failed, try again with the URL query and/or hash removed
		suffix := strings.IndexAny(importPath, "?#")
		if suffix < 1 {
			r.flushDebugLogs(flushDueToFailure)
			return nil, debugMeta
		}
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Retrying resolution after removing the suffix %q", importPath[suffix:]))
		}
		if result2 := r.resolveWithoutSymlinks(sourceDir, sourceDirInfo, importPath[:suffix]); result2 == nil {
			r.flushDebugLogs(flushDueToFailure)
			return nil, debugMeta
		} else {
			result = result2
			result.PathPair.Primary.IgnoredSuffix = importPath[suffix:]
			if result.PathPair.HasSecondary() {
				result.PathPair.Secondary.IgnoredSuffix = importPath[suffix:]
			}
		}
	}

	// If successful, resolve symlinks using the directory info cache
	r.finalizeResolve(result)
	r.flushDebugLogs(flushDueToSuccess)
	return result, debugMeta
}

// This returns nil on failure and non-nil on success. Note that this may
// return an empty array to indicate a successful search that returned zero
// results.
func (res *Resolver) ResolveGlob(sourceDir string, importPathPattern []helpers.GlobPart, kind ast.ImportKind, prettyPattern string) (map[string]ResolveResult, *logger.Msg) {
	var debugMeta DebugMeta
	r := resolverQuery{
		Resolver:  res,
		debugMeta: &debugMeta,
		kind:      kind,
	}

	if r.log.Level <= logger.LevelDebug {
		r.debugLogs = &debugLogs{what: fmt.Sprintf(
			"Resolving glob import %s in directory %q of type %q",
			prettyPattern, sourceDir, kind.StringForMetafile())}
	}

	if len(importPathPattern) == 0 {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Ignoring empty glob pattern")
		}
		r.flushDebugLogs(flushDueToFailure)
		return nil, nil
	}
	firstPrefix := importPathPattern[0].Prefix

	// Glob patterns only work for relative URLs
	if !strings.HasPrefix(firstPrefix, "./") && !strings.HasPrefix(firstPrefix, "../") &&
		!strings.HasPrefix(firstPrefix, ".\\") && !strings.HasPrefix(firstPrefix, "..\\") {
		if kind == ast.ImportEntryPoint {
			// Be permissive about forgetting "./" for entry points since it's common
			// to omit "./" on the command line. But don't accidentally treat absolute
			// paths as relative (even on Windows).
			if !r.fs.IsAbs(firstPrefix) {
				firstPrefix = "./" + firstPrefix
			}
		} else {
			// Don't allow omitting "./" for other imports since node doesn't let you do this either
			if r.debugLogs != nil {
				r.debugLogs.addNote("Ignoring glob import that doesn't start with \"./\" or \"../\"")
			}
			r.flushDebugLogs(flushDueToFailure)
			return nil, nil
		}
	}

	// Handle leading directories in the pattern (including "../")
	dirPrefix := 0
	for {
		slash := strings.IndexAny(firstPrefix[dirPrefix:], "/\\")
		if slash == -1 {
			break
		}
		if star := strings.IndexByte(firstPrefix[dirPrefix:], '*'); star != -1 && slash > star {
			break
		}
		dirPrefix += slash + 1
	}

	// If the pattern is an absolute path, then just replace source directory.
	// Otherwise join the source directory with the prefix from the pattern.
	if suffix := firstPrefix[:dirPrefix]; r.fs.IsAbs(suffix) {
		sourceDir = suffix
	} else {
		sourceDir = r.fs.Join(sourceDir, suffix)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Look up the directory to start from
	sourceDirInfo := r.dirInfoCached(sourceDir)
	if sourceDirInfo == nil {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Failed to find the directory %q", sourceDir))
		}
		r.flushDebugLogs(flushDueToFailure)
		return nil, nil
	}

	// Turn the glob pattern into a regular expression
	canMatchOnSlash := false
	wasGlobStar := false
	sb := strings.Builder{}
	sb.WriteByte('^')
	for i, part := range importPathPattern {
		prefix := part.Prefix
		if i == 0 {
			prefix = firstPrefix
		}
		if wasGlobStar && len(prefix) > 0 && (prefix[0] == '/' || prefix[0] == '\\') {
			prefix = prefix[1:] // Move over the "/" after a globstar
		}
		sb.WriteString(regexp.QuoteMeta(prefix))
		switch part.Wildcard {
		case helpers.GlobAllIncludingSlash:
			// It's a globstar, so match zero or more path segments
			sb.WriteString("(?:[^/]*(?:/|$))*")
			canMatchOnSlash = true
			wasGlobStar = true
		case helpers.GlobAllExceptSlash:
			// It's not a globstar, so only match one path segment
			sb.WriteString("[^/]*")
			wasGlobStar = false
		}
	}
	sb.WriteByte('$')
	re := regexp.MustCompile(sb.String())

	// Initialize "results" to a non-nil value to indicate that the glob is valid
	results := make(map[string]ResolveResult)

	var visit func(dirInfo *dirInfo, dir string)
	visit = func(dirInfo *dirInfo, dir string) {
		for _, key := range dirInfo.entries.SortedKeys() {
			entry, _ := dirInfo.entries.Get(key)
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Considering entry %q", r.fs.Join(dirInfo.absPath, key)))
				r.debugLogs.increaseIndent()
			}

			switch entry.Kind(r.fs) {
			case fs.DirEntry:
				// To avoid infinite loops, don't follow any symlinks
				if canMatchOnSlash && entry.Symlink(r.fs) == "" {
					if childDirInfo := r.dirInfoCached(r.fs.Join(dirInfo.absPath, key)); childDirInfo != nil {
						visit(childDirInfo, fmt.Sprintf("%s%s/", dir, key))
					}
				}

			case fs.FileEntry:
				if relPath := dir + key; re.MatchString(relPath) {
					var result ResolveResult

					if r.isExternal(r.options.ExternalSettings.PreResolve, relPath, kind) {
						result.PathPair = PathPair{Primary: logger.Path{Text: relPath}, IsExternal: true}

						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", result.PathPair.Primary.Text))
						}
					} else {
						absPath := r.fs.Join(dirInfo.absPath, key)
						result.PathPair = PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}
					}

					r.finalizeResolve(&result)
					results[relPath] = result
				}
			}

			if r.debugLogs != nil {
				r.debugLogs.decreaseIndent()
			}
		}
	}

	visit(sourceDirInfo, firstPrefix[:dirPrefix])

	var warning *logger.Msg
	if len(results) == 0 {
		warning = &logger.Msg{
			ID:   logger.MsgID_Bundler_EmptyGlob,
			Kind: logger.Warning,
			Data: logger.MsgData{Text: fmt.Sprintf("The glob pattern %s did not match any files", prettyPattern)},
		}
	}

	r.flushDebugLogs(flushDueToSuccess)
	return results, warning
}

func (r resolverQuery) isExternal(matchers config.ExternalMatchers, path string, kind ast.ImportKind) bool {
	if kind == ast.ImportEntryPoint {
		// Never mark an entry point as external. This is not useful.
		return false
	}
	if _, ok := matchers.Exact[path]; ok {
		return true
	}
	for _, pattern := range matchers.Patterns {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Checking %q against the external pattern %q", path, pattern.Prefix+"*"+pattern.Suffix))
		}
		if len(path) >= len(pattern.Prefix)+len(pattern.Suffix) &&
			strings.HasPrefix(path, pattern.Prefix) &&
			strings.HasSuffix(path, pattern.Suffix) {
			return true
		}
	}
	return false
}

// This tries to run "Resolve" on a package path as a relative path. If
// successful, the user just forgot a leading "./" in front of the path.
func (res *Resolver) ProbeResolvePackageAsRelative(sourceDir string, importPath string, kind ast.ImportKind) (*ResolveResult, DebugMeta) {
	var debugMeta DebugMeta
	r := resolverQuery{
		Resolver:  res,
		debugMeta: &debugMeta,
		kind:      kind,
	}
	absPath := r.fs.Join(sourceDir, importPath)

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if pair, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
		result := &ResolveResult{PathPair: pair, DifferentCase: diffCase}
		r.finalizeResolve(result)
		r.flushDebugLogs(flushDueToSuccess)
		return result, debugMeta
	}

	return nil, debugMeta
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
	d.notes = append(d.notes, logger.MsgData{Text: text, DisableMaximumWidth: true})
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
			r.log.AddIDWithNotes(logger.MsgID_None, logger.Debug, nil, logger.Range{}, r.debugLogs.what, r.debugLogs.notes)
		} else if r.log.Level <= logger.LevelVerbose {
			r.log.AddIDWithNotes(logger.MsgID_None, logger.Verbose, nil, logger.Range{}, r.debugLogs.what, r.debugLogs.notes)
		}
	}
}

func (r resolverQuery) finalizeResolve(result *ResolveResult) {
	if !result.PathPair.IsExternal && r.isExternal(r.options.ExternalSettings.PostResolve, result.PathPair.Primary.Text, r.kind) {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", result.PathPair.Primary.Text))
		}
		result.PathPair.IsExternal = true
	} else {
		for i, path := range result.PathPair.iter() {
			if path.Namespace != "file" {
				continue
			}
			dirInfo := r.dirInfoCached(r.fs.Dir(path.Text))
			if dirInfo == nil {
				continue
			}
			base := r.fs.Base(path.Text)

			// If the path contains symlinks, rewrite the path to the real path
			if !r.options.PreserveSymlinks {
				if entry, _ := dirInfo.entries.Get(base); entry != nil {
					symlink := entry.Symlink(r.fs)
					if symlink != "" {
						// This means the entry itself is a symlink
					} else if dirInfo.absRealPath != "" {
						// There is at least one parent directory with a symlink
						symlink = r.fs.Join(dirInfo.absRealPath, base)
					}
					if symlink != "" {
						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("Resolved symlink %q to %q", path.Text, symlink))
						}
						path.Text = symlink

						// Look up the directory over again if it was changed
						dirInfo = r.dirInfoCached(r.fs.Dir(path.Text))
						if dirInfo == nil {
							continue
						}
						base = r.fs.Base(path.Text)
					}
				}
			}

			// Path attributes are only taken from the primary path
			if i > 0 {
				continue
			}

			// Path attributes are not taken from disabled files
			if path.IsDisabled() {
				continue
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
			if pkgJSON := dirInfo.enclosingPackageJSON; pkgJSON != nil {
				if pkgJSON.sideEffectsMap != nil {
					hasSideEffects := false
					pathLookup := strings.ReplaceAll(path.Text, "\\", "/") // Avoid problems with Windows-style slashes
					if pkgJSON.sideEffectsMap[pathLookup] {
						// Fast path: map lookup
						hasSideEffects = true
					} else {
						// Slow path: glob tests
						for _, re := range pkgJSON.sideEffectsRegexps {
							if re.MatchString(pathLookup) {
								hasSideEffects = true
								break
							}
						}
					}
					if !hasSideEffects {
						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("Marking this file as having no side effects due to %q",
								pkgJSON.source.KeyPath.Text))
						}
						result.PrimarySideEffectsData = pkgJSON.sideEffectsData
					}
				}

				// Also copy over the "type" field
				result.ModuleTypeData = pkgJSON.moduleTypeData
			}

			// Copy various fields from the nearest enclosing "tsconfig.json" file if present
			if tsConfigJSON := r.tsConfigForDir(dirInfo); tsConfigJSON != nil {
				result.TSConfig = &tsConfigJSON.Settings
				result.TSConfigJSX = tsConfigJSON.JSXSettings
				result.TSAlwaysStrict = tsConfigJSON.TSAlwaysStrictOrStrict()

				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("This import is under the effect of %q",
						tsConfigJSON.AbsPath))
					if result.TSConfigJSX.JSXFactory != nil {
						r.debugLogs.addNote(fmt.Sprintf("\"jsxFactory\" is %q due to %q",
							strings.Join(result.TSConfigJSX.JSXFactory, "."),
							tsConfigJSON.AbsPath))
					}
					if result.TSConfigJSX.JSXFragmentFactory != nil {
						r.debugLogs.addNote(fmt.Sprintf("\"jsxFragment\" is %q due to %q",
							strings.Join(result.TSConfigJSX.JSXFragmentFactory, "."),
							tsConfigJSON.AbsPath))
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

func (r resolverQuery) resolveWithoutSymlinks(sourceDir string, sourceDirInfo *dirInfo, importPath string) *ResolveResult {
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
		if tsConfigJSON := r.tsConfigForDir(sourceDirInfo); tsConfigJSON != nil && tsConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(tsConfigJSON, importPath); ok {
				return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}
			}
		}

		// Run node's resolution rules (e.g. adding ".js")
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(importPath); ok {
			return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}
		} else {
			return nil
		}
	}

	// Check both relative and package paths for CSS URL tokens, with relative
	// paths taking precedence over package paths to match Webpack behavior.
	isPackagePath := IsPackagePath(importPath)
	checkRelative := !isPackagePath || r.kind.IsFromCSS()
	checkPackage := isPackagePath

	if checkRelative {
		absPath := r.fs.Join(sourceDir, importPath)

		// Check for external packages first
		if r.isExternal(r.options.ExternalSettings.PostResolve, absPath, r.kind) {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", absPath))
			}
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}, IsExternal: true}}
		}

		// Check the "browser" map
		if importDirInfo := r.dirInfoCached(r.fs.Dir(absPath)); importDirInfo != nil {
			if remapped, ok := r.checkBrowserMap(importDirInfo, absPath, absolutePathKind); ok {
				if remapped == nil {
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}}
				}
				if remappedResult, ok, diffCase, sideEffects := r.resolveWithoutRemapping(importDirInfo.enclosingBrowserScope, *remapped); ok {
					result = ResolveResult{PathPair: remappedResult, DifferentCase: diffCase, PrimarySideEffectsData: sideEffects}
					checkRelative = false
					checkPackage = false
				}
			}
		}

		if checkRelative {
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
				checkPackage = false
				result = ResolveResult{PathPair: absolute, DifferentCase: diffCase}
			} else if !checkPackage {
				return nil
			}
		}
	}

	if checkPackage {
		// Support remapping one package path to another via the "browser" field
		if remapped, ok := r.checkBrowserMap(sourceDirInfo, importPath, packagePathKind); ok {
			if remapped == nil {
				// "browser": {"module": false}
				if absolute, ok, diffCase, sideEffects := r.loadNodeModules(importPath, sourceDirInfo, false /* forbidImports */); ok {
					absolute.Primary = logger.Path{Text: absolute.Primary.Text, Namespace: "file", Flags: logger.PathDisabled}
					if absolute.HasSecondary() {
						absolute.Secondary = logger.Path{Text: absolute.Secondary.Text, Namespace: "file", Flags: logger.PathDisabled}
					}
					return &ResolveResult{PathPair: absolute, DifferentCase: diffCase, PrimarySideEffectsData: sideEffects}
				} else {
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Flags: logger.PathDisabled}}, DifferentCase: diffCase}
				}
			}

			// "browser": {"module": "./some-file"}
			// "browser": {"module": "another-module"}
			importPath = *remapped
			sourceDirInfo = sourceDirInfo.enclosingBrowserScope
		}

		if absolute, ok, diffCase, sideEffects := r.resolveWithoutRemapping(sourceDirInfo, importPath); ok {
			result = ResolveResult{PathPair: absolute, DifferentCase: diffCase, PrimarySideEffectsData: sideEffects}
		} else {
			// Note: node's "self references" are not currently supported
			return nil
		}
	}

	return &result
}

func (r resolverQuery) resolveWithoutRemapping(sourceDirInfo *dirInfo, importPath string) (PathPair, bool, *fs.DifferentCase, *SideEffectsData) {
	if IsPackagePath(importPath) {
		return r.loadNodeModules(importPath, sourceDirInfo, false /* forbidImports */)
	} else {
		absolute, ok, diffCase := r.loadAsFileOrDirectory(r.fs.Join(sourceDirInfo.absPath, importPath))
		return absolute, ok, diffCase, nil
	}
}

func PrettyPath(fs fs.FS, path logger.Path) string {
	if path.Namespace == "file" {
		if rel, ok := fs.Rel(fs.Cwd(), path.Text); ok {
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
	absPath               string
	pnpManifestAbsPath    string
	entries               fs.DirEntries
	packageJSON           *packageJSON  // Is there a "package.json" file in this directory?
	enclosingPackageJSON  *packageJSON  // Is there a "package.json" file in this directory or a parent directory?
	enclosingTSConfigJSON *TSConfigJSON // Is there a "tsconfig.json" file in this directory or a parent directory?
	absRealPath           string        // If non-empty, this is the real absolute path resolving any symlinks
	isNodeModules         bool          // Is the base name "node_modules"?
	hasNodeModules        bool          // Is there a "node_modules" subdirectory?
	isInsideNodeModules   bool          // Is this within a  "node_modules" subtree?
}

func (r resolverQuery) tsConfigForDir(dirInfo *dirInfo) *TSConfigJSON {
	if dirInfo.isInsideNodeModules {
		return nil
	}
	if r.tsConfigOverride != nil {
		return r.tsConfigOverride
	}
	if dirInfo != nil {
		return dirInfo.enclosingTSConfigJSON
	}
	return nil
}

func (r resolverQuery) dirInfoCached(path string) *dirInfo {
	// First, check the cache
	cached, ok := r.dirCache[path]

	// Cache hit: stop now
	if !ok {
		// Update the cache to indicate failure. Even if the read failed, we don't
		// want to retry again later. The directory is inaccessible so trying again
		// is wasted. Doing this before calling "dirInfoUncached" prevents stack
		// overflow in case this directory is recursively encountered again.
		r.dirCache[path] = nil

		// Cache miss: read the info
		cached = r.dirInfoUncached(path)

		// Only update the cache again on success
		if cached != nil {
			r.dirCache[path] = cached
		}
	}

	if r.debugLogs != nil {
		if cached == nil {
			r.debugLogs.addNote(fmt.Sprintf("Failed to read directory %q", path))
		} else {
			count := cached.entries.PeekEntryCount()
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
func (r resolverQuery) parseTSConfig(file string, visited map[string]bool, configDir string) (*TSConfigJSON, error) {
	// Resolve any symlinks first before parsing the file
	if !r.options.PreserveSymlinks {
		if real, ok := r.fs.EvalSymlinks(file); ok {
			file = real
		}
	}

	// Don't infinite loop if a series of "extends" links forms a cycle
	if visited[file] {
		return nil, errParseErrorImportCycle
	}

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
		PrettyPath: PrettyPath(r.fs, keyPath),
		Contents:   contents,
	}
	if visited != nil {
		// This is only non-nil for "build" API calls. This is nil for "transform"
		// API calls, which tells us to not process "extends" fields.
		visited[file] = true
	}
	result, err := r.parseTSConfigFromSource(source, visited, configDir)
	if visited != nil {
		// Reset this to back false in case something uses TypeScript 5.0's multiple
		// inheritance feature for "tsconfig.json" files. It should be valid to visit
		// the same base "tsconfig.json" file multiple times from different multiple
		// inheritance subtrees.
		visited[file] = false
	}
	return result, err
}

func (r resolverQuery) parseTSConfigFromSource(source logger.Source, visited map[string]bool, configDir string) (*TSConfigJSON, error) {
	tracker := logger.MakeLineColumnTracker(&source)
	fileDir := r.fs.Dir(source.KeyPath.Text)
	isExtends := len(visited) > 1

	result := ParseTSConfigJSON(r.log, source, &r.caches.JSONCache, r.fs, fileDir, configDir, func(extends string, extendsRange logger.Range) *TSConfigJSON {
		if visited == nil {
			// If this is nil, then we're in a "transform" API call. In that case we
			// deliberately skip processing "extends" fields. This is because the
			// "transform" API is supposed to be without a file system.
			return nil
		}

		// Note: This doesn't use the normal node module resolution algorithm
		// both because it's different (e.g. we don't want to match a directory)
		// and because it would deadlock since we're currently in the middle of
		// populating the directory info cache.

		maybeFinishOurSearch := func(base *TSConfigJSON, err error, extendsFile string) (*TSConfigJSON, bool) {
			if err == nil {
				return base, true
			}

			if err == syscall.ENOENT {
				// Return false to indicate that we should continue searching
				return nil, false
			}

			if err == errParseErrorImportCycle {
				r.log.AddID(logger.MsgID_TSConfigJSON_Cycle, logger.Warning, &tracker, extendsRange,
					fmt.Sprintf("Base config file %q forms cycle", extends))
			} else if err != errParseErrorAlreadyLogged {
				r.log.AddError(&tracker, extendsRange,
					fmt.Sprintf("Cannot read file %q: %s",
						PrettyPath(r.fs, logger.Path{Text: extendsFile, Namespace: "file"}), err.Error()))
			}
			return nil, true
		}

		// Check for a Yarn PnP manifest and use that to rewrite the path
		if IsPackagePath(extends) {
			pnpData := r.pnpManifest

			// If we haven't loaded the Yarn PnP manifest yet, try to find one
			if pnpData == nil {
				current := fileDir
				for {
					if _, _, ok := fs.ParseYarnPnPVirtualPath(current); !ok {
						absPath := r.fs.Join(current, ".pnp.data.json")
						if json, source := r.extractYarnPnPDataFromJSON(absPath, pnpIgnoreErrorsAboutMissingFiles); json.Data != nil {
							pnpData = compileYarnPnPData(absPath, current, json, source)
							break
						}

						absPath = r.fs.Join(current, ".pnp.cjs")
						if json, source := r.tryToExtractYarnPnPDataFromJS(absPath, pnpIgnoreErrorsAboutMissingFiles); json.Data != nil {
							pnpData = compileYarnPnPData(absPath, current, json, source)
							break
						}

						absPath = r.fs.Join(current, ".pnp.js")
						if json, source := r.tryToExtractYarnPnPDataFromJS(absPath, pnpIgnoreErrorsAboutMissingFiles); json.Data != nil {
							pnpData = compileYarnPnPData(absPath, current, json, source)
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
			}

			if pnpData != nil {
				if result := r.resolveToUnqualified(extends, fileDir, pnpData); result.status == pnpErrorGeneric {
					if r.debugLogs != nil {
						r.debugLogs.addNote("The Yarn PnP path resolution algorithm returned an error")
					}
					goto pnpError
				} else if result.status == pnpSuccess {
					// If Yarn PnP path resolution succeeded, run a custom abbreviated
					// version of node's module resolution algorithm. The Yarn PnP
					// specification says to use node's module resolution algorithm verbatim
					// but that isn't what Yarn actually does. See this for more info:
					// https://github.com/evanw/esbuild/issues/2473#issuecomment-1216774461
					if entries, _, dirErr := r.fs.ReadDirectory(result.pkgDirPath); dirErr == nil {
						if entry, _ := entries.Get("package.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
							// Check the "exports" map
							if packageJSON := r.parsePackageJSON(result.pkgDirPath); packageJSON != nil && packageJSON.exportsMap != nil {
								if absolute, ok, _ := r.esmResolveAlgorithm(finalizeImportsExportsYarnPnPTSConfigExtends,
									result.pkgIdent, "."+result.pkgSubpath, packageJSON, result.pkgDirPath, source.KeyPath.Text); ok {
									base, err := r.parseTSConfig(absolute.Primary.Text, visited, configDir)
									if result, shouldReturn := maybeFinishOurSearch(base, err, absolute.Primary.Text); shouldReturn {
										return result
									}
								}
								goto pnpError
							}
						}
					}

					// Continue with the module resolution algorithm from node.js
					extends = r.fs.Join(result.pkgDirPath, result.pkgSubpath)
				}
			}
		}

		if IsPackagePath(extends) && !r.fs.IsAbs(extends) {
			esmPackageName, esmPackageSubpath, esmOK := esmParsePackageName(extends)
			if r.debugLogs != nil && esmOK {
				r.debugLogs.addNote(fmt.Sprintf("Parsed tsconfig package name %q and package subpath %q", esmPackageName, esmPackageSubpath))
			}

			// If this is still a package path, try to resolve it to a "node_modules" directory
			current := fileDir
			for {
				// Skip "node_modules" folders
				if r.fs.Base(current) != "node_modules" {
					join := r.fs.Join(current, "node_modules", extends)

					// Check to see if "package.json" exists
					pkgDir := r.fs.Join(current, "node_modules", esmPackageName)
					pjFile := r.fs.Join(pkgDir, "package.json")
					if _, err, originalError := r.fs.ReadFile(pjFile); err == nil {
						if packageJSON := r.parsePackageJSON(pkgDir); packageJSON != nil {
							// Try checking the "tsconfig" field of "package.json". The ability to use "extends" like this was added in TypeScript 3.2:
							// https://www.typescriptlang.org/docs/handbook/release-notes/typescript-3-2.html#tsconfigjson-inheritance-via-nodejs-packages
							if packageJSON.tsconfig != "" {
								join = packageJSON.tsconfig
								if !r.fs.IsAbs(join) {
									join = r.fs.Join(pkgDir, join)
								}
							}

							// Try checking the "exports" map. The ability to use "extends" like this was added in TypeScript 5.0:
							// https://devblogs.microsoft.com/typescript/announcing-typescript-5-0/
							if packageJSON.exportsMap != nil {
								if r.debugLogs != nil {
									r.debugLogs.addNote(fmt.Sprintf("Looking for %q in \"exports\" map in %q", esmPackageSubpath, packageJSON.source.KeyPath.Text))
									r.debugLogs.increaseIndent()
									defer r.debugLogs.decreaseIndent()
								}

								// Note: TypeScript appears to always treat this as a "require" import
								conditions := r.esmConditionsRequire
								resolvedPath, status, debug := r.esmPackageExportsResolve("/", esmPackageSubpath, packageJSON.exportsMap.root, conditions)
								resolvedPath, status, debug = r.esmHandlePostConditions(resolvedPath, status, debug)

								// This is a very abbreviated version of our ESM resolution
								if status == pjStatusExact || status == pjStatusExactEndsWithStar {
									fileToCheck := r.fs.Join(pkgDir, resolvedPath)
									base, err := r.parseTSConfig(fileToCheck, visited, configDir)

									if result, shouldReturn := maybeFinishOurSearch(base, err, fileToCheck); shouldReturn {
										return result
									}
								}
							}
						}
					} else if r.debugLogs != nil && originalError != nil {
						r.debugLogs.addNote(fmt.Sprintf("Failed to read file %q: %s", pjFile, originalError.Error()))
					}

					filesToCheck := []string{r.fs.Join(join, "tsconfig.json"), join, join + ".json"}
					for _, fileToCheck := range filesToCheck {
						base, err := r.parseTSConfig(fileToCheck, visited, configDir)

						// Explicitly ignore matches if they are directories instead of files
						if err != nil && err != syscall.ENOENT {
							if entries, _, dirErr := r.fs.ReadDirectory(r.fs.Dir(fileToCheck)); dirErr == nil {
								if entry, _ := entries.Get(r.fs.Base(fileToCheck)); entry != nil && entry.Kind(r.fs) == fs.DirEntry {
									continue
								}
							}
						}

						if result, shouldReturn := maybeFinishOurSearch(base, err, fileToCheck); shouldReturn {
							return result
						}
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
			extendsFile := extends

			// The TypeScript compiler has a strange behavior that seems like a bug
			// where "." and ".." behave differently than other forms such as "./."
			// or "../." and are interpreted as having an implicit "tsconfig.json"
			// suffix.
			//
			// I believe their bug is caused by some parts of their code checking for
			// relative paths using the literal "./" and "../" prefixes (requiring
			// the slash) and other parts checking using the regular expression
			// /^\.\.?($|[\\/])/ (with the slash optional).
			//
			// In any case, people are now relying on this behavior. One example is
			// this: https://github.com/esbuild-kit/tsx/pull/158. So we replicate this
			// bug in esbuild as well.
			if extendsFile == "." || extendsFile == ".." {
				extendsFile += "/tsconfig.json"
			}

			// If this is a regular path, search relative to the enclosing directory
			if !r.fs.IsAbs(extendsFile) {
				extendsFile = r.fs.Join(fileDir, extendsFile)
			}
			base, err := r.parseTSConfig(extendsFile, visited, configDir)

			// TypeScript's handling of "extends" has some specific edge cases. We
			// must only try adding ".json" if it's not already present, which is
			// unlike how node path resolution works. We also need to explicitly
			// ignore matches if they are directories instead of files. Some users
			// name directories the same name as their config files.
			if err != nil && !strings.HasSuffix(extendsFile, ".json") {
				if entries, _, dirErr := r.fs.ReadDirectory(r.fs.Dir(extendsFile)); dirErr == nil {
					extendsBase := r.fs.Base(extendsFile)
					if entry, _ := entries.Get(extendsBase); entry == nil || entry.Kind(r.fs) != fs.FileEntry {
						if entry, _ := entries.Get(extendsBase + ".json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
							base, err = r.parseTSConfig(extendsFile+".json", visited, configDir)
						}
					}
				}
			}

			if result, shouldReturn := maybeFinishOurSearch(base, err, extendsFile); shouldReturn {
				return result
			}
		}

		// Suppress warnings about missing base config files inside "node_modules"
	pnpError:
		if !helpers.IsInsideNodeModules(source.KeyPath.Text) {
			var notes []logger.MsgData
			if r.debugLogs != nil {
				notes = r.debugLogs.notes
			}
			r.log.AddIDWithNotes(logger.MsgID_TSConfigJSON_Missing, logger.Warning, &tracker, extendsRange,
				fmt.Sprintf("Cannot find base config file %q", extends), notes)
		}

		return nil
	})

	if result == nil {
		return nil, errParseErrorAlreadyLogged
	}

	// Now that we have parsed the entire "tsconfig.json" file, filter out any
	// paths that are invalid due to being a package-style path without a base
	// URL specified. This must be done here instead of when we're parsing the
	// original file because TypeScript allows one "tsconfig.json" file to
	// specify "baseUrl" and inherit a "paths" from another file via "extends".
	if !isExtends && result.Paths != nil && result.BaseURL == nil {
		var tracker *logger.LineColumnTracker
		for key, paths := range result.Paths.Map {
			end := 0
			for _, path := range paths {
				if isValidTSConfigPathNoBaseURLPattern(path.Text, r.log, &result.Paths.Source, &tracker, path.Loc) {
					paths[end] = path
					end++
				}
			}
			if end < len(paths) {
				result.Paths.Map[key] = paths[:end]
			}
		}
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
	if err == syscall.EACCES || err == syscall.EPERM {
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
			r.log.AddError(nil, logger.Range{},
				fmt.Sprintf("Cannot read directory %q: %s",
					PrettyPath(r.fs, logger.Path{Text: path, Namespace: "file"}), err.Error()))
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
	if base == "node_modules" {
		info.isNodeModules = true
		info.isInsideNodeModules = true
	} else if entry, _ := entries.Get("node_modules"); entry != nil {
		info.hasNodeModules = entry.Kind(r.fs) == fs.DirEntry
	}

	// Propagate the browser scope into child directories
	if parentInfo != nil {
		info.enclosingPackageJSON = parentInfo.enclosingPackageJSON
		info.enclosingBrowserScope = parentInfo.enclosingBrowserScope
		info.enclosingTSConfigJSON = parentInfo.enclosingTSConfigJSON
		if parentInfo.isInsideNodeModules {
			info.isInsideNodeModules = true
		}

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

		// Propagate this "package.json" file into child directories
		if info.packageJSON != nil {
			info.enclosingPackageJSON = info.packageJSON
			if info.packageJSON.browserMap != nil {
				info.enclosingBrowserScope = info
			}
		}
	}

	// Record if this directory has a tsconfig.json or jsconfig.json file
	if r.tsConfigOverride == nil {
		var tsConfigPath string
		if entry, _ := entries.Get("tsconfig.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			tsConfigPath = r.fs.Join(path, "tsconfig.json")
		} else if entry, _ := entries.Get("jsconfig.json"); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			tsConfigPath = r.fs.Join(path, "jsconfig.json")
		}

		// Except don't do this if we're inside a "node_modules" directory. Package
		// authors often publish their "tsconfig.json" files to npm because of
		// npm's default-include publishing model and because these authors
		// probably don't know about ".npmignore" files.
		//
		// People trying to use these packages with esbuild have historically
		// complained that esbuild is respecting "tsconfig.json" in these cases.
		// The assumption is that the package author published these files by
		// accident.
		//
		// Ignoring "tsconfig.json" files inside "node_modules" directories breaks
		// the use case of publishing TypeScript code and having it be transpiled
		// for you, but that's the uncommon case and likely doesn't work with
		// many other tools anyway. So now these files are ignored.
		if tsConfigPath != "" && !info.isInsideNodeModules {
			var err error
			info.enclosingTSConfigJSON, err = r.parseTSConfig(tsConfigPath, make(map[string]bool), r.fs.Dir(tsConfigPath))
			if err != nil {
				if err == syscall.ENOENT {
					r.log.AddError(nil, logger.Range{}, fmt.Sprintf("Cannot find tsconfig file %q",
						PrettyPath(r.fs, logger.Path{Text: tsConfigPath, Namespace: "file"})))
				} else if err != errParseErrorAlreadyLogged {
					r.log.AddID(logger.MsgID_TSConfigJSON_Missing, logger.Debug, nil, logger.Range{},
						fmt.Sprintf("Cannot read file %q: %s",
							PrettyPath(r.fs, logger.Path{Text: tsConfigPath, Namespace: "file"}), err.Error()))
				}
			}
		}
	}

	// Record if this directory has a Yarn PnP manifest. This must not be done
	// for Yarn virtual paths because that will result in duplicate copies of
	// the same manifest which will result in multiple copies of the same virtual
	// directory in the same path, which we don't handle (and which also doesn't
	// match Yarn's behavior).
	//
	// For example, imagine a project with a manifest here:
	//
	//   /project/.pnp.cjs
	//
	// and a source file with an import of "bar" here:
	//
	//   /project/.yarn/__virtual__/pkg/1/foo.js
	//
	// If we didn't ignore Yarn PnP manifests in virtual folders, then we would
	// pick up on the one here:
	//
	//   /project/.yarn/__virtual__/pkg/1/.pnp.cjs
	//
	// which means we would potentially resolve the import to something like this:
	//
	//   /project/.yarn/__virtual__/pkg/1/.yarn/__virtual__/pkg/1/bar
	//
	if r.pnpManifest == nil {
		if _, _, ok := fs.ParseYarnPnPVirtualPath(path); !ok {
			if pnp, _ := entries.Get(".pnp.data.json"); pnp != nil && pnp.Kind(r.fs) == fs.FileEntry {
				info.pnpManifestAbsPath = r.fs.Join(path, ".pnp.data.json")
			} else if pnp, _ := entries.Get(".pnp.cjs"); pnp != nil && pnp.Kind(r.fs) == fs.FileEntry {
				info.pnpManifestAbsPath = r.fs.Join(path, ".pnp.cjs")
			} else if pnp, _ := entries.Get(".pnp.js"); pnp != nil && pnp.Kind(r.fs) == fs.FileEntry {
				info.pnpManifestAbsPath = r.fs.Join(path, ".pnp.js")
			}
		}
	}

	return info
}

// TypeScript-specific behavior: if the extension is ".js" or ".jsx", try
// replacing it with ".ts" or ".tsx". At the time of writing this specific
// behavior comes from the function "loadModuleFromFile()" in the file
// "moduleNameResolver.ts" in the TypeScript compiler source code. It
// contains this comment:
//
//	If that didn't work, try stripping a ".js" or ".jsx" extension and
//	replacing it with a TypeScript one; e.g. "./foo.js" can be matched
//	by "./foo.ts" or "./foo.d.ts"
//
// We don't care about ".d.ts" files because we can't do anything with
// those, so we ignore that part of the behavior.
//
// See the discussion here for more historical context:
// https://github.com/microsoft/TypeScript/issues/4595
var rewrittenFileExtensions = map[string][]string{
	// Note that the official compiler code always tries ".ts" before
	// ".tsx" even if the original extension was ".jsx".
	".js":  {".ts", ".tsx"},
	".jsx": {".ts", ".tsx"},
	".mjs": {".mts"},
	".cjs": {".cts"},
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
			r.log.AddError(nil, logger.Range{},
				fmt.Sprintf("Cannot read directory %q: %s",
					PrettyPath(r.fs, logger.Path{Text: dirPath, Namespace: "file"}), err.Error()))
		}
		return "", false, nil
	}

	tryFile := func(base string) (string, bool, *fs.DifferentCase) {
		baseWithSuffix := base
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Checking for file %q", baseWithSuffix))
		}
		if entry, diffCase := entries.Get(baseWithSuffix); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found file %q", baseWithSuffix))
			}
			return r.fs.Join(dirPath, baseWithSuffix), true, diffCase
		}
		return "", false, nil
	}

	base := r.fs.Base(path)

	// Given "./x.js", node's algorithm tries things in the following order:
	//
	//   ./x.js
	//   ./x.js.js
	//   ./x.js.json
	//   ./x.js.node
	//   ./x.js/index.js
	//   ./x.js/index.json
	//   ./x.js/index.node
	//
	// Given "./x.js", TypeScript's algorithm tries things in the following order:
	//
	//   ./x.js.ts
	//   ./x.js.tsx
	//   ./x.js.d.ts
	//   ./x.ts
	//   ./x.tsx
	//   ./x.d.ts
	//   ./x.js/index.ts
	//   ./x.js/index.tsx
	//   ./x.js/index.d.ts
	//   ./x.js.js
	//   ./x.js.jsx
	//   ./x.js
	//   ./x.jsx
	//   ./x.js/index.js
	//   ./x.js/index.jsx
	//
	// Our order below is a blend of both. We try to follow node's algorithm but
	// with the features of TypeScript's algorithm (omitting ".d.ts" files, which
	// don't contain code). This means we should end up checking the same files
	// as TypeScript, but in a different order.
	//
	// One reason we use a different order is because we support a customizable
	// extension resolution order, which doesn't fit well into TypeScript's
	// algorithm. For example, you can configure esbuild to check for extensions
	// in the order ".js,.ts,.jsx,.tsx" but TypeScript always checks TypeScript
	// extensions before JavaScript extensions, so we can't obey the user's
	// intent if we follow TypeScript's algorithm exactly.
	//
	// Another reason we deviate from TypeScript's order is because our code is
	// structured to handle node's algorithm and TypeScript's algorithm has a
	// different structure. It intermixes multiple calls to LOAD_AS_FILE and
	// LOAD_INDEX together while node always does one LOAD_AS_FILE before one
	// LOAD_INDEX.

	// Try the plain path without any extensions
	if absolute, ok, diffCase := tryFile(base); ok {
		return absolute, ok, diffCase
	}

	// Try the path with extensions
	for _, ext := range extensionOrder {
		if absolute, ok, diffCase := tryFile(base + ext); ok {
			return absolute, ok, diffCase
		}
	}

	// TypeScript-specific behavior: try rewriting ".js" to ".ts"
	for old, exts := range rewrittenFileExtensions {
		if !strings.HasSuffix(base, old) {
			continue
		}
		lastDot := strings.LastIndexByte(base, '.')
		for _, ext := range exts {
			if absolute, ok, diffCase := tryFile(base[:lastDot] + ext); ok {
				return absolute, ok, diffCase
			}
		}
		break
	}

	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to find file %q", base))
	}
	return "", false, nil
}

func (r resolverQuery) loadAsIndex(dirInfo *dirInfo, extensionOrder []string) (PathPair, bool, *fs.DifferentCase) {
	// Try the "index" file with extensions
	for _, ext := range extensionOrder {
		base := "index" + ext
		if entry, diffCase := dirInfo.entries.Get(base); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found file %q", r.fs.Join(dirInfo.absPath, base)))
			}
			return PathPair{Primary: logger.Path{Text: r.fs.Join(dirInfo.absPath, base), Namespace: "file"}}, true, diffCase
		}
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Failed to find file %q", r.fs.Join(dirInfo.absPath, base)))
		}
	}

	return PathPair{}, false, nil
}

func (r resolverQuery) loadAsIndexWithBrowserRemapping(dirInfo *dirInfo, path string, extensionOrder []string) (PathPair, bool, *fs.DifferentCase) {
	// Potentially remap using the "browser" field
	absPath := r.fs.Join(path, "index")
	if remapped, ok := r.checkBrowserMap(dirInfo, absPath, absolutePathKind); ok {
		if remapped == nil {
			return PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil
		}
		remappedAbs := r.fs.Join(path, *remapped)

		// Is this a file?
		absolute, ok, diffCase := r.loadAsFile(remappedAbs, extensionOrder)
		if ok {
			return PathPair{Primary: logger.Path{Text: absolute, Namespace: "file"}}, true, diffCase
		}

		// Is it a directory with an index?
		if fieldDirInfo := r.dirInfoCached(remappedAbs); fieldDirInfo != nil {
			if absolute, ok, _ := r.loadAsIndex(fieldDirInfo, extensionOrder); ok {
				return absolute, true, nil
			}
		}

		return PathPair{}, false, nil
	}

	return r.loadAsIndex(dirInfo, extensionOrder)
}

func getProperty(json js_ast.Expr, name string) (js_ast.Expr, logger.Loc, bool) {
	if obj, ok := json.Data.(*js_ast.EObject); ok {
		for _, prop := range obj.Properties {
			if key, ok := prop.Key.Data.(*js_ast.EString); ok && key.Value != nil && helpers.UTF16EqualsString(key.Value, name) {
				return prop.ValueOrNil, prop.Key.Loc, true
			}
		}
	}
	return js_ast.Expr{}, logger.Loc{}, false
}

func getString(json js_ast.Expr) (string, bool) {
	if value, ok := json.Data.(*js_ast.EString); ok {
		return helpers.UTF16ToString(value.Value), true
	}
	return "", false
}

func getBool(json js_ast.Expr) (bool, bool) {
	if value, ok := json.Data.(*js_ast.EBoolean); ok {
		return value.Value, true
	}
	return false, false
}

func (r resolverQuery) loadAsFileOrDirectory(path string) (PathPair, bool, *fs.DifferentCase) {
	extensionOrder := r.options.ExtensionOrder
	if r.kind.MustResolveToCSS() {
		// Use a special import order for CSS "@import" imports
		extensionOrder = r.cssExtensionOrder
	} else if helpers.IsInsideNodeModules(path) {
		// Use a special import order for imports inside "node_modules"
		extensionOrder = r.nodeModulesExtensionOrder
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
	if absolute, ok, diffCase := r.loadAsMainField(dirInfo, path, extensionOrder); ok {
		return absolute, true, diffCase
	}

	// Look for an "index" file with known extensions
	if absolute, ok, diffCase := r.loadAsIndexWithBrowserRemapping(dirInfo, path, extensionOrder); ok {
		return absolute, true, diffCase
	}

	return PathPair{}, false, nil
}

func (r resolverQuery) loadAsMainField(dirInfo *dirInfo, path string, extensionOrder []string) (PathPair, bool, *fs.DifferentCase) {
	if dirInfo.packageJSON == nil {
		return PathPair{}, false, nil
	}

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
		fieldAbsPath := r.fs.Join(path, fieldRelPath)
		if remapped, ok := r.checkBrowserMap(dirInfo, fieldAbsPath, absolutePathKind); ok {
			if remapped == nil {
				return PathPair{Primary: logger.Path{Text: fieldAbsPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil
			}
			fieldAbsPath = r.fs.Join(path, *remapped)
		}

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
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	foundSomething := false

	for _, key := range mainFieldKeys {
		value, ok := mainFieldValues[key]
		if !ok {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Did not find main field %q", key))
			}
			continue
		}
		foundSomething = true

		absolute, ok, diffCase := loadMainField(value.relPath, key)
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

			if main, ok := mainFieldValues["main"]; ok {
				if absolute, ok, diffCase := loadMainField(main.relPath, "main"); ok {
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
				if r.kind != ast.ImportRequire {
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

	// Let the user know if "main" exists but was skipped due to mis-configuration
	if !foundSomething {
		for _, field := range mainFieldsForFailure {
			if main, ok := mainFieldValues[field]; ok {
				tracker := logger.MakeLineColumnTracker(&dirInfo.packageJSON.source)
				keyRange := dirInfo.packageJSON.source.RangeOfString(main.keyLoc)
				if len(mainFieldKeys) == 0 && r.options.Platform == config.PlatformNeutral {
					r.debugMeta.notes = append(r.debugMeta.notes, tracker.MsgData(keyRange,
						fmt.Sprintf("The %q field here was ignored. Main fields must be configured explicitly when using the \"neutral\" platform.",
							field)))
				} else {
					r.debugMeta.notes = append(r.debugMeta.notes, tracker.MsgData(keyRange,
						fmt.Sprintf("The %q field here was ignored because the list of main fields to use is currently set to [%s].",
							field, helpers.StringArrayToQuotedCommaSeparatedString(mainFieldKeys))))
				}
				break
			}
		}
	}

	return PathPair{}, false, nil
}

func hasCaseInsensitiveSuffix(s string, suffix string) bool {
	return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
}

// This closely follows the behavior of "tryLoadModuleUsingPaths()" in the
// official TypeScript compiler
func (r resolverQuery) matchTSConfigPaths(tsConfigJSON *TSConfigJSON, path string) (PathPair, bool, *fs.DifferentCase) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Matching %q against \"paths\" in %q", path, tsConfigJSON.AbsPath))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	absBaseURL := tsConfigJSON.BaseURLForPaths

	// The explicit base URL should take precedence over the implicit base URL
	// if present. This matters when a tsconfig.json file overrides "baseUrl"
	// from another extended tsconfig.json file but doesn't override "paths".
	if tsConfigJSON.BaseURL != nil {
		absBaseURL = *tsConfigJSON.BaseURL
	}

	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Using %q as \"baseUrl\"", absBaseURL))
	}

	// Check for exact matches first
	for key, originalPaths := range tsConfigJSON.Paths.Map {
		if key == path {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Found an exact match for %q in \"paths\"", key))
			}
			for _, originalPath := range originalPaths {
				// Ignore ".d.ts" files because this rule is obviously only here for type checking
				if hasCaseInsensitiveSuffix(originalPath.Text, ".d.ts") {
					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("Ignoring substitution %q because it ends in \".d.ts\"", originalPath.Text))
					}
					continue
				}

				// Load the original path relative to the "baseUrl" from tsconfig.json
				absoluteOriginalPath := originalPath.Text
				if !r.fs.IsAbs(absoluteOriginalPath) {
					absoluteOriginalPath = r.fs.Join(absBaseURL, absoluteOriginalPath)
				}
				if absolute, ok, diffCase := r.loadAsFileOrDirectory(absoluteOriginalPath); ok {
					return absolute, true, diffCase
				}
			}
			return PathPair{}, false, nil
		}
	}

	type match struct {
		prefix        string
		suffix        string
		originalPaths []TSConfigPath
	}

	// Check for pattern matches next
	longestMatchPrefixLength := -1
	longestMatchSuffixLength := -1
	var longestMatch match
	for key, originalPaths := range tsConfigJSON.Paths.Map {
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
			originalPath := strings.Replace(originalPath.Text, "*", matchedText, 1)

			// Ignore ".d.ts" files because this rule is obviously only here for type checking
			if hasCaseInsensitiveSuffix(originalPath, ".d.ts") {
				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("Ignoring substitution %q because it ends in \".d.ts\"", originalPath))
				}
				continue
			}

			// Load the original path relative to the "baseUrl" from tsconfig.json
			absoluteOriginalPath := originalPath
			if !r.fs.IsAbs(originalPath) {
				absoluteOriginalPath = r.fs.Join(absBaseURL, originalPath)
			}
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absoluteOriginalPath); ok {
				return absolute, true, diffCase
			}
		}
	}

	return PathPair{}, false, nil
}

func (r resolverQuery) loadPackageImports(importPath string, dirInfoPackageJSON *dirInfo) (PathPair, bool, *fs.DifferentCase, *SideEffectsData) {
	packageJSON := dirInfoPackageJSON.packageJSON

	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Looking for %q in \"imports\" map in %q", importPath, packageJSON.source.KeyPath.Text))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	// Filter out invalid module specifiers now where we have more information for
	// a better error message instead of later when we're inside the algorithm
	if importPath == "#" || strings.HasPrefix(importPath, "#/") {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("The path %q must not equal \"#\" and must not start with \"#/\".", importPath))
		}
		tracker := logger.MakeLineColumnTracker(&packageJSON.source)
		r.debugMeta.notes = append(r.debugMeta.notes, tracker.MsgData(packageJSON.importsMap.root.firstToken,
			fmt.Sprintf("This \"imports\" map was ignored because the module specifier %q is invalid:", importPath)))
		return PathPair{}, false, nil, nil
	}

	// The condition set is determined by the kind of import
	conditions := r.esmConditionsDefault
	switch r.kind {
	case ast.ImportStmt, ast.ImportDynamic:
		conditions = r.esmConditionsImport
	case ast.ImportRequire, ast.ImportRequireResolve:
		conditions = r.esmConditionsRequire
	}

	resolvedPath, status, debug := r.esmPackageImportsResolve(importPath, packageJSON.importsMap.root, conditions)
	resolvedPath, status, debug = r.esmHandlePostConditions(resolvedPath, status, debug)

	if status == pjStatusPackageResolve {
		if pathPair, ok, sideEffects := r.checkForBuiltInNodeModules(resolvedPath); ok {
			return pathPair, true, nil, sideEffects
		}

		// The import path was remapped via "imports" to another import path
		// that now needs to be resolved too. Set "forbidImports" to true
		// so we don't try to resolve "imports" again and end up in a loop.
		absolute, ok, diffCase, sideEffects := r.loadNodeModules(resolvedPath, dirInfoPackageJSON, true /* forbidImports */)
		if !ok {
			tracker := logger.MakeLineColumnTracker(&packageJSON.source)
			r.debugMeta.notes = append(
				[]logger.MsgData{tracker.MsgData(debug.token,
					fmt.Sprintf("The remapped path %q could not be resolved:", resolvedPath))},
				r.debugMeta.notes...)
		}
		return absolute, ok, diffCase, sideEffects
	}

	absolute, ok, diffCase := r.finalizeImportsExportsResult(
		finalizeImportsExportsNormal,
		dirInfoPackageJSON.absPath, conditions, *packageJSON.importsMap, packageJSON,
		resolvedPath, status, debug,
		"", "", "",
	)
	return absolute, ok, diffCase, nil
}

func (r resolverQuery) esmResolveAlgorithm(
	kind finalizeImportsExportsKind,
	esmPackageName string,
	esmPackageSubpath string,
	packageJSON *packageJSON,
	absPkgPath string,
	absPath string,
) (PathPair, bool, *fs.DifferentCase) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Looking for %q in \"exports\" map in %q", esmPackageSubpath, packageJSON.source.KeyPath.Text))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	// The condition set is determined by the kind of import
	conditions := r.esmConditionsDefault
	switch r.kind {
	case ast.ImportStmt, ast.ImportDynamic:
		conditions = r.esmConditionsImport
	case ast.ImportRequire, ast.ImportRequireResolve:
		conditions = r.esmConditionsRequire
	case ast.ImportEntryPoint:
		// Treat entry points as imports instead of requires for consistency with
		// Webpack and Rollup. More information:
		//
		// * https://github.com/evanw/esbuild/issues/1956
		// * https://github.com/nodejs/node/issues/41686
		// * https://github.com/evanw/entry-point-resolve-test
		//
		conditions = r.esmConditionsImport
	}

	// Resolve against the path "/", then join it with the absolute
	// directory path. This is done because ESM package resolution uses
	// URLs while our path resolution uses file system paths. We don't
	// want problems due to Windows paths, which are very unlike URL
	// paths. We also want to avoid any "%" characters in the absolute
	// directory path accidentally being interpreted as URL escapes.
	resolvedPath, status, debug := r.esmPackageExportsResolve("/", esmPackageSubpath, packageJSON.exportsMap.root, conditions)
	resolvedPath, status, debug = r.esmHandlePostConditions(resolvedPath, status, debug)

	return r.finalizeImportsExportsResult(
		kind,
		absPkgPath, conditions, *packageJSON.exportsMap, packageJSON,
		resolvedPath, status, debug,
		esmPackageName, esmPackageSubpath, absPath,
	)
}

func (r resolverQuery) loadNodeModules(importPath string, dirInfo *dirInfo, forbidImports bool) (PathPair, bool, *fs.DifferentCase, *SideEffectsData) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Searching for %q in \"node_modules\" directories starting from %q", importPath, dirInfo.absPath))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
	if tsConfigJSON := r.tsConfigForDir(dirInfo); tsConfigJSON != nil {
		// Try path substitutions first
		if tsConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(tsConfigJSON, importPath); ok {
				return absolute, true, diffCase, nil
			}
		}

		// Try looking up the path relative to the base URL
		if tsConfigJSON.BaseURL != nil {
			basePath := r.fs.Join(*tsConfigJSON.BaseURL, importPath)
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(basePath); ok {
				return absolute, true, diffCase, nil
			}
		}
	}

	// Find the parent directory with the "package.json" file
	dirInfoPackageJSON := dirInfo
	for dirInfoPackageJSON != nil && dirInfoPackageJSON.packageJSON == nil {
		dirInfoPackageJSON = dirInfoPackageJSON.parent
	}

	// Check for subpath imports: https://nodejs.org/api/packages.html#subpath-imports
	if dirInfoPackageJSON != nil && strings.HasPrefix(importPath, "#") && !forbidImports && dirInfoPackageJSON.packageJSON.importsMap != nil {
		return r.loadPackageImports(importPath, dirInfoPackageJSON)
	}

	// "import 'pkg'" when all packages are external (vs. "import './pkg'")
	if r.options.ExternalPackages && IsPackagePath(importPath) {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this path as external because it's a package path")
		}
		return PathPair{Primary: logger.Path{Text: importPath}, IsExternal: true}, true, nil, nil
	}

	// If Yarn PnP is active, use it to find the package
	if r.pnpManifest != nil {
		if result := r.resolveToUnqualified(importPath, dirInfo.absPath, r.pnpManifest); result.status.isError() {
			if r.debugLogs != nil {
				r.debugLogs.addNote("The Yarn PnP path resolution algorithm returned an error")
			}

			// Try to provide more information about this error if it's available
			switch result.status {
			case pnpErrorDependencyNotFound:
				r.debugMeta.notes = []logger.MsgData{r.pnpManifest.tracker.MsgData(result.errorRange,
					fmt.Sprintf("The Yarn Plug'n'Play manifest forbids importing %q here because it's not listed as a dependency of this package:", result.errorIdent))}

			case pnpErrorUnfulfilledPeerDependency:
				r.debugMeta.notes = []logger.MsgData{r.pnpManifest.tracker.MsgData(result.errorRange,
					fmt.Sprintf("The Yarn Plug'n'Play manifest says this package has a peer dependency on %q, but the package %q has not been installed:", result.errorIdent, result.errorIdent))}
			}

			return PathPair{}, false, nil, nil
		} else if result.status == pnpSuccess {
			absPath := r.fs.Join(result.pkgDirPath, result.pkgSubpath)

			// If Yarn PnP path resolution succeeded, run a custom abbreviated
			// version of node's module resolution algorithm. The Yarn PnP
			// specification says to use node's module resolution algorithm verbatim
			// but that isn't what Yarn actually does. See this for more info:
			// https://github.com/evanw/esbuild/issues/2473#issuecomment-1216774461
			if pkgDirInfo := r.dirInfoCached(result.pkgDirPath); pkgDirInfo != nil {
				// Check the "exports" map
				if packageJSON := pkgDirInfo.packageJSON; packageJSON != nil && packageJSON.exportsMap != nil {
					absolute, ok, diffCase := r.esmResolveAlgorithm(finalizeImportsExportsNormal, result.pkgIdent, "."+result.pkgSubpath, packageJSON, pkgDirInfo.absPath, absPath)
					return absolute, ok, diffCase, nil
				}

				// Check the "browser" map
				if remapped, ok := r.checkBrowserMap(pkgDirInfo, absPath, absolutePathKind); ok {
					if remapped == nil {
						return PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil, nil
					}
					if remappedResult, ok, diffCase, sideEffects := r.resolveWithoutRemapping(pkgDirInfo.enclosingBrowserScope, *remapped); ok {
						return remappedResult, true, diffCase, sideEffects
					}
				}

				if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
					return absolute, true, diffCase, nil
				}
			}

			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("Failed to resolve %q to a file", absPath))
			}
			return PathPair{}, false, nil, nil
		}
	}

	// Try to parse the package name using node's ESM-specific rules
	esmPackageName, esmPackageSubpath, esmOK := esmParsePackageName(importPath)
	if r.debugLogs != nil && esmOK {
		r.debugLogs.addNote(fmt.Sprintf("Parsed package name %q and package subpath %q", esmPackageName, esmPackageSubpath))
	}

	// Check for self-references
	if dirInfoPackageJSON != nil {
		if packageJSON := dirInfoPackageJSON.packageJSON; packageJSON.name == esmPackageName && packageJSON.exportsMap != nil {
			absolute, ok, diffCase := r.esmResolveAlgorithm(finalizeImportsExportsNormal, esmPackageName, esmPackageSubpath, packageJSON,
				dirInfoPackageJSON.absPath, r.fs.Join(dirInfoPackageJSON.absPath, esmPackageSubpath))
			return absolute, ok, diffCase, nil
		}
	}

	// Common package resolution logic shared between "node_modules" and "NODE_PATHS"
	tryToResolvePackage := func(absDir string) (PathPair, bool, *fs.DifferentCase, *SideEffectsData, bool) {
		absPath := r.fs.Join(absDir, importPath)
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("Checking for a package in the directory %q", absPath))
		}

		// Try node's new package resolution rules
		if esmOK {
			absPkgPath := r.fs.Join(absDir, esmPackageName)
			if pkgDirInfo := r.dirInfoCached(absPkgPath); pkgDirInfo != nil {
				// Check the "exports" map
				if packageJSON := pkgDirInfo.packageJSON; packageJSON != nil && packageJSON.exportsMap != nil {
					absolute, ok, diffCase := r.esmResolveAlgorithm(finalizeImportsExportsNormal, esmPackageName, esmPackageSubpath, packageJSON, absPkgPath, absPath)
					return absolute, ok, diffCase, nil, true
				}

				// Check the "browser" map
				if remapped, ok := r.checkBrowserMap(pkgDirInfo, absPath, absolutePathKind); ok {
					if remapped == nil {
						return PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil, nil, true
					}
					if remappedResult, ok, diffCase, sideEffects := r.resolveWithoutRemapping(pkgDirInfo.enclosingBrowserScope, *remapped); ok {
						return remappedResult, true, diffCase, sideEffects, true
					}
				}
			}
		}

		// Try node's old package resolution rules
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
			return absolute, true, diffCase, nil, true
		}

		return PathPair{}, false, nil, nil, false
	}

	// Then check for the package in any enclosing "node_modules" directories
	for {
		// Skip directories that are themselves called "node_modules", since we
		// don't ever want to search for "node_modules/node_modules"
		if dirInfo.hasNodeModules {
			if absolute, ok, diffCase, sideEffects, shouldStop := tryToResolvePackage(r.fs.Join(dirInfo.absPath, "node_modules")); shouldStop {
				return absolute, ok, diffCase, sideEffects
			}
		}

		// Go to the parent directory, stopping at the file system root
		dirInfo = dirInfo.parent
		if dirInfo == nil {
			break
		}
	}

	// Then check the global "NODE_PATH" environment variable. It has been
	// clarified that this step comes last after searching for "node_modules"
	// directories: https://github.com/nodejs/node/issues/38128.
	for _, absDir := range r.options.AbsNodePaths {
		if absolute, ok, diffCase, sideEffects, shouldStop := tryToResolvePackage(absDir); shouldStop {
			return absolute, ok, diffCase, sideEffects
		}
	}

	return PathPair{}, false, nil, nil
}

func (r resolverQuery) checkForBuiltInNodeModules(importPath string) (PathPair, bool, *SideEffectsData) {
	// "import fs from 'fs'"
	if r.options.Platform == config.PlatformNode && BuiltInNodeModules[importPath] {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this path as implicitly external due to it being a node built-in")
		}

		r.flushDebugLogs(flushDueToSuccess)
		return PathPair{Primary: logger.Path{Text: importPath}, IsExternal: true},
			true,
			&SideEffectsData{} // Mark this with "sideEffects: false"
	}

	// "import fs from 'node:fs'"
	// "require('node:fs')"
	if r.options.Platform == config.PlatformNode && strings.HasPrefix(importPath, "node:") {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this path as implicitly external due to the \"node:\" prefix")
		}

		// If this is a known node built-in module, mark it with "sideEffects: false"
		var sideEffects *SideEffectsData
		if BuiltInNodeModules[strings.TrimPrefix(importPath, "node:")] {
			sideEffects = &SideEffectsData{}
		}

		// Check whether the path will end up as "import" or "require"
		convertImportToRequire := !r.options.OutputFormat.KeepESMImportExportSyntax()
		isImport := !convertImportToRequire && (r.kind == ast.ImportStmt || r.kind == ast.ImportDynamic)
		isRequire := r.kind == ast.ImportRequire || r.kind == ast.ImportRequireResolve ||
			(convertImportToRequire && (r.kind == ast.ImportStmt || r.kind == ast.ImportDynamic))

		// Check for support with "import"
		if isImport && r.options.UnsupportedJSFeatures.Has(compat.NodeColonPrefixImport) {
			if r.debugLogs != nil {
				r.debugLogs.addNote("Removing the \"node:\" prefix because the target environment doesn't support it with \"import\" statements")
			}

			// Automatically strip the prefix if it's not supported
			importPath = importPath[5:]
		}

		// Check for support with "require"
		if isRequire && r.options.UnsupportedJSFeatures.Has(compat.NodeColonPrefixRequire) {
			if r.debugLogs != nil {
				r.debugLogs.addNote("Removing the \"node:\" prefix because the target environment doesn't support it with \"require\" calls")
			}

			// Automatically strip the prefix if it's not supported
			importPath = importPath[5:]
		}

		r.flushDebugLogs(flushDueToSuccess)
		return PathPair{Primary: logger.Path{Text: importPath}, IsExternal: true}, true, sideEffects
	}

	return PathPair{}, false, nil
}

type finalizeImportsExportsKind uint8

const (
	finalizeImportsExportsNormal finalizeImportsExportsKind = iota
	finalizeImportsExportsYarnPnPTSConfigExtends
)

func (r resolverQuery) finalizeImportsExportsResult(
	kind finalizeImportsExportsKind,
	absDirPath string,
	conditions map[string]bool,
	importExportMap pjMap,
	packageJSON *packageJSON,

	// Resolution results
	resolvedPath string,
	status pjStatus,
	debug pjDebug,

	// Only for exports
	esmPackageName string,
	esmPackageSubpath string,
	absImportPath string,
) (PathPair, bool, *fs.DifferentCase) {
	missingSuffix := ""

	if (status == pjStatusExact || status == pjStatusExactEndsWithStar || status == pjStatusInexact) && strings.HasPrefix(resolvedPath, "/") {
		absResolvedPath := r.fs.Join(absDirPath, resolvedPath)

		switch status {
		case pjStatusExact, pjStatusExactEndsWithStar:
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The resolved path %q is exact", absResolvedPath))
			}

			// Avoid calling "dirInfoCached" recursively for "tsconfig.json" extends with Yarn PnP
			if kind == finalizeImportsExportsYarnPnPTSConfigExtends {
				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("Resolved to %q", absResolvedPath))
				}
				return PathPair{Primary: logger.Path{Text: absResolvedPath, Namespace: "file"}}, true, nil
			}

			resolvedDirInfo := r.dirInfoCached(r.fs.Dir(absResolvedPath))
			base := r.fs.Base(absResolvedPath)
			extensionOrder := r.options.ExtensionOrder
			if r.kind.MustResolveToCSS() {
				extensionOrder = r.cssExtensionOrder
			}

			if resolvedDirInfo == nil {
				status = pjStatusModuleNotFound
			} else {
				entry, diffCase := resolvedDirInfo.entries.Get(base)

				// TypeScript-specific behavior: try rewriting ".js" to ".ts"
				if entry == nil {
					for old, exts := range rewrittenFileExtensions {
						if !strings.HasSuffix(base, old) {
							continue
						}
						lastDot := strings.LastIndexByte(base, '.')
						for _, ext := range exts {
							baseWithExt := base[:lastDot] + ext
							entry, diffCase = resolvedDirInfo.entries.Get(baseWithExt)
							if entry != nil {
								absResolvedPath = r.fs.Join(resolvedDirInfo.absPath, baseWithExt)
								break
							}
						}
						break
					}
				}

				if entry == nil {
					endsWithStar := status == pjStatusExactEndsWithStar
					status = pjStatusModuleNotFound

					// Try to have a friendly error message if people forget the extension
					if endsWithStar {
						for _, ext := range extensionOrder {
							if entry, _ := resolvedDirInfo.entries.Get(base + ext); entry != nil {
								if r.debugLogs != nil {
									r.debugLogs.addNote(fmt.Sprintf("The import %q is missing the extension %q", path.Join(esmPackageName, esmPackageSubpath), ext))
								}
								status = pjStatusModuleNotFoundMissingExtension
								missingSuffix = ext
								break
							}
						}
					}
				} else if kind := entry.Kind(r.fs); kind == fs.DirEntry {
					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("The path %q is a directory, which is not allowed", absResolvedPath))
					}
					endsWithStar := status == pjStatusExactEndsWithStar
					status = pjStatusUnsupportedDirectoryImport

					// Try to have a friendly error message if people forget the "/index.js" suffix
					if endsWithStar {
						if resolvedDirInfo := r.dirInfoCached(absResolvedPath); resolvedDirInfo != nil {
							for _, ext := range extensionOrder {
								base := "index" + ext
								if entry, _ := resolvedDirInfo.entries.Get(base); entry != nil && entry.Kind(r.fs) == fs.FileEntry {
									status = pjStatusUnsupportedDirectoryImportMissingIndex
									missingSuffix = "/" + base
									if r.debugLogs != nil {
										r.debugLogs.addNote(fmt.Sprintf("The import %q is missing the suffix %q", path.Join(esmPackageName, esmPackageSubpath), missingSuffix))
									}
									break
								}
							}
						}
					}
				} else if kind != fs.FileEntry {
					status = pjStatusModuleNotFound
				} else {
					if r.debugLogs != nil {
						r.debugLogs.addNote(fmt.Sprintf("Resolved to %q", absResolvedPath))
					}
					return PathPair{Primary: logger.Path{Text: absResolvedPath, Namespace: "file"}}, true, diffCase
				}
			}

		case pjStatusInexact:
			// If this was resolved against an expansion key ending in a "/"
			// instead of a "*", we need to try CommonJS-style implicit
			// extension and/or directory detection.
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The resolved path %q is inexact", absResolvedPath))
			}
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absResolvedPath); ok {
				return absolute, true, diffCase
			}
			status = pjStatusModuleNotFound
		}
	}

	if strings.HasPrefix(resolvedPath, "/") {
		resolvedPath = "." + resolvedPath
	}

	// Provide additional details about the failure to help with debugging
	tracker := logger.MakeLineColumnTracker(&packageJSON.source)
	switch status {
	case pjStatusInvalidModuleSpecifier:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("The module specifier %q is invalid%s:", resolvedPath, debug.invalidBecause))}

	case pjStatusInvalidPackageConfiguration:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			"The package configuration has an invalid value here:")}

	case pjStatusInvalidPackageTarget:
		why := fmt.Sprintf("The package target %q is invalid%s:", resolvedPath, debug.invalidBecause)
		if resolvedPath == "" {
			// "PACKAGE_TARGET_RESOLVE" is specified to throw an "Invalid
			// Package Target" error for what is actually an invalid package
			// configuration error
			why = "The package configuration has an invalid value here:"
		}
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token, why)}

	case pjStatusPackagePathNotExported:
		if debug.isBecauseOfNullLiteral {
			r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
				fmt.Sprintf("The path %q cannot be imported from package %q because it was explicitly disabled by the package author here:", esmPackageSubpath, esmPackageName))}
			break
		}

		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("The path %q is not exported by package %q:", esmPackageSubpath, esmPackageName))}

		// If this fails, try to resolve it using the old algorithm
		if absolute, ok, _ := r.loadAsFileOrDirectory(absImportPath); ok && absolute.Primary.Namespace == "file" {
			if relPath, ok := r.fs.Rel(absDirPath, absolute.Primary.Text); ok {
				query := "." + path.Join("/", strings.ReplaceAll(relPath, "\\", "/"))

				// If that succeeds, try to do a reverse lookup using the
				// "exports" map for the currently-active set of conditions
				if ok, subpath, token := r.esmPackageExportsReverseResolve(
					query, importExportMap.root, conditions); ok {
					r.debugMeta.notes = append(r.debugMeta.notes, tracker.MsgData(token,
						fmt.Sprintf("The file %q is exported at path %q:", query, subpath)))

					// Provide an inline suggestion message with the correct import path
					actualImportPath := path.Join(esmPackageName, subpath)
					r.debugMeta.suggestionText = string(helpers.QuoteForJSON(actualImportPath, false))
					r.debugMeta.suggestionMessage = fmt.Sprintf("Import from %q to get the file %q:",
						actualImportPath, PrettyPath(r.fs, absolute.Primary))
				}
			}
		}

	case pjStatusPackageImportNotDefined:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("The package import %q is not defined in this \"imports\" map:", resolvedPath))}

	case pjStatusModuleNotFound, pjStatusModuleNotFoundMissingExtension:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("The module %q was not found on the file system:", resolvedPath))}

		// Provide an inline suggestion message with the correct import path
		if status == pjStatusModuleNotFoundMissingExtension {
			actualImportPath := path.Join(esmPackageName, esmPackageSubpath+missingSuffix)
			r.debugMeta.suggestionRange = suggestionRangeEnd
			r.debugMeta.suggestionText = missingSuffix
			r.debugMeta.suggestionMessage = fmt.Sprintf("Import from %q to get the file %q:",
				actualImportPath, PrettyPath(r.fs, logger.Path{Text: r.fs.Join(absDirPath, resolvedPath+missingSuffix), Namespace: "file"}))
		}

	case pjStatusUnsupportedDirectoryImport, pjStatusUnsupportedDirectoryImportMissingIndex:
		r.debugMeta.notes = []logger.MsgData{
			tracker.MsgData(debug.token, fmt.Sprintf("Importing the directory %q is forbidden by this package:", resolvedPath)),
			tracker.MsgData(packageJSON.source.RangeOfString(importExportMap.propertyKeyLoc),
				fmt.Sprintf("The presence of %q here makes importing a directory forbidden:", importExportMap.propertyKey)),
		}

		// Provide an inline suggestion message with the correct import path
		if status == pjStatusUnsupportedDirectoryImportMissingIndex {
			actualImportPath := path.Join(esmPackageName, esmPackageSubpath+missingSuffix)
			r.debugMeta.suggestionRange = suggestionRangeEnd
			r.debugMeta.suggestionText = missingSuffix
			r.debugMeta.suggestionMessage = fmt.Sprintf("Import from %q to get the file %q:",
				actualImportPath, PrettyPath(r.fs, logger.Path{Text: r.fs.Join(absDirPath, resolvedPath+missingSuffix), Namespace: "file"}))
		}

	case pjStatusUndefinedNoConditionsMatch:
		keys := make([]string, 0, len(conditions))
		for key := range conditions {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		unmatchedConditions := make([]string, len(debug.unmatchedConditions))
		for i, key := range debug.unmatchedConditions {
			unmatchedConditions[i] = key.Text
		}

		r.debugMeta.notes = []logger.MsgData{
			tracker.MsgData(importExportMap.root.firstToken,
				fmt.Sprintf("The path %q is not currently exported by package %q:",
					esmPackageSubpath, esmPackageName)),

			tracker.MsgData(debug.token,
				fmt.Sprintf("None of the conditions in the package definition (%s) match any of the currently active conditions (%s):",
					helpers.StringArrayToQuotedCommaSeparatedString(unmatchedConditions),
					helpers.StringArrayToQuotedCommaSeparatedString(keys),
				)),
		}

		didSuggestEnablingCondition := false
		for _, key := range debug.unmatchedConditions {
			switch key.Text {
			case "import":
				if r.kind == ast.ImportRequire || r.kind == ast.ImportRequireResolve {
					r.debugMeta.suggestionMessage = "Consider using an \"import\" statement to import this file, " +
						"which will work because the \"import\" condition is supported by this package:"
				}

			case "require":
				if r.kind == ast.ImportStmt || r.kind == ast.ImportDynamic {
					r.debugMeta.suggestionMessage = "Consider using a \"require()\" call to import this file, " +
						"which will work because the \"require\" condition is supported by this package:"
				}

			default:
				// Note: Don't suggest the adding the "types" condition because
				// TypeScript uses that for type definitions, which are not
				// intended to be included in a bundle as executable code
				if !didSuggestEnablingCondition && key.Text != "types" {
					var how string
					switch logger.API {
					case logger.CLIAPI:
						how = fmt.Sprintf("\"--conditions=%s\"", key.Text)
					case logger.JSAPI:
						how = fmt.Sprintf("\"conditions: ['%s']\"", key.Text)
					case logger.GoAPI:
						how = fmt.Sprintf("'Conditions: []string{%q}'", key.Text)
					}
					r.debugMeta.notes = append(r.debugMeta.notes, tracker.MsgData(key.Range,
						fmt.Sprintf("Consider enabling the %q condition if this package expects it to be enabled. "+
							"You can use %s to do that:", key.Text, how)))
					didSuggestEnablingCondition = true
				}
			}
		}
	}

	return PathPair{}, false, nil
}

// Package paths are loaded from a "node_modules" directory. Non-package paths
// are relative or absolute paths.
func IsPackagePath(path string) bool {
	return !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "./") &&
		!strings.HasPrefix(path, "../") && path != "." && path != ".."
}

// This list can be obtained with the following command:
//
//	node --experimental-wasi-unstable-preview1 -p "[...require('module').builtinModules].join('\n')"
//
// Be sure to use the *LATEST* version of node when updating this list!
var BuiltInNodeModules = map[string]bool{
	"_http_agent":         true,
	"_http_client":        true,
	"_http_common":        true,
	"_http_incoming":      true,
	"_http_outgoing":      true,
	"_http_server":        true,
	"_stream_duplex":      true,
	"_stream_passthrough": true,
	"_stream_readable":    true,
	"_stream_transform":   true,
	"_stream_wrap":        true,
	"_stream_writable":    true,
	"_tls_common":         true,
	"_tls_wrap":           true,
	"assert":              true,
	"assert/strict":       true,
	"async_hooks":         true,
	"buffer":              true,
	"child_process":       true,
	"cluster":             true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"diagnostics_channel": true,
	"dns":                 true,
	"dns/promises":        true,
	"domain":              true,
	"events":              true,
	"fs":                  true,
	"fs/promises":         true,
	"http":                true,
	"http2":               true,
	"https":               true,
	"inspector":           true,
	"module":              true,
	"net":                 true,
	"os":                  true,
	"path":                true,
	"path/posix":          true,
	"path/win32":          true,
	"perf_hooks":          true,
	"process":             true,
	"punycode":            true,
	"querystring":         true,
	"readline":            true,
	"repl":                true,
	"stream":              true,
	"stream/consumers":    true,
	"stream/promises":     true,
	"stream/web":          true,
	"string_decoder":      true,
	"sys":                 true,
	"timers":              true,
	"timers/promises":     true,
	"tls":                 true,
	"trace_events":        true,
	"tty":                 true,
	"url":                 true,
	"util":                true,
	"util/types":          true,
	"v8":                  true,
	"vm":                  true,
	"wasi":                true,
	"worker_threads":      true,
	"zlib":                true,
}
