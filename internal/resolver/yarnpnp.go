package resolver

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

// This file implements the Yarn PnP specification: https://yarnpkg.com/advanced/pnp-spec/

type pnpData struct {
	// A list of package locators that are roots of the dependency tree. There
	// will typically be one entry for each workspace in the project (always at
	// least one, as the top-level package is a workspace by itself).
	dependencyTreeRoots map[string]string

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
	ignorePatternData *regexp.Regexp

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
}

type pnpPackage struct {
	packageDependencies map[string]pnpIdentAndReference
	packageLocation     string
	discardFromLookup   bool
}

type pnpPackageLocatorByLocation struct {
	locator           pnpIdentAndReference
	discardFromLookup bool
}

// Note: If this returns successfully then the node module resolution algorithm
// (i.e. NM_RESOLVE in the Yarn PnP specification) is always run afterward
func (r resolverQuery) pnpResolve(specifier string, parentURL string, parentManifest *pnpData) (string, bool) {
	// If specifier is a Node.js builtin, then
	if BuiltInNodeModules[specifier] {
		// Set resolved to specifier itself and return it
		return specifier, true
	}

	// Otherwise, if `specifier` is either an absolute path or a path prefixed with "./" or "../", then
	if r.fs.IsAbs(specifier) || strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") {
		// Set resolved to NM_RESOLVE(specifier, parentURL) and return it
		return specifier, true
	}

	// Otherwise,
	// Note: specifier is now a bare identifier
	// Let unqualified be RESOLVE_TO_UNQUALIFIED(specifier, parentURL)
	// Set resolved to NM_RESOLVE(unqualified, parentURL)
	return r.resolveToUnqualified(specifier, parentURL, parentManifest)
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

func (r resolverQuery) resolveToUnqualified(specifier string, parentURL string, manifest *pnpData) (string, bool) {
	// Let resolved be undefined

	// Let ident and modulePath be the result of PARSE_BARE_IDENTIFIER(specifier)
	ident, modulePath, ok := parseBareIdentifier(specifier)
	if !ok {
		return "", false
	}

	// Let manifest be FIND_PNP_MANIFEST(parentURL)
	// (this is already done by the time we get here)

	// If manifest is null, then
	// Set resolved to NM_RESOLVE(specifier, parentURL) and return it
	if manifest == nil {
		return specifier, true
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("Using Yarn PnP manifest from %q to resolve %q", manifest.absPath, ident))
	}

	// Let parentLocator be FIND_LOCATOR(manifest, parentURL)
	parentLocator, ok := r.findLocator(manifest, parentURL)

	// If parentLocator is null, then
	// Set resolved to NM_RESOLVE(specifier, parentURL) and return it
	if !ok {
		return specifier, true
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Found parent locator: [%s, %s]", quoteOrNullIfEmpty(parentLocator.ident), quoteOrNullIfEmpty(parentLocator.reference)))
	}

	// Let parentPkg be GET_PACKAGE(manifest, parentLocator)
	parentPkg, ok := r.getPackage(manifest, parentLocator.ident, parentLocator.reference)
	if !ok {
		// We aren't supposed to get here according to the Yarn PnP specification
		return "", false
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
			if set, _ := manifest.fallbackExclusionList[parentLocator.ident]; !set[parentLocator.reference] {
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
		return "", false
	}

	// If referenceOrAlias is still null, then
	if referenceOrAlias.reference == "" {
		// Note: It means that parentPkg has an unfulfilled peer dependency on ident
		// Throw a resolution error
		return "", false
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
			return "", false
		}
	} else {
		// Otherwise,
		// Let dependencyPkg be GET_PACKAGE(manifest, {ident, reference})
		dependencyPkg, ok = r.getPackage(manifest, ident, referenceOrAlias.reference)
		if !ok {
			// We aren't supposed to get here according to the Yarn PnP specification
			return "", false
		}
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Found package %q at %q", ident, dependencyPkg.packageLocation))
	}

	// Return path.resolve(manifest.dirPath, dependencyPkg.packageLocation, modulePath)
	result := r.fs.Join(manifest.absDirPath, dependencyPkg.packageLocation, modulePath)
	if !strings.HasSuffix(result, "/") && ((modulePath != "" && strings.HasSuffix(modulePath, "/")) ||
		(modulePath == "" && strings.HasSuffix(dependencyPkg.packageLocation, "/"))) {
		result += "/" // This is important for matching Yarn PnP's expectations in tests
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Resolved %q via Yarn PnP to %q", specifier, result))
	}
	return result, true
}

func (r resolverQuery) findLocator(manifest *pnpData, moduleUrl string) (pnpIdentAndReference, bool) {
	// Let relativeUrl be the relative path between manifest and moduleUrl
	relativeUrl, ok := r.fs.Rel(manifest.absDirPath, moduleUrl)
	if !ok {
		return pnpIdentAndReference{}, false
	}

	// The relative path must not start with ./; trim it if needed
	if strings.HasPrefix(relativeUrl, "./") {
		relativeUrl = relativeUrl[2:]
	}

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

	// Return it immediatly, whether it's defined or not
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

func compileYarnPnPData(absPath string, absDirPath string, json js_ast.Expr) *pnpData {
	data := pnpData{
		absPath:    absPath,
		absDirPath: absDirPath,
	}

	if value, _, ok := getProperty(json, "dependencyTreeRoots"); ok {
		if array, ok := value.Data.(*js_ast.EArray); ok {
			data.dependencyTreeRoots = make(map[string]string, len(array.Items))

			for _, item := range array.Items {
				if name, _, ok := getProperty(item, "name"); ok {
					if reference, _, ok := getProperty(item, "reference"); ok {
						if name, ok := getString(name); ok {
							if reference, ok := getString(reference); ok {
								data.dependencyTreeRoots[name] = reference
							}
						}
					}
				}
			}
		}
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
			data.ignorePatternData, _ = regexp.Compile(ignorePatternData)
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
															discardFromLookup:   discardFromLookup,
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
		return pnpIdentAndReference{}, true

	case *js_ast.EString:
		return pnpIdentAndReference{reference: helpers.UTF16ToString(d.Value)}, true

	case *js_ast.EArray:
		if len(d.Items) == 2 {
			if name, ok := getString(d.Items[0]); ok {
				if reference, ok := getString(d.Items[1]); ok {
					return pnpIdentAndReference{
						ident:     name,
						reference: reference,
					}, true
				}
			}
		}
	}

	return pnpIdentAndReference{}, false
}

func (r resolverQuery) extractYarnPnPDataFromJSON(pnpDataPath string, jsonCache *cache.JSONCache) (result js_ast.Expr) {
	contents, err, originalError := r.caches.FSCache.ReadFile(r.fs, pnpDataPath)
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read file %q: %s", pnpDataPath, originalError.Error()))
	}
	if err != nil {
		r.log.AddError(nil, logger.Range{},
			fmt.Sprintf("Cannot read file %q: %s",
				r.PrettyPath(logger.Path{Text: pnpDataPath, Namespace: "file"}), err.Error()))
		return
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("The file %q exists", pnpDataPath))
	}
	keyPath := logger.Path{Text: pnpDataPath, Namespace: "file"}
	source := logger.Source{
		KeyPath:    keyPath,
		PrettyPath: r.PrettyPath(keyPath),
		Contents:   contents,
	}
	result, _ = jsonCache.Parse(r.log, source, js_parser.JSONOptions{})
	return
}

func (r resolverQuery) tryToExtractYarnPnPDataFromJS(pnpDataPath string, jsonCache *cache.JSONCache) (result js_ast.Expr) {
	contents, err, originalError := r.caches.FSCache.ReadFile(r.fs, pnpDataPath)
	if r.debugLogs != nil && originalError != nil {
		r.debugLogs.addNote(fmt.Sprintf("Failed to read file %q: %s", pnpDataPath, originalError.Error()))
	}
	if err != nil {
		r.log.AddError(nil, logger.Range{},
			fmt.Sprintf("Cannot read file %q: %s",
				r.PrettyPath(logger.Path{Text: pnpDataPath, Namespace: "file"}), err.Error()))
		return
	}
	if r.debugLogs != nil {
		r.debugLogs.addNote(fmt.Sprintf("The file %q exists", pnpDataPath))
	}

	keyPath := logger.Path{Text: pnpDataPath, Namespace: "file"}
	source := logger.Source{
		KeyPath:    keyPath,
		PrettyPath: r.PrettyPath(keyPath),
		Contents:   contents,
	}
	ast, _ := js_parser.Parse(r.log, source, js_parser.OptionsForYarnPnP())

	if r.debugLogs != nil && ast.ManifestForYarnPnP.Data != nil {
		r.debugLogs.addNote(fmt.Sprintf("  Extracted JSON data from %q", pnpDataPath))
	}
	return ast.ManifestForYarnPnP
}
