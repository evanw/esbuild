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
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
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

	// If non-empty, this was the result of an "onResolve" plugin
	PluginName string

	// If this was resolved by a plugin, the plugin gets to store its data here
	PluginData interface{}

	// If not empty, these should override the default values
	JSXFactory  []string // Default if empty: "React.createElement"
	JSXFragment []string // Default if empty: "React.Fragment"

	DifferentCase *fs.DifferentCase

	// If present, any ES6 imports to this file can be considered to have no side
	// effects. This means they should be removed if unused.
	PrimarySideEffectsData *SideEffectsData

	TSTarget *config.TSTarget

	// This is the "type" field from "package.json"
	ModuleTypeData js_ast.ModuleTypeData

	IsExternal bool

	// If true, the class field transform should use Object.defineProperty().
	UseDefineForClassFieldsTS config.MaybeBool

	// This is the "importsNotUsedAsValues" and "preserveValueImports" fields from "package.json"
	UnusedImportsTS config.UnusedImportsTS
}

func prettyPrintPluginName(prefix string, key string, value string) string {
	if value == "" {
		return fmt.Sprintf("%s  %q: null,", prefix, key)
	}
	return fmt.Sprintf("%s  %q: %q,", prefix, key, value)
}

func prettyPrintPath(prefix string, key string, value logger.Path) string {
	lines := []string{
		fmt.Sprintf("%s  %q: {", prefix, key),
		fmt.Sprintf("%s    \"text\": %q,", prefix, value.Text),
		fmt.Sprintf("%s    \"namespace\": %q,", prefix, value.Namespace),
	}
	if value.IgnoredSuffix != "" {
		lines = append(lines, fmt.Sprintf("%s    \"suffix\": %q,", prefix, value.IgnoredSuffix))
	}
	if value.IsDisabled() {
		lines = append(lines, fmt.Sprintf("%s    \"disabled\": true,", prefix))
	}
	lines = append(lines, fmt.Sprintf("%s  },", prefix))
	return strings.Join(lines, "\n")
}

func prettyPrintStringArray(prefix string, key string, value []string) string {
	return fmt.Sprintf("%s  %q: [%s],", prefix, key, helpers.StringArrayToQuotedCommaSeparatedString(value))
}

func prettyPrintTSTarget(prefix string, key string, value *config.TSTarget) string {
	if value == nil {
		return fmt.Sprintf("%s  %q: null,", prefix, key)
	}
	return fmt.Sprintf("%s  %q: %q,", prefix, key, value.Target)
}

func prettyPrintModuleType(prefix string, key string, value js_ast.ModuleType) string {
	kind := "null"
	if value.IsCommonJS() {
		kind = "\"commonjs\""
	} else if value.IsESM() {
		kind = "\"module\""
	}
	return fmt.Sprintf("%s  %q: %s,", prefix, key, kind)
}

func prettyPrintUnusedImports(prefix string, key string, value config.UnusedImportsTS) string {
	source := "null"
	switch value {
	case config.UnusedImportsKeepStmtRemoveValues:
		source = "{ \"importsNotUsedAsValues\": \"preserve\" }"
	case config.UnusedImportsKeepValues:
		source = "{ \"preserveValueImports\": true }"
	}
	return fmt.Sprintf("%s  %q: %s,", prefix, key, source)
}

func (old *ResolveResult) Compare(new *ResolveResult) (diff []string) {
	var oldDiff []string
	var newDiff []string

	if !old.PathPair.Primary.IsEquivalentTo(new.PathPair.Primary) {
		oldDiff = append(oldDiff, prettyPrintPath("-", "path", old.PathPair.Primary))
		newDiff = append(newDiff, prettyPrintPath("+", "path", new.PathPair.Primary))
	}

	if !old.PathPair.Secondary.IsEquivalentTo(new.PathPair.Secondary) {
		oldDiff = append(oldDiff, prettyPrintPath("-", "secondaryPath", old.PathPair.Secondary))
		newDiff = append(newDiff, prettyPrintPath("+", "secondaryPath", new.PathPair.Secondary))
	}

	if !helpers.StringArraysEqual(old.JSXFactory, new.JSXFactory) {
		oldDiff = append(oldDiff, prettyPrintStringArray("-", "jsxFactory", old.JSXFactory))
		newDiff = append(newDiff, prettyPrintStringArray("+", "jsxFactory", new.JSXFactory))
	}

	if !helpers.StringArraysEqual(old.JSXFragment, new.JSXFragment) {
		oldDiff = append(oldDiff, prettyPrintStringArray("-", "jsxFragment", old.JSXFragment))
		newDiff = append(newDiff, prettyPrintStringArray("+", "jsxFragment", new.JSXFragment))
	}

	if (old.PrimarySideEffectsData != nil) != (new.PrimarySideEffectsData != nil) {
		oldDiff = append(oldDiff, fmt.Sprintf("-  \"sideEffects\": %v,", old.PrimarySideEffectsData != nil))
		newDiff = append(newDiff, fmt.Sprintf("+  \"sideEffects\": %v,", new.PrimarySideEffectsData != nil))
	}

	if !old.TSTarget.IsEquivalentTo(new.TSTarget) {
		oldDiff = append(oldDiff, prettyPrintTSTarget("-", "tsTarget", old.TSTarget))
		newDiff = append(newDiff, prettyPrintTSTarget("+", "tsTarget", new.TSTarget))
	}

	if !old.ModuleTypeData.Type.IsEquivalentTo(new.ModuleTypeData.Type) {
		oldDiff = append(oldDiff, prettyPrintModuleType("-", "type", old.ModuleTypeData.Type))
		newDiff = append(newDiff, prettyPrintModuleType("+", "type", new.ModuleTypeData.Type))
	}

	if old.IsExternal != new.IsExternal {
		oldDiff = append(oldDiff, fmt.Sprintf("-  \"external\": %v,", old.IsExternal))
		newDiff = append(newDiff, fmt.Sprintf("+  \"external\": %v,", new.IsExternal))
	}

	if old.UseDefineForClassFieldsTS != new.UseDefineForClassFieldsTS {
		oldDiff = append(oldDiff, fmt.Sprintf("-  \"useDefineForClassFields\": %v,", old.UseDefineForClassFieldsTS))
		newDiff = append(newDiff, fmt.Sprintf("+  \"useDefineForClassFields\": %v,", new.UseDefineForClassFieldsTS))
	}

	if old.UnusedImportsTS != new.UnusedImportsTS {
		oldDiff = append(oldDiff, prettyPrintUnusedImports("-", "unusedImports", old.UnusedImportsTS))
		newDiff = append(newDiff, prettyPrintUnusedImports("+", "unusedImports", new.UnusedImportsTS))
	}

	if oldDiff != nil {
		diff = make([]string, 0, 2+len(oldDiff)+len(newDiff))
		diff = append(diff, " {")
		diff = append(diff, oldDiff...)
		diff = append(diff, newDiff...)
		diff = append(diff, " }")
	}
	return
}

type DebugMeta struct {
	suggestionText    string
	suggestionMessage string
	notes             []logger.MsgData
}

func (dm DebugMeta) LogErrorMsg(log logger.Log, source *logger.Source, r logger.Range, text string, suggestion string, notes []logger.MsgData) {
	tracker := logger.MakeLineColumnTracker(source)

	if source != nil && dm.suggestionMessage != "" {
		data := tracker.MsgData(r, dm.suggestionMessage)
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

type Resolver interface {
	Resolve(sourceDir string, importPath string, kind ast.ImportKind) (result *ResolveResult, debug DebugMeta)
	ResolveAbs(absPath string) *ResolveResult
	PrettyPath(path logger.Path) string
	Finalize(result *ResolveResult)

	// This tries to run "Resolve" on a package path as a relative path. If
	// successful, the user just forgot a leading "./" in front of the path.
	ProbeResolvePackageAsRelative(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult
}

type resolver struct {
	fs     fs.FS
	log    logger.Log
	caches *cache.CacheSet

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

	// This cache maps a directory path to information about that directory and
	// all parent directories
	dirCache map[string]*dirInfo

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
	*resolver
	debugMeta *DebugMeta
	debugLogs *debugLogs
	kind      ast.ImportKind
}

func NewResolver(fs fs.FS, log logger.Log, caches *cache.CacheSet, options config.Options) Resolver {
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
	var debugMeta DebugMeta
	r := resolverQuery{
		resolver:  rr,
		debugMeta: &debugMeta,
		kind:      kind,
	}
	if r.log.Level <= logger.LevelDebug {
		r.debugLogs = &debugLogs{what: fmt.Sprintf(
			"Resolving import %q in directory %q of type %q",
			importPath, sourceDir, kind.StringForMetafile())}
	}

	// Certain types of URLs default to being external for convenience
	if isExplicitlyExternal := r.isExternal(r.options.ExternalSettings.PreResolve, importPath); isExplicitlyExternal ||

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
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
		}, debugMeta
	}

	// "import fs from 'fs'"
	if r.options.Platform == config.PlatformNode && BuiltInNodeModules[importPath] {
		if r.debugLogs != nil {
			r.debugLogs.addNote("Marking this path as implicitly external due to it being a node built-in")
		}

		r.flushDebugLogs(flushDueToSuccess)
		return &ResolveResult{
			PathPair:               PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal:             true,
			PrimarySideEffectsData: &SideEffectsData{}, // Mark this with "sideEffects: false"
		}, debugMeta
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
		convertImportToRequire := !r.options.OutputFormat.KeepES6ImportExportSyntax()
		isImport := !convertImportToRequire && (kind == ast.ImportStmt || kind == ast.ImportDynamic)
		isRequire := kind == ast.ImportRequire || kind == ast.ImportRequireResolve ||
			(convertImportToRequire && (kind == ast.ImportStmt || kind == ast.ImportDynamic))

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
		return &ResolveResult{
			PathPair:               PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal:             true,
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
			PathPair:   PathPair{Primary: logger.Path{Text: importPath}},
			IsExternal: true,
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

	r.mutex.Lock()
	defer r.mutex.Unlock()

	result := r.resolveWithoutSymlinks(sourceDir, importPath)
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
		if result2 := r.resolveWithoutSymlinks(sourceDir, importPath[:suffix]); result2 == nil {
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

func (r resolverQuery) isExternal(matchers config.ExternalMatchers, path string) bool {
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

func (rr *resolver) Finalize(result *ResolveResult) {
	r := resolverQuery{resolver: rr}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.finalizeResolve(result)
}

func (rr *resolver) ProbeResolvePackageAsRelative(sourceDir string, importPath string, kind ast.ImportKind) *ResolveResult {
	r := resolverQuery{
		resolver: rr,
		kind:     kind,
	}
	absPath := r.fs.Join(sourceDir, importPath)

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if pair, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
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
			r.log.AddWithNotes(logger.Debug, nil, logger.Range{}, r.debugLogs.what, r.debugLogs.notes)
		} else if r.log.Level <= logger.LevelVerbose {
			r.log.AddWithNotes(logger.Verbose, nil, logger.Range{}, r.debugLogs.what, r.debugLogs.notes)
		}
	}
}

func (r resolverQuery) finalizeResolve(result *ResolveResult) {
	if !result.IsExternal && r.isExternal(r.options.ExternalSettings.PostResolve, result.PathPair.Primary.Text) {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", result.PathPair.Primary.Text))
		}
		result.IsExternal = true
		return
	}

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
				if pkgJSON := dirInfo.enclosingPackageJSON; pkgJSON != nil && *path == result.PathPair.Primary {
					if pkgJSON.sideEffectsMap != nil {
						hasSideEffects := false
						if pkgJSON.sideEffectsMap[path.Text] {
							// Fast path: map lookup
							hasSideEffects = true
						} else {
							// Slow path: glob tests
							for _, re := range pkgJSON.sideEffectsRegexps {
								if re.MatchString(path.Text) {
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
				if path == &result.PathPair.Primary && dirInfo.enclosingTSConfigJSON != nil {
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
					if helpers.IsInsideNodeModules(result.PathPair.Primary.Text) {
						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("Ignoring %q because %q is inside \"node_modules\"",
								dirInfo.enclosingTSConfigJSON.AbsPath,
								result.PathPair.Primary.Text))
						}
					} else {
						result.JSXFactory = dirInfo.enclosingTSConfigJSON.JSXFactory
						result.JSXFragment = dirInfo.enclosingTSConfigJSON.JSXFragmentFactory
						result.UseDefineForClassFieldsTS = dirInfo.enclosingTSConfigJSON.UseDefineForClassFields
						result.UnusedImportsTS = config.UnusedImportsFromTsconfigValues(
							dirInfo.enclosingTSConfigJSON.PreserveImportsNotUsedAsValues,
							dirInfo.enclosingTSConfigJSON.PreserveValueImports,
						)
						result.TSTarget = dirInfo.enclosingTSConfigJSON.TSTarget

						if r.debugLogs != nil {
							r.debugLogs.addNote(fmt.Sprintf("This import is under the effect of %q",
								dirInfo.enclosingTSConfigJSON.AbsPath))
							if result.JSXFactory != nil {
								r.debugLogs.addNote(fmt.Sprintf("\"jsxFactory\" is %q due to %q",
									strings.Join(result.JSXFactory, "."),
									dirInfo.enclosingTSConfigJSON.AbsPath))
							}
							if result.JSXFragment != nil {
								r.debugLogs.addNote(fmt.Sprintf("\"jsxFragment\" is %q due to %q",
									strings.Join(result.JSXFragment, "."),
									dirInfo.enclosingTSConfigJSON.AbsPath))
							}
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

func (r resolverQuery) resolveWithoutSymlinks(sourceDir string, importPath string) *ResolveResult {
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
		if dirInfo := r.dirInfoCached(sourceDir); dirInfo != nil && dirInfo.enclosingTSConfigJSON != nil && dirInfo.enclosingTSConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(dirInfo.enclosingTSConfigJSON, importPath); ok {
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
	checkRelative := !isPackagePath || r.kind == ast.ImportURL || r.kind == ast.ImportAt
	checkPackage := isPackagePath

	if checkRelative {
		absPath := r.fs.Join(sourceDir, importPath)

		// Check for external packages first
		if r.isExternal(r.options.ExternalSettings.PostResolve, absPath) {
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The path %q was marked as external by the user", absPath))
			}
			return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file"}}, IsExternal: true}
		}

		// Check the "browser" map
		if importDirInfo := r.dirInfoCached(r.fs.Dir(absPath)); importDirInfo != nil {
			if remapped, ok := r.checkBrowserMap(importDirInfo, absPath, absolutePathKind); ok {
				if remapped == nil {
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}}
				}
				if remappedResult, ok, diffCase := r.resolveWithoutRemapping(importDirInfo.enclosingBrowserScope, *remapped); ok {
					result = ResolveResult{PathPair: remappedResult, DifferentCase: diffCase}
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
		sourceDirInfo := r.dirInfoCached(sourceDir)
		if sourceDirInfo == nil {
			// Bail if the directory is missing for some reason
			return nil
		}

		// Support remapping one package path to another via the "browser" field
		if remapped, ok := r.checkBrowserMap(sourceDirInfo, importPath, packagePathKind); ok {
			if remapped == nil {
				// "browser": {"module": false}
				if absolute, ok, diffCase := r.loadNodeModules(importPath, sourceDirInfo, false /* forbidImports */); ok {
					absolute.Primary = logger.Path{Text: absolute.Primary.Text, Namespace: "file", Flags: logger.PathDisabled}
					if absolute.HasSecondary() {
						absolute.Secondary = logger.Path{Text: absolute.Secondary.Text, Namespace: "file", Flags: logger.PathDisabled}
					}
					return &ResolveResult{PathPair: absolute, DifferentCase: diffCase}
				} else {
					return &ResolveResult{PathPair: PathPair{Primary: logger.Path{Text: importPath, Flags: logger.PathDisabled}}, DifferentCase: diffCase}
				}
			}

			// "browser": {"module": "./some-file"}
			// "browser": {"module": "another-module"}
			importPath = *remapped
			sourceDirInfo = sourceDirInfo.enclosingBrowserScope
		}

		if absolute, ok, diffCase := r.resolveWithoutRemapping(sourceDirInfo, importPath); ok {
			result = ResolveResult{PathPair: absolute, DifferentCase: diffCase}
		} else {
			// Note: node's "self references" are not currently supported
			return nil
		}
	}

	return &result
}

func (r resolverQuery) resolveWithoutRemapping(sourceDirInfo *dirInfo, importPath string) (PathPair, bool, *fs.DifferentCase) {
	if IsPackagePath(importPath) {
		return r.loadNodeModules(importPath, sourceDirInfo, false /* forbidImports */)
	} else {
		return r.loadAsFileOrDirectory(r.fs.Join(sourceDirInfo.absPath, importPath))
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
	absPath               string
	entries               fs.DirEntries
	packageJSON           *packageJSON  // Is there a "package.json" file in this directory?
	enclosingPackageJSON  *packageJSON  // Is there a "package.json" file in this directory or a parent directory?
	enclosingTSConfigJSON *TSConfigJSON // Is there a "tsconfig.json" file in this directory or a parent directory?
	absRealPath           string        // If non-empty, this is the real absolute path resolving any symlinks
	isNodeModules         bool          // Is the base name "node_modules"?
	hasNodeModules        bool          // Is there a "node_modules" subdirectory?
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
			count := len(cached.entries.SortedKeys())
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
	isExtends := len(visited) != 0
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
	tracker := logger.MakeLineColumnTracker(&source)
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
							r.log.Add(logger.Warning, &tracker, extendsRange,
								fmt.Sprintf("Base config file %q forms cycle", extends))
						} else if err != errParseErrorAlreadyLogged {
							r.log.Add(logger.Error, &tracker, extendsRange,
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
					r.log.Add(logger.Warning, &tracker, extendsRange,
						fmt.Sprintf("Base config file %q forms cycle", extends))
				} else if err != errParseErrorAlreadyLogged {
					r.log.Add(logger.Error, &tracker, extendsRange,
						fmt.Sprintf("Cannot read file %q: %s",
							r.PrettyPath(logger.Path{Text: fileToCheck, Namespace: "file"}), err.Error()))
				}
				return nil
			}
		}

		// Suppress warnings about missing base config files inside "node_modules"
		if !helpers.IsInsideNodeModules(file) {
			r.log.Add(logger.Warning, &tracker, extendsRange,
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
			r.log.Add(logger.Error, nil, logger.Range{},
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
	if base == "node_modules" {
		info.isNodeModules = true
	} else if entry, _ := entries.Get("node_modules"); entry != nil {
		info.hasNodeModules = entry.Kind(r.fs) == fs.DirEntry
	}

	// Propagate the browser scope into child directories
	if parentInfo != nil {
		info.enclosingPackageJSON = parentInfo.enclosingPackageJSON
		info.enclosingBrowserScope = parentInfo.enclosingBrowserScope
		info.enclosingTSConfigJSON = parentInfo.enclosingTSConfigJSON

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
			info.enclosingTSConfigJSON, err = r.parseTSConfig(tsConfigPath, make(map[string]bool))
			if err != nil {
				if err == syscall.ENOENT {
					r.log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Cannot find tsconfig file %q",
						r.PrettyPath(logger.Path{Text: tsConfigPath, Namespace: "file"})))
				} else if err != errParseErrorAlreadyLogged {
					r.log.Add(logger.Debug, nil, logger.Range{},
						fmt.Sprintf("Cannot read file %q: %s",
							r.PrettyPath(logger.Path{Text: tsConfigPath, Namespace: "file"}), err.Error()))
				}
			}
		}
	}

	return info
}

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
			r.log.Add(logger.Error, nil, logger.Range{},
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
	for old, exts := range rewrittenFileExtensions {
		if !strings.HasSuffix(base, old) {
			continue
		}
		lastDot := strings.LastIndexByte(base, '.')
		for _, ext := range exts {
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
		break
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
			if absolute, ok, _ := r.loadAsIndex(fieldDirInfo, remappedAbs, extensionOrder); ok {
				return absolute, true, nil
			}
		}

		return PathPair{}, false, nil
	}

	return r.loadAsIndex(dirInfo, path, extensionOrder)
}

func getProperty(json js_ast.Expr, name string) (js_ast.Expr, logger.Loc, bool) {
	if obj, ok := json.Data.(*js_ast.EObject); ok {
		for _, prop := range obj.Properties {
			if key, ok := prop.Key.Data.(*js_ast.EString); ok && key.Value != nil &&
				len(key.Value) == len(name) && helpers.UTF16ToString(key.Value) == name {
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
	// Use a special import order for CSS "@import" imports
	extensionOrder := r.options.ExtensionOrder
	if r.kind == ast.ImportAt || r.kind == ast.ImportAtConditional {
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
		r.debugLogs.addNote(fmt.Sprintf("Using %q as \"baseURL\"", absBaseURL))
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

func (r resolverQuery) loadNodeModules(importPath string, dirInfo *dirInfo, forbidImports bool) (PathPair, bool, *fs.DifferentCase) {
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Searching for %q in \"node_modules\" directories starting from %q", importPath, dirInfo.absPath))
		r.debugLogs.increaseIndent()
		defer r.debugLogs.decreaseIndent()
	}

	// First, check path overrides from the nearest enclosing TypeScript "tsconfig.json" file
	if dirInfo.enclosingTSConfigJSON != nil {
		// Try path substitutions first
		if dirInfo.enclosingTSConfigJSON.Paths != nil {
			if absolute, ok, diffCase := r.matchTSConfigPaths(dirInfo.enclosingTSConfigJSON, importPath); ok {
				return absolute, true, diffCase
			}
		}

		// Try looking up the path relative to the base URL
		if dirInfo.enclosingTSConfigJSON.BaseURL != nil {
			basePath := r.fs.Join(*dirInfo.enclosingTSConfigJSON.BaseURL, importPath)
			if absolute, ok, diffCase := r.loadAsFileOrDirectory(basePath); ok {
				return absolute, true, diffCase
			}
		}
	}

	// Find the parent directory with the "package.json" file
	dirInfoPackageJSON := dirInfo
	for dirInfoPackageJSON != nil && dirInfoPackageJSON.packageJSON == nil {
		dirInfoPackageJSON = dirInfoPackageJSON.parent
	}

	// Then check for the package in any enclosing "node_modules" directories
	if dirInfoPackageJSON != nil && strings.HasPrefix(importPath, "#") && !forbidImports && dirInfoPackageJSON.packageJSON.importsMap != nil {
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
			return PathPair{}, false, nil
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
			// The import path was remapped via "imports" to another import path
			// that now needs to be resolved too. Set "forbidImports" to true
			// so we don't try to resolve "imports" again and end up in a loop.
			absolute, ok, diffCase := r.loadNodeModules(resolvedPath, dirInfoPackageJSON, true /* forbidImports */)
			if !ok {
				tracker := logger.MakeLineColumnTracker(&packageJSON.source)
				r.debugMeta.notes = append(
					[]logger.MsgData{tracker.MsgData(debug.token,
						fmt.Sprintf("The remapped path %q could not be resolved:", resolvedPath))},
					r.debugMeta.notes...)
			}
			return absolute, ok, diffCase
		}

		return r.finalizeImportsExportsResult(
			dirInfoPackageJSON.absPath, conditions, *packageJSON.importsMap, packageJSON,
			resolvedPath, status, debug,
			"", "", "",
		)
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

			// Check the package's package.json file
			if esmOK {
				absPkgPath := r.fs.Join(dirInfo.absPath, "node_modules", esmPackageName)
				if pkgDirInfo := r.dirInfoCached(absPkgPath); pkgDirInfo != nil {
					// Check the "exports" map
					if packageJSON := pkgDirInfo.packageJSON; packageJSON != nil && packageJSON.exportsMap != nil {
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
							absPkgPath, conditions, *packageJSON.exportsMap, packageJSON,
							resolvedPath, status, debug,
							esmPackageName, esmPackageSubpath, absPath,
						)
					}

					// Check the "browser" map
					if remapped, ok := r.checkBrowserMap(pkgDirInfo, absPath, absolutePathKind); ok {
						if remapped == nil {
							return PathPair{Primary: logger.Path{Text: absPath, Namespace: "file", Flags: logger.PathDisabled}}, true, nil
						}
						if remappedResult, ok, diffCase := r.resolveWithoutRemapping(pkgDirInfo.enclosingBrowserScope, *remapped); ok {
							return remappedResult, true, diffCase
						}
					}
				}
			}

			if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
				return absolute, true, diffCase
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
		if absolute, ok, diffCase := r.loadAsFileOrDirectory(absPath); ok {
			return absolute, true, diffCase
		}
	}

	return PathPair{}, false, nil
}

func (r resolverQuery) finalizeImportsExportsResult(
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
	if (status == pjStatusExact || status == pjStatusInexact) && strings.HasPrefix(resolvedPath, "/") {
		absResolvedPath := r.fs.Join(absDirPath, resolvedPath[1:])

		switch status {
		case pjStatusExact:
			if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("The resolved path %q is exact", absResolvedPath))
			}
			resolvedDirInfo := r.dirInfoCached(r.fs.Dir(absResolvedPath))
			if resolvedDirInfo == nil {
				status = pjStatusModuleNotFound
			} else if entry, diffCase := resolvedDirInfo.entries.Get(r.fs.Base(absResolvedPath)); entry == nil {
				status = pjStatusModuleNotFound
			} else if kind := entry.Kind(r.fs); kind == fs.DirEntry {
				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("The path %q is a directory, which is not allowed", absResolvedPath))
				}
				status = pjStatusUnsupportedDirectoryImport
			} else if kind != fs.FileEntry {
				status = pjStatusModuleNotFound
			} else {
				if r.debugLogs != nil {
					r.debugLogs.addNote(fmt.Sprintf("Resolved to %q", absResolvedPath))
				}
				return PathPair{Primary: logger.Path{Text: absResolvedPath, Namespace: "file"}}, true, diffCase
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
			fmt.Sprintf("The module specifier %q is invalid:", resolvedPath))}

	case pjStatusInvalidPackageConfiguration:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			"The package configuration has an invalid value here:")}

	case pjStatusInvalidPackageTarget:
		why := fmt.Sprintf("The package target %q is invalid:", resolvedPath)
		if resolvedPath == "" {
			// "PACKAGE_TARGET_RESOLVE" is specified to throw an "Invalid
			// Package Target" error for what is actually an invalid package
			// configuration error
			why = "The package configuration has an invalid value here:"
		}
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token, why)}

	case pjStatusPackagePathNotExported:
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
					r.debugMeta.suggestionText = string(js_printer.QuoteForJSON(actualImportPath, false))
					r.debugMeta.suggestionMessage = fmt.Sprintf("Import from %q to get the file %q:",
						actualImportPath, r.PrettyPath(absolute.Primary))
				}
			}
		}

	case pjStatusPackageImportNotDefined:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("The package import %q is not defined in this \"imports\" map:", resolvedPath))}

	case pjStatusModuleNotFound:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("The module %q was not found on the file system:", resolvedPath))}

	case pjStatusUnsupportedDirectoryImport:
		r.debugMeta.notes = []logger.MsgData{tracker.MsgData(debug.token,
			fmt.Sprintf("Importing the directory %q is not supported:", resolvedPath))}

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
				fmt.Sprintf("None of the conditions provided (%s) match any of the currently active conditions (%s):",
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
				if !didSuggestEnablingCondition {
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
//   node --experimental-wasi-unstable-preview1 -p "[...require('module').builtinModules].join('\n')"
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
