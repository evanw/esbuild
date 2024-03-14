package resolver

// This file implements the Yarn PnP specification: https://yarnpkg.com/advanced/pnp-spec/

import (
	"fmt"
	"regexp"
	"strings"
	"syscall"

	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

type pnpData struct {
	// Keys are the package idents, values are sets of references. Combining the
	// ident with each individual reference yields the set of affected locators.
	fallbackExclusionList map[string]map[string]bool

	// A map of locators that all packages are allowed to access, regardless
	// whether they list them in their dependencies or not.
	fallbackPool map[string]pnpIdentAndReference

	// A nullable regexp. If set, all project-relative importer paths should be
	// matched against it. If the match succeeds, the resolution should follow
	// the classic Node.js resolution algorithm rather than the Plug'n'Play one.
	// Note that unlike other paths in the manifest, the one checked against this
	// regexp won't begin by `./`.
	ignorePatternData        *regexp.Regexp
	invalidIgnorePatternData string

	// This is the main part of the PnP data file. This table contains the list
	// of all packages, first keyed by package ident then by package reference.
	// One entry will have `null` in both fields and represents the absolute
	// top-level package.
	packageRegistryData map[string]map[string]pnpPackage

	packageLocatorsByLocations map[string]pnpPackageLocatorByLocation

	// If true, should a dependency resolution fail for an importer that isn't
	// explicitly listed in `fallbackExclusionList`, the runtime must first check
	// whether the resolution would succeed for any of the packages in
	// `fallbackPool`; if it would, transparently return this resolution. Note
	// that all dependencies from the top-level package are implicitly part of
	// the fallback pool, even if not listed here.
	enableTopLevelFallback bool

	tracker    logger.LineColumnTracker
	absPath    string
	absDirPath string
}

// This is called both a "locator" and a "dependency target" in the specification.
// When it's used as a dependency target, it can only be in one of three states:
//
//  1. A reference, to link with the dependency name
//     In this case ident is "".
//
//  2. An aliased package
//     In this case neither ident nor reference are "".
//
//  3. A missing peer dependency
//     In this case ident and reference are "".
type pnpIdentAndReference struct {
	ident     string // Empty if null
	reference string // Empty if null
	span      logger.Range
}

type pnpPackage struct {
	packageDependencies      map[string]pnpIdentAndReference
	packageLocation          string
	packageDependenciesRange logger.Range
	discardFromLookup        bool
}

type pnpPackageLocatorByLocation struct {
	locator           pnpIdentAndReference
	discardFromLookup bool
}

func parseBareIdentifier(specifier string) (ident string, modulePath string, ok bool) {
	slash := strings.IndexByte(specifier, '/')

	// If specifier starts with "@", then
	if strings.HasPrefix(specifier, "@") {
		// If specifier doesn't contain a "/" separator, then
		if slash == -1 {
			// Throw an error
			return
		}

		// Otherwise,
		// Set ident to the substring of specifier until the second "/" separator or the end of string, whatever happens first
		if slash2 := strings.IndexByte(specifier[slash+1:], '/'); slash2 != -1 {
			ident = specifier[:slash+1+slash2]
		} else {
			ident = specifier
		}
	} else {
		// Otherwise,
		// Set ident to the substring of specifier until the first "/" separator or the end of string, whatever happens first
		if slash != -1 {
			ident = specifier[:slash]
		} else {
			ident = specifier
		}
	}

	// Set modulePath to the substring of specifier starting from ident.length
	modulePath = specifier[len(ident):]

	// Return {ident, modulePath}
	ok = true
	return
}

type pnpStatus uint8

const (
	pnpErrorGeneric pnpStatus = iota
	pnpErrorDependencyNotFound
	pnpErrorUnfulfilledPeerDependency
	pnpSuccess
	pnpSkipped
)

func (status pnpStatus) isError() bool {
	return status < pnpSuccess
}

type pnpResult struct {
	status     pnpStatus
	pkgDirPath string
	pkgIdent   string
	pkgSubpath string

	// This is for error messages
	errorIdent string
	errorRange logger.Range
}

// Note: If this returns successfully then the node module resolution algorithm
// (i.e. NM_RESOLVE in the Yarn PnP specification) is always run afterward
func (r resolverQuery) resolveToUnqualified(specifier string, parentURL string, manifest *pnpData) pnpResult {
	// Let resolved be undefined

	// Let manifest be FIND_PNP_MANIFEST(parentURL)
	// (this is already done by the time we get here)
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Using Yarn PnP manifest from %q", manifest.absPath))
		r.debugLogs.addNote(fmt.Sprintf("  Resolving %q in %q", specifier, parentURL))
	}

	// Let ident and modulePath be the result of PARSE_BARE_IDENTIFIER(specifier)
	ident, modulePath, ok := parseBareIdentifier(specifier)
	if !ok {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("  Failed to parse specifier %q into a bare identifier", specifier))
		}
		return pnpResult{status: pnpErrorGeneric}
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Parsed bare identifier %q and module path %q", ident, modulePath))
	}

	// Let parentLocator be FIND_LOCATOR(manifest, parentURL)
	parentLocator, ok := r.findLocator(manifest, parentURL)

	// If parentLocator is null, then
	// Set resolved to NM_RESOLVE(specifier, parentURL) and return it
	if !ok {
		return pnpResult{status: pnpSkipped}
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Found parent locator: [%s, %s]", quoteOrNullIfEmpty(parentLocator.ident), quoteOrNullIfEmpty(parentLocator.reference)))
	}

	// Let parentPkg be GET_PACKAGE(manifest, parentLocator)
	parentPkg, ok := r.getPackage(manifest, parentLocator.ident, parentLocator.reference)
	if !ok {
		// We aren't supposed to get here according to the Yarn PnP specification
		return pnpResult{status: pnpErrorGeneric}
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Found parent package at %q", parentPkg.packageLocation))
	}

	// Let referenceOrAlias be the entry from parentPkg.packageDependencies referenced by ident
	referenceOrAlias, ok := parentPkg.packageDependencies[ident]

	// If referenceOrAlias is null or undefined, then
	if !ok || referenceOrAlias.reference == "" {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("  Failed to find %q in \"packageDependencies\" of parent package", ident))
		}

		// If manifest.enableTopLevelFallback is true, then
		if manifest.enableTopLevelFallback {
			if r.debugLogs != nil {
				r.debugLogs.addNote("  Searching for a fallback because \"enableTopLevelFallback\" is true")
			}

			// If parentLocator isn't in manifest.fallbackExclusionList, then
			if set := manifest.fallbackExclusionList[parentLocator.ident]; !set[parentLocator.reference] {
				// Let fallback be RESOLVE_VIA_FALLBACK(manifest, ident)
				fallback, _ := r.resolveViaFallback(manifest, ident)

				// If fallback is neither null nor undefined
				if fallback.reference != "" {
					// Set referenceOrAlias to fallback
					referenceOrAlias = fallback
					ok = true
				}
			} else if r.debugLogs != nil {
				r.debugLogs.addNote(fmt.Sprintf("    Stopping because [%s, %s] is in \"fallbackExclusionList\"",
					quoteOrNullIfEmpty(parentLocator.ident), quoteOrNullIfEmpty(parentLocator.reference)))
			}
		}
	}

	// If referenceOrAlias is still undefined, then
	if !ok {
		// Throw a resolution error
		return pnpResult{
			status:     pnpErrorDependencyNotFound,
			errorIdent: ident,
			errorRange: parentPkg.packageDependenciesRange,
		}
	}

	// If referenceOrAlias is still null, then
	if referenceOrAlias.reference == "" {
		// Note: It means that parentPkg has an unfulfilled peer dependency on ident
		// Throw a resolution error
		return pnpResult{
			status:     pnpErrorUnfulfilledPeerDependency,
			errorIdent: ident,
			errorRange: referenceOrAlias.span,
		}
	}

	if r.debugLogs != nil {
		var referenceOrAliasStr string
		if referenceOrAlias.ident != "" {
			referenceOrAliasStr = fmt.Sprintf("[%q, %q]", referenceOrAlias.ident, referenceOrAlias.reference)
		} else {
			referenceOrAliasStr = quoteOrNullIfEmpty(referenceOrAlias.reference)
		}
		r.debugLogs.addNote(fmt.Sprintf("  Found dependency locator: [%s, %s]", quoteOrNullIfEmpty(ident), referenceOrAliasStr))
	}

	// Otherwise, if referenceOrAlias is an array, then
	var dependencyPkg pnpPackage
	if referenceOrAlias.ident != "" {
		// Let alias be referenceOrAlias
		alias := referenceOrAlias

		// Let dependencyPkg be GET_PACKAGE(manifest, alias)
		dependencyPkg, ok = r.getPackage(manifest, alias.ident, alias.reference)
		if !ok {
			// We aren't supposed to get here according to the Yarn PnP specification
			return pnpResult{status: pnpErrorGeneric}
		}
	} else {
		// Otherwise,
		// Let dependencyPkg be GET_PACKAGE(manifest, {ident, reference})
		dependencyPkg, ok = r.getPackage(manifest, ident, referenceOrAlias.reference)
		if !ok {
			// We aren't supposed to get here according to the Yarn PnP specification
			return pnpResult{status: pnpErrorGeneric}
		}
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Found package %q at %q", ident, dependencyPkg.packageLocation))
	}

	// Return path.resolve(manifest.dirPath, dependencyPkg.packageLocation, modulePath)
	pkgDirPath := r.fs.Join(manifest.absDirPath, dependencyPkg.packageLocation)
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Resolved %q via Yarn PnP to %q with subpath %q", specifier, pkgDirPath, modulePath))
	}
	return pnpResult{
		status:     pnpSuccess,
		pkgDirPath: pkgDirPath,
		pkgIdent:   ident,
		pkgSubpath: modulePath,
	}
}

func (r resolverQuery) findLocator(manifest *pnpData, moduleUrl string) (pnpIdentAndReference, bool) {
	// Let relativeUrl be the relative path between manifest and moduleUrl
	relativeUrl, ok := r.fs.Rel(manifest.absDirPath, moduleUrl)
	if !ok {
		return pnpIdentAndReference{}, false
	} else {
		// Relative URLs on Windows will use \ instead of /, which will break
		// everything we do below. Use normal slashes to keep things working.
		relativeUrl = strings.ReplaceAll(relativeUrl, "\\", "/")
	}

	// The relative path must not start with ./; trim it if needed
	relativeUrl = strings.TrimPrefix(relativeUrl, "./")

	// If relativeUrl matches manifest.ignorePatternData, then
	if manifest.ignorePatternData != nil && manifest.ignorePatternData.MatchString(relativeUrl) {
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("  Ignoring %q because it matches \"ignorePatternData\"", relativeUrl))
		}

		// Return null
		return pnpIdentAndReference{}, false
	}

	// Note: Make sure relativeUrl always starts with a ./ or ../
	if !strings.HasSuffix(relativeUrl, "/") {
		relativeUrl += "/"
	}
	if !strings.HasPrefix(relativeUrl, "./") && !strings.HasPrefix(relativeUrl, "../") {
		relativeUrl = "./" + relativeUrl
	}

	// This is the inner loop from Yarn's PnP resolver implementation. This is
	// different from the specification, which contains a hypothetical slow
	// algorithm instead. The algorithm from the specification can sometimes
	// produce different results from the one used by the implementation, so
	// we follow the implementation.
	for {
		entry, ok := manifest.packageLocatorsByLocations[relativeUrl]
		if !ok || entry.discardFromLookup {
			// Remove the last path component and try again
			relativeUrl = relativeUrl[:strings.LastIndexByte(relativeUrl[:len(relativeUrl)-1], '/')+1]
			if relativeUrl == "" {
				break
			}
			continue
		}
		return entry.locator, true
	}

	return pnpIdentAndReference{}, false
}

func (r resolverQuery) resolveViaFallback(manifest *pnpData, ident string) (pnpIdentAndReference, bool) {
	// Let topLevelPkg be GET_PACKAGE(manifest, {null, null})
	topLevelPkg, ok := r.getPackage(manifest, "", "")
	if !ok {
		// We aren't supposed to get here according to the Yarn PnP specification
		return pnpIdentAndReference{}, false
	}

	// Let referenceOrAlias be the entry from topLevelPkg.packageDependencies referenced by ident
	referenceOrAlias, ok := topLevelPkg.packageDependencies[ident]

	// If referenceOrAlias is defined, then
	if ok {
		// Return it immediately
		if r.debugLogs != nil {
			r.debugLogs.addNote(fmt.Sprintf("    Found fallback for %q in \"packageDependencies\" of top-level package: [%s, %s]", ident,
				quoteOrNullIfEmpty(referenceOrAlias.ident), quoteOrNullIfEmpty(referenceOrAlias.reference)))
		}
		return referenceOrAlias, true
	}

	// Otherwise,
	// Let referenceOrAlias be the entry from manifest.fallbackPool referenced by ident
	referenceOrAlias, ok = manifest.fallbackPool[ident]

	// Return it immediately, whether it's defined or not
	if r.debugLogs != nil {
		if ok {
			r.debugLogs.addNote(fmt.Sprintf("    Found fallback for %q in \"fallbackPool\": [%s, %s]", ident,
				quoteOrNullIfEmpty(referenceOrAlias.ident), quoteOrNullIfEmpty(referenceOrAlias.reference)))
		} else {
			r.debugLogs.addNote(fmt.Sprintf("    Failed to find fallback for %q in \"fallbackPool\"", ident))
		}
	}
	return referenceOrAlias, ok
}

func (r resolverQuery) getPackage(manifest *pnpData, ident string, reference string) (pnpPackage, bool) {
	if inner, ok := manifest.packageRegistryData[ident]; ok {
		if pkg, ok := inner[reference]; ok {
			return pkg, true
		}
	}

	if r.debugLogs != nil {
		// We aren't supposed to get here according to the Yarn PnP specification:
		// "Note: pkg cannot be undefined here; all packages referenced in any of the
		// Plug'n'Play data tables MUST have a corresponding entry inside packageRegistryData."
		r.debugLogs.addNote(fmt.Sprintf("  Yarn PnP invariant violation: GET_PACKAGE failed to find a package: [%s, %s]",
			quoteOrNullIfEmpty(ident), quoteOrNullIfEmpty(reference)))
	}
	return pnpPackage{}, false
}

func quoteOrNullIfEmpty(str string) string {
	if str != "" {
		return fmt.Sprintf("%q", str)
	}
	return "null"
}

func compileYarnPnPData(absPath string, absDirPath string, json js_ast.Expr, source logger.Source) *pnpData {
	data := pnpData{
		absPath:    absPath,
		absDirPath: absDirPath,
		tracker:    logger.MakeLineColumnTracker(&source),
	}

	if value, _, ok := getProperty(json, "enableTopLevelFallback"); ok {
		if enableTopLevelFallback, ok := getBool(value); ok {
			data.enableTopLevelFallback = enableTopLevelFallback
		}
	}

	if value, _, ok := getProperty(json, "fallbackExclusionList"); ok {
		if array, ok := value.Data.(*js_ast.EArray); ok {
			data.fallbackExclusionList = make(map[string]map[string]bool, len(array.Items))

			for _, item := range array.Items {
				if tuple, ok := item.Data.(*js_ast.EArray); ok && len(tuple.Items) == 2 {
					if ident, ok := getStringOrNull(tuple.Items[0]); ok {
						if array2, ok := tuple.Items[1].Data.(*js_ast.EArray); ok {
							references := make(map[string]bool, len(array2.Items))

							for _, item2 := range array2.Items {
								if reference, ok := getString(item2); ok {
									references[reference] = true
								}
							}

							data.fallbackExclusionList[ident] = references
						}
					}
				}
			}
		}
	}

	if value, _, ok := getProperty(json, "fallbackPool"); ok {
		if array, ok := value.Data.(*js_ast.EArray); ok {
			data.fallbackPool = make(map[string]pnpIdentAndReference, len(array.Items))

			for _, item := range array.Items {
				if array2, ok := item.Data.(*js_ast.EArray); ok && len(array2.Items) == 2 {
					if ident, ok := getString(array2.Items[0]); ok {
						if dependencyTarget, ok := getDependencyTarget(array2.Items[1]); ok {
							data.fallbackPool[ident] = dependencyTarget
						}
					}
				}
			}
		}
	}

	if value, _, ok := getProperty(json, "ignorePatternData"); ok {
		if ignorePatternData, ok := getString(value); ok {
			// The Go regular expression engine doesn't support some of the features
			// that JavaScript regular expressions support, including "(?!" negative
			// lookaheads which Yarn uses. This is deliberate on Go's part. See this:
			// https://github.com/golang/go/issues/18868.
			//
			// Yarn uses this feature to exclude the "." and ".." path segments in
			// the middle of a relative path. However, we shouldn't ever generate
			// such path segments in the first place. So as a hack, we just remove
			// the specific character sequences used by Yarn for this so that the
			// regular expression is more likely to be able to be compiled.
			ignorePatternData = strings.ReplaceAll(ignorePatternData, `(?!\.)`, "")
			ignorePatternData = strings.ReplaceAll(ignorePatternData, `(?!(?:^|\/)\.)`, "")
			ignorePatternData = strings.ReplaceAll(ignorePatternData, `(?!\.{1,2}(?:\/|$))`, "")
			ignorePatternData = strings.ReplaceAll(ignorePatternData, `(?!(?:^|\/)\.{1,2}(?:\/|$))`, "")

			if reg, err := regexp.Compile(ignorePatternData); err == nil {
				data.ignorePatternData = reg
			} else {
				data.invalidIgnorePatternData = ignorePatternData
			}
		}
	}

	if value, _, ok := getProperty(json, "packageRegistryData"); ok {
		if array, ok := value.Data.(*js_ast.EArray); ok {
			data.packageRegistryData = make(map[string]map[string]pnpPackage, len(array.Items))
			data.packageLocatorsByLocations = make(map[string]pnpPackageLocatorByLocation)

			for _, item := range array.Items {
				if tuple, ok := item.Data.(*js_ast.EArray); ok && len(tuple.Items) == 2 {
					if packageIdent, ok := getStringOrNull(tuple.Items[0]); ok {
						if array2, ok := tuple.Items[1].Data.(*js_ast.EArray); ok {
							references := make(map[string]pnpPackage, len(array2.Items))
							data.packageRegistryData[packageIdent] = references

							for _, item2 := range array2.Items {
								if tuple2, ok := item2.Data.(*js_ast.EArray); ok && len(tuple2.Items) == 2 {
									if packageReference, ok := getStringOrNull(tuple2.Items[0]); ok {
										pkg := tuple2.Items[1]

										if packageLocation, _, ok := getProperty(pkg, "packageLocation"); ok {
											if packageDependencies, _, ok := getProperty(pkg, "packageDependencies"); ok {
												if packageLocation, ok := getString(packageLocation); ok {
													if array3, ok := packageDependencies.Data.(*js_ast.EArray); ok {
														deps := make(map[string]pnpIdentAndReference, len(array3.Items))
														discardFromLookup := false

														for _, dep := range array3.Items {
															if array4, ok := dep.Data.(*js_ast.EArray); ok && len(array4.Items) == 2 {
																if ident, ok := getString(array4.Items[0]); ok {
																	if dependencyTarget, ok := getDependencyTarget(array4.Items[1]); ok {
																		deps[ident] = dependencyTarget
																	}
																}
															}
														}

														if value, _, ok := getProperty(pkg, "discardFromLookup"); ok {
															if value, ok := getBool(value); ok {
																discardFromLookup = value
															}
														}

														references[packageReference] = pnpPackage{
															packageLocation:     packageLocation,
															packageDependencies: deps,
															packageDependenciesRange: logger.Range{
																Loc: packageDependencies.Loc,
																Len: array3.CloseBracketLoc.Start + 1 - packageDependencies.Loc.Start,
															},
															discardFromLookup: discardFromLookup,
														}

														// This is what Yarn's PnP implementation does (specifically in
														// "hydrateRuntimeState"), so we replicate that behavior here:
														if entry, ok := data.packageLocatorsByLocations[packageLocation]; !ok {
															data.packageLocatorsByLocations[packageLocation] = pnpPackageLocatorByLocation{
																locator:           pnpIdentAndReference{ident: packageIdent, reference: packageReference},
																discardFromLookup: discardFromLookup,
															}
														} else {
															entry.discardFromLookup = entry.discardFromLookup && discardFromLookup
															if !discardFromLookup {
																entry.locator = pnpIdentAndReference{ident: packageIdent, reference: packageReference}
															}
															data.packageLocatorsByLocations[packageLocation] = entry
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
			}
		}
	}

	return &data
}

func getStringOrNull(json js_ast.Expr) (string, bool) {
	switch value := json.Data.(type) {
	case *js_ast.EString:
		return helpers.UTF16ToString(value.Value), true

	case *js_ast.ENull:
		return "", true
	}

	return "", false
}

func getDependencyTarget(json js_ast.Expr) (pnpIdentAndReference, bool) {
	switch d := json.Data.(type) {
	case *js_ast.ENull:
		return pnpIdentAndReference{span: logger.Range{Loc: json.Loc, Len: 4}}, true

	case *js_ast.EString:
		return pnpIdentAndReference{reference: helpers.UTF16ToString(d.Value), span: logger.Range{Loc: json.Loc}}, true

	case *js_ast.EArray:
		if len(d.Items) == 2 {
			if name, ok := getString(d.Items[0]); ok {
				if reference, ok := getString(d.Items[1]); ok {
					return pnpIdentAndReference{
						ident:     name,
						reference: reference,
						span:      logger.Range{Loc: json.Loc, Len: d.CloseBracketLoc.Start + 1 - json.Loc.Start},
					}, true
				}
			}
		}
	}

	return pnpIdentAndReference{}, false
}

type pnpDataMode uint8

const (
	pnpIgnoreErrorsAboutMissingFiles pnpDataMode = iota
	pnpReportErrorsAboutMissingFiles
)

func (r resolverQuery) extractYarnPnPDataFromJSON(pnpDataPath string, mode pnpDataMode) (result js_ast.Expr, source logger.Source) {
	contents, err, originalError := r.caches.FSCache.ReadFile(r.fs, pnpDataPath)
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read file %q: %s", pnpDataPath, originalError.Error()))
	}
	if err != nil {
		if mode == pnpReportErrorsAboutMissingFiles || err != syscall.ENOENT {
			r.log.AddError(nil, logger.Range{},
				fmt.Sprintf("Cannot read file %q: %s",
					PrettyPath(r.fs, logger.Path{Text: pnpDataPath, Namespace: "file"}), err.Error()))
		}
		return
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("The file %q exists", pnpDataPath))
	}
	keyPath := logger.Path{Text: pnpDataPath, Namespace: "file"}
	source = logger.Source{
		KeyPath:    keyPath,
		PrettyPath: PrettyPath(r.fs, keyPath),
		Contents:   contents,
	}
	result, _ = r.caches.JSONCache.Parse(r.log, source, js_parser.JSONOptions{})
	return
}

func (r resolverQuery) tryToExtractYarnPnPDataFromJS(pnpDataPath string, mode pnpDataMode) (result js_ast.Expr, source logger.Source) {
	contents, err, originalError := r.caches.FSCache.ReadFile(r.fs, pnpDataPath)
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read file %q: %s", pnpDataPath, originalError.Error()))
	}
	if err != nil {
		if mode == pnpReportErrorsAboutMissingFiles || err != syscall.ENOENT {
			r.log.AddError(nil, logger.Range{},
				fmt.Sprintf("Cannot read file %q: %s",
					PrettyPath(r.fs, logger.Path{Text: pnpDataPath, Namespace: "file"}), err.Error()))
		}
		return
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("The file %q exists", pnpDataPath))
	}

	keyPath := logger.Path{Text: pnpDataPath, Namespace: "file"}
	source = logger.Source{
		KeyPath:    keyPath,
		PrettyPath: PrettyPath(r.fs, keyPath),
		Contents:   contents,
	}
	ast, _ := r.caches.JSCache.Parse(r.log, source, js_parser.OptionsForYarnPnP())

	if r.debugLogs != nil && ast.ManifestForYarnPnP.Data != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Extracted JSON data from %q", pnpDataPath))
	}
	return ast.ManifestForYarnPnP, source
}
