package resolver

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

type packageJSON struct {
	source        logger.Source
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
	//
	// Note that the non-package "browser" map has to be checked twice to match
	// Webpack's behavior: once before resolution and once after resolution. It
	// leads to some unintuitive failure cases that we must emulate around missing
	// file extensions:
	//
	// * Given the mapping "./no-ext": "./no-ext-browser.js" the query "./no-ext"
	//   should match but the query "./no-ext.js" should NOT match.
	//
	// * Given the mapping "./ext.js": "./ext-browser.js" the query "./ext.js"
	//   should match and the query "./ext" should ALSO match.
	//
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

	// This represents the "exports" field in this package.json file.
	exportsMap *peMap

	hasNativeBindings bool
}

func (r *resolver) parsePackageJSON(path string) *packageJSON {
	packageJSONPath := r.fs.Join(path, "package.json")
	contents, err := r.caches.FSCache.ReadFile(r.fs, packageJSONPath)
	if err != nil {
		r.log.AddError(nil, logger.Loc{},
			fmt.Sprintf("Cannot read file %q: %s",
				r.PrettyPath(logger.Path{Text: packageJSONPath, Namespace: "file"}), err.Error()))
		return nil
	}

	keyPath := logger.Path{Text: packageJSONPath, Namespace: "file"}
	jsonSource := logger.Source{
		KeyPath:    keyPath,
		PrettyPath: r.PrettyPath(keyPath),
		Contents:   contents,
	}

	json, ok := r.caches.JSONCache.Parse(r.log, jsonSource, js_parser.JSONOptions{})
	if !ok {
		return nil
	}

	toAbsPath := func(pathText string, pathRange logger.Range) *string {
		// Is it a file?
		if absolute, ok, _ := r.loadAsFile(pathText, r.options.ExtensionOrder); ok {
			return &absolute
		}

		// Is it a directory?
		if mainEntries, err := r.fs.ReadDirectory(pathText); err == nil {
			// Look for an "index" file with known extensions
			if absolute, ok, _ := r.loadAsIndex(pathText, mainEntries); ok {
				return &absolute
			}
		} else if err != syscall.ENOENT {
			r.log.AddRangeError(&jsonSource, pathRange,
				fmt.Sprintf("Cannot read directory %q: %s",
					r.PrettyPath(logger.Path{Text: pathText, Namespace: "file"}), err.Error()))
		}
		return nil
	}

	packageJSON := &packageJSON{source: jsonSource}

	// Read the "main" fields
	mainFields := r.options.MainFields
	if mainFields == nil {
		mainFields = defaultMainFields[r.options.Platform]
	}
	for _, field := range mainFields {
		if mainJSON, _, ok := getProperty(json, field); ok {
			if main, ok := getString(mainJSON); ok {
				if packageJSON.absMainFields == nil {
					packageJSON.absMainFields = make(map[string]string)
				}
				if absPath := toAbsPath(r.fs.Join(path, main), jsonSource.RangeOfString(mainJSON.Loc)); absPath != nil {
					packageJSON.absMainFields[field] = *absPath
				}
			}
		}
	}

	// Look for native module markers in "devDependencies"
	if devDependenciesJSON, _, ok := getProperty(json, "devDependencies"); ok {
		if devDependencies, ok := devDependenciesJSON.Data.(*js_ast.EObject); ok {
			for _, prop := range devDependencies.Properties {
				if dependencyName, ok := getString(prop.Key); ok {
					if NativeModuleMarkers[dependencyName] {
						packageJSON.hasNativeBindings = true

						break
					}
				}
			}
		}
	}

	if !packageJSON.hasNativeBindings {
		// Look for native module markers in "dependencies"
		if dependenciesJSON, _, ok := getProperty(json, "dependencies"); ok {
			if dependencies, ok := dependenciesJSON.Data.(*js_ast.EObject); ok {
				for _, prop := range dependencies.Properties {
					if dependencyName, ok := getString(prop.Key); ok {
						if NativeModuleMarkers[dependencyName] {
							packageJSON.hasNativeBindings = true

							break
						}
					}
				}
			}
		}
	}

	// Read the "browser" property, but only when targeting the browser
	if browserJSON, _, ok := getProperty(json, "browser"); ok && r.options.Platform == config.PlatformBrowser {
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
		if browser, ok := browserJSON.Data.(*js_ast.EObject); ok {
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
	if sideEffectsJSON, sideEffectsLoc, ok := getProperty(json, "sideEffects"); ok {
		switch data := sideEffectsJSON.Data.(type) {
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
			for _, itemJSON := range data.Items {
				item, ok := itemJSON.Data.(*js_ast.EString)
				if !ok || item.Value == nil {
					r.log.AddWarning(&jsonSource, itemJSON.Loc,
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
			r.log.AddWarning(&jsonSource, sideEffectsJSON.Loc,
				"The value for \"sideEffects\" must be a boolean or an array")
		}
	}

	// Read the "exports" map
	if exportsJSON, exportsRange, ok := getProperty(json, "exports"); ok {
		if exportsMap := parseExportsMap(jsonSource, r.log, exportsJSON); exportsMap != nil {
			exportsMap.exportsRange = jsonSource.RangeOfString(exportsRange)
			packageJSON.exportsMap = exportsMap
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

// Reference: https://nodejs.org/api/esm.html#esm_resolver_algorithm_specification
type peMap struct {
	exportsRange logger.Range
	root         peEntry
}

type peKind uint8

const (
	peNull peKind = iota
	peString
	peArray
	peObject
	peInvalid
)

type peEntry struct {
	strData       string
	arrData       []peEntry
	mapData       []peMapEntry // Can't be a "map" because order matters
	expansionKeys expansionKeysArray
	firstToken    logger.Range
	kind          peKind
}

type peMapEntry struct {
	key      string
	keyRange logger.Range
	value    peEntry
}

// This type is just so we can use Go's native sort function
type expansionKeysArray []peMapEntry

func (a expansionKeysArray) Len() int          { return len(a) }
func (a expansionKeysArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a expansionKeysArray) Less(i int, j int) bool {
	return len(a[i].key) > len(a[j].key)
}

func (entry peEntry) valueForKey(key string) (peEntry, bool) {
	for _, item := range entry.mapData {
		if item.key == key {
			return item.value, true
		}
	}
	return peEntry{}, false
}

func parseExportsMap(source logger.Source, log logger.Log, json js_ast.Expr) *peMap {
	var visit func(expr js_ast.Expr) peEntry

	visit = func(expr js_ast.Expr) peEntry {
		var firstToken logger.Range

		switch e := expr.Data.(type) {
		case *js_ast.ENull:
			return peEntry{
				kind:       peNull,
				firstToken: js_lexer.RangeOfIdentifier(source, expr.Loc),
			}

		case *js_ast.EString:
			return peEntry{
				kind:       peString,
				firstToken: source.RangeOfString(expr.Loc),
				strData:    js_lexer.UTF16ToString(e.Value),
			}

		case *js_ast.EArray:
			arrData := make([]peEntry, len(e.Items))
			for i, item := range e.Items {
				arrData[i] = visit(item)
			}
			return peEntry{
				kind:       peArray,
				firstToken: logger.Range{Loc: expr.Loc, Len: 1},
				arrData:    arrData,
			}

		case *js_ast.EObject:
			mapData := make([]peMapEntry, len(e.Properties))
			expansionKeys := make(expansionKeysArray, 0, len(e.Properties))
			firstToken := logger.Range{Loc: expr.Loc, Len: 1}
			isConditionalSugar := false

			for i, property := range e.Properties {
				keyStr, _ := property.Key.Data.(*js_ast.EString)
				key := js_lexer.UTF16ToString(keyStr.Value)
				keyRange := source.RangeOfString(property.Key.Loc)

				// If exports is an Object with both a key starting with "." and a key
				// not starting with ".", throw an Invalid Package Configuration error.
				curIsConditionalSugar := !strings.HasPrefix(key, ".")
				if i == 0 {
					isConditionalSugar = curIsConditionalSugar
				} else if isConditionalSugar != curIsConditionalSugar {
					prevEntry := mapData[i-1]
					log.AddRangeWarningWithNotes(&source, keyRange,
						"This object cannot contain keys that both start with \".\" and don't start with \".\"",
						[]logger.MsgData{logger.RangeData(&source, prevEntry.keyRange,
							fmt.Sprintf("The previous key %q is incompatible with the current key %q", prevEntry.key, key))})
					return peEntry{
						kind:       peInvalid,
						firstToken: firstToken,
					}
				}

				entry := peMapEntry{
					key:      key,
					keyRange: keyRange,
					value:    visit(*property.Value),
				}

				if strings.HasSuffix(key, "/") || strings.HasSuffix(key, "*") {
					expansionKeys = append(expansionKeys, entry)
				}

				mapData[i] = entry
			}

			// Let expansionKeys be the list of keys of matchObj ending in "/" or "*",
			// sorted by length descending.
			sort.Stable(expansionKeys)

			return peEntry{
				kind:          peObject,
				firstToken:    firstToken,
				mapData:       mapData,
				expansionKeys: expansionKeys,
			}

		case *js_ast.EBoolean:
			firstToken = js_lexer.RangeOfIdentifier(source, expr.Loc)

		case *js_ast.ENumber:
			firstToken = source.RangeOfNumber(expr.Loc)

		default:
			firstToken.Loc = expr.Loc
		}

		log.AddRangeWarning(&source, firstToken, "This value must be a string, an object, an array, or null")
		return peEntry{
			kind:       peInvalid,
			firstToken: firstToken,
		}
	}

	root := visit(json)

	if root.kind == peNull {
		return nil
	}

	return &peMap{root: root}
}

func (entry peEntry) keysStartWithDot() bool {
	return len(entry.mapData) > 0 && strings.HasPrefix(entry.mapData[0].key, ".")
}

type peStatus uint8

const (
	peStatusUndefined peStatus = iota
	peStatusNull
	peStatusExact
	peStatusInexact // This means we may need to try CommonJS-style extension suffixes

	// Module specifier is an invalid URL, package name or package subpath specifier.
	peStatusInvalidModuleSpecifier

	// package.json configuration is invalid or contains an invalid configuration.
	peStatusInvalidPackageConfiguration

	// Package exports or imports define a target module for the package that is an invalid type or string target.
	peStatusInvalidPackageTarget

	// Package exports do not define or permit a target subpath in the package for the given module.
	peStatusPackagePathNotExported

	// The package or module requested does not exist.
	peStatusModuleNotFound

	// The resolved path corresponds to a directory, which is not a supported target for module imports.
	peStatusUnsupportedDirectoryImport
)

func esmPackageExportsResolveWithPostConditions(
	packageURL string,
	subpath string,
	exports peEntry,
	conditions map[string]bool,
) (string, peStatus, logger.Range) {
	resolved, status, token := esmPackageExportsResolve(packageURL, subpath, exports, conditions)
	if status != peStatusExact && status != peStatusInexact {
		return resolved, status, token
	}

	// If resolved contains any percent encodings of "/" or "\" ("%2f" and "%5C"
	// respectively), then throw an Invalid Module Specifier error.
	resolvedPath, err := url.PathUnescape(resolved)
	if err != nil {
		return resolved, peStatusInvalidModuleSpecifier, token
	}
	if strings.Contains(resolved, "%2f") || strings.Contains(resolved, "%2F") ||
		strings.Contains(resolved, "%5c") || strings.Contains(resolved, "%5C") {
		return resolved, peStatusInvalidModuleSpecifier, token
	}

	// If the file at resolved is a directory, then throw an Unsupported Directory
	// Import error.
	if strings.HasSuffix(resolvedPath, "/") || strings.HasSuffix(resolvedPath, "\\") {
		return resolved, peStatusUnsupportedDirectoryImport, token
	}

	// Set resolved to the real path of resolved.
	return resolvedPath, status, token
}

func esmPackageExportsResolve(
	packageURL string,
	subpath string,
	exports peEntry,
	conditions map[string]bool,
) (string, peStatus, logger.Range) {
	if exports.kind == peInvalid {
		return "", peStatusInvalidPackageConfiguration, exports.firstToken
	}
	if subpath == "." {
		mainExport := peEntry{kind: peNull}
		if exports.kind == peString || exports.kind == peArray || (exports.kind == peObject && !exports.keysStartWithDot()) {
			mainExport = exports
		} else if exports.kind == peObject {
			if dot, ok := exports.valueForKey("."); ok {
				mainExport = dot
			}
		}
		if mainExport.kind != peNull {
			resolved, status, token := esmPackageTargetResolve(packageURL, mainExport, "", false, conditions)
			if status != peStatusNull && status != peStatusUndefined {
				return resolved, status, token
			}
		}
	} else if exports.kind == peObject && exports.keysStartWithDot() {
		resolved, status, token := esmPackageImportsExportsResolve(subpath, exports, packageURL, conditions)
		if status != peStatusNull && status != peStatusUndefined {
			return resolved, status, token
		}
	}
	return "", peStatusPackagePathNotExported, exports.firstToken
}

func esmPackageImportsExportsResolve(
	matchKey string,
	matchObj peEntry,
	packageURL string,
	conditions map[string]bool,
) (string, peStatus, logger.Range) {
	if !strings.HasSuffix(matchKey, "*") {
		if target, ok := matchObj.valueForKey(matchKey); ok {
			return esmPackageTargetResolve(packageURL, target, "", false, conditions)
		}
	}

	for _, expansion := range matchObj.expansionKeys {
		// If expansionKey ends in "*" and matchKey starts with but is not equal to
		// the substring of expansionKey excluding the last "*" character
		if strings.HasSuffix(expansion.key, "*") {
			if substr := expansion.key[:len(expansion.key)-1]; strings.HasPrefix(matchKey, substr) && matchKey != substr {
				target := expansion.value
				subpath := matchKey[len(expansion.key)-1:]
				return esmPackageTargetResolve(packageURL, target, subpath, true, conditions)
			}
		}

		if strings.HasPrefix(matchKey, expansion.key) {
			target := expansion.value
			subpath := matchKey[len(expansion.key):]
			result, status, token := esmPackageTargetResolve(packageURL, target, subpath, false, conditions)
			if status == peStatusExact {
				// Return the object { resolved, exact: false }.
				status = peStatusInexact
			}
			return result, status, token
		}
	}

	return "", peStatusNull, matchObj.firstToken
}

// If path split on "/" or "\" contains any ".", ".." or "node_modules"
// segments after the first segment, throw an Invalid Package Target error.
func hasInvalidSegment(path string) bool {
	slash := strings.IndexAny(path, "/\\")
	if slash == -1 {
		return false
	}
	path = path[slash+1:]
	for path != "" {
		slash := strings.IndexAny(path, "/\\")
		segment := path
		if slash != -1 {
			segment = path[:slash]
			path = path[slash+1:]
		} else {
			path = ""
		}
		if segment == "." || segment == ".." || segment == "node_modules" {
			return true
		}
	}
	return false
}

func esmPackageTargetResolve(
	packageURL string,
	target peEntry,
	subpath string,
	pattern bool,
	conditions map[string]bool,
) (string, peStatus, logger.Range) {
	switch target.kind {
	case peString:
		// If pattern is false, subpath has non-zero length and target
		// does not end with "/", throw an Invalid Module Specifier error.
		if !pattern && subpath != "" && !strings.HasSuffix(target.strData, "/") {
			return target.strData, peStatusInvalidModuleSpecifier, target.firstToken
		}

		if !strings.HasPrefix(target.strData, "./") {
			return target.strData, peStatusInvalidPackageTarget, target.firstToken
		}

		// If target split on "/" or "\" contains any ".", ".." or "node_modules"
		// segments after the first segment, throw an Invalid Package Target error.
		if hasInvalidSegment(target.strData) {
			return target.strData, peStatusInvalidPackageTarget, target.firstToken
		}

		// Let resolvedTarget be the URL resolution of the concatenation of packageURL and target.
		resolvedTarget := path.Join(packageURL, target.strData)

		// If subpath split on "/" or "\" contains any ".", ".." or "node_modules"
		// segments, throw an Invalid Module Specifier error.
		if hasInvalidSegment(subpath) {
			return subpath, peStatusInvalidModuleSpecifier, target.firstToken
		}

		if pattern {
			// Return the URL resolution of resolvedTarget with every instance of "*" replaced with subpath.
			return strings.ReplaceAll(resolvedTarget, "*", subpath), peStatusExact, target.firstToken
		} else {
			// Return the URL resolution of the concatenation of subpath and resolvedTarget.
			return path.Join(resolvedTarget, subpath), peStatusExact, target.firstToken
		}

	case peObject:
		for _, p := range target.mapData {
			if p.key == "default" || conditions[p.key] {
				targetValue := p.value
				resolved, status, token := esmPackageTargetResolve(packageURL, targetValue, subpath, pattern, conditions)
				if status == peStatusUndefined {
					continue
				}
				return resolved, status, token
			}
		}
		return "", peStatusUndefined, target.firstToken

	case peArray:
		if len(target.arrData) == 0 {
			return "", peStatusNull, target.firstToken
		}
		lastException := peStatusUndefined
		lastToken := target.firstToken
		for _, targetValue := range target.arrData {
			// Let resolved be the result, continuing the loop on any Invalid Package Target error.
			resolved, status, token := esmPackageTargetResolve(packageURL, targetValue, subpath, pattern, conditions)
			if status == peStatusInvalidPackageTarget || status == peStatusNull {
				lastException = status
				lastToken = token
				continue
			}
			if status == peStatusUndefined {
				continue
			}
			return resolved, status, token
		}

		// Return or throw the last fallback resolution null return or error.
		return "", lastException, lastToken

	case peNull:
		return "", peStatusNull, target.firstToken
	}

	return "", peStatusInvalidPackageTarget, target.firstToken
}

func esmParsePackageName(packageSpecifier string) (packageName string, packageSubpath string, ok bool) {
	if packageSpecifier == "" {
		return
	}

	slash := strings.IndexByte(packageSpecifier, '/')
	if !strings.HasPrefix(packageSpecifier, "@") {
		if slash == -1 {
			slash = len(packageSpecifier)
		}
		packageName = packageSpecifier[:slash]
	} else {
		if slash == -1 {
			return
		}
		slash2 := strings.IndexByte(packageSpecifier[slash+1:], '/')
		if slash2 == -1 {
			slash2 = len(packageSpecifier[slash+1:])
		}
		packageName = packageSpecifier[:slash]
	}

	if strings.HasPrefix(packageName, ".") || strings.ContainsAny(packageName, "\\%") {
		return
	}

	packageSubpath = "." + packageSpecifier[len(packageName):]
	ok = true
	return
}

// If a module has any of these as dependencies, it likely has native bindings
var NativeModuleMarkers = map[string]bool{
	"bindings":       true,
	"nan":            true,
	"node-gyp-build": true,
	"node-pre-gyp":   true,
	"prebuild":       true,
}
