package resolver

import (
	"fmt"
	"regexp"
	"strings"
	"syscall"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

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

	packageJSON := &packageJSON{}

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
