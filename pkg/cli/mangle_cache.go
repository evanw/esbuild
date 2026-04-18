package cli

// The mangle cache is a JSON file that remembers esbuild's property renaming
// decisions. It's a flat map where the keys are strings and the values are
// either strings or the boolean value "false". This is the case both in JSON
// and in Go (so the "interface{}" values are also either strings or "false").
//
// Namespace caches are stored at the top level with a "#" prefix on the key
// (e.g. "#TypeA_"), since "#" cannot start a valid mangled property name.
// Their values are objects with the same string-or-false format.

import (
	"fmt"
	"sort"
	"strings"
	"syscall"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
)

func parseMangleCache(osArgs []string, fs fs.FS, absPath string) (map[string]interface{}, map[string]map[string]interface{}, []string) {
	// Log problems with the mangle cache to stderr
	log := logger.NewStderrLog(logger.OutputOptionsForArgs(osArgs))
	defer log.Done()

	// Try to read the existing file
	prettyPath := absPath
	if rel, ok := fs.Rel(fs.Cwd(), absPath); ok {
		prettyPath = rel
	}
	prettyPath = strings.ReplaceAll(prettyPath, "\\", "/")
	bytes, err, originalError := fs.ReadFile(absPath)
	if err != nil {
		// It's ok if it's just missing
		if err == syscall.ENOENT {
			return make(map[string]interface{}), nil, []string{}
		}

		// Otherwise, report the error
		log.AddError(nil, logger.Range{},
			fmt.Sprintf("Failed to read from mangle cache file %q: %s", prettyPath, originalError.Error()))
		return nil, nil, nil
	}

	// Use our JSON parser so we get pretty-printed error messages
	keyPath := logger.Path{Text: absPath, Namespace: "file"}
	source := logger.Source{
		KeyPath:     keyPath,
		PrettyPaths: resolver.MakePrettyPaths(fs, keyPath),
		Contents:    string(bytes),
	}
	result, ok := js_parser.ParseJSON(log, source, js_parser.JSONOptions{})
	if !ok || log.HasErrors() {
		// Stop if there were any errors so we don't continue and then overwrite this file
		return nil, nil, nil
	}
	tracker := logger.MakeLineColumnTracker(&source)

	// Validate the top-level object
	root, ok := result.Data.(*js_ast.EObject)
	if !ok {
		log.AddError(&tracker, logger.Range{Loc: result.Loc},
			"Expected a top-level object in mangle cache file")
		return nil, nil, nil
	}

	mangleCache := make(map[string]interface{}, len(root.Properties))
	var namespaceCaches map[string]map[string]interface{}
	order := make([]string, 0, len(root.Properties))

	for _, property := range root.Properties {
		key := helpers.UTF16ToString(property.Key.Data.(*js_ast.EString).Value)
		order = append(order, key)

		// Keys starting with "#" are namespace caches
		if strings.HasPrefix(key, "#") {
			nsKey := key[1:] // Strip the "#" prefix
			nsObj, ok := property.ValueOrNil.Data.(*js_ast.EObject)
			if !ok {
				log.AddError(&tracker, logger.Range{Loc: property.ValueOrNil.Loc},
					fmt.Sprintf("Expected %q in mangle cache file to be an object", key))
				continue
			}
			nsCache := make(map[string]interface{}, len(nsObj.Properties))
			for _, innerProp := range nsObj.Properties {
				innerKey := helpers.UTF16ToString(innerProp.Key.Data.(*js_ast.EString).Value)
				switch v := innerProp.ValueOrNil.Data.(type) {
				case *js_ast.EBoolean:
					if v.Value {
						log.AddError(&tracker, js_lexer.RangeOfIdentifier(source, innerProp.ValueOrNil.Loc),
							fmt.Sprintf("Expected %q in %q in mangle cache file to map to either a string or false", innerKey, key))
					} else {
						nsCache[innerKey] = false
					}
				case *js_ast.EString:
					nsCache[innerKey] = helpers.UTF16ToString(v.Value)
				default:
					log.AddError(&tracker, logger.Range{Loc: innerProp.ValueOrNil.Loc},
						fmt.Sprintf("Expected %q in %q in mangle cache file to map to either a string or false", innerKey, key))
				}
			}
			if namespaceCaches == nil {
				namespaceCaches = make(map[string]map[string]interface{})
			}
			namespaceCaches[nsKey] = nsCache
			continue
		}

		switch v := property.ValueOrNil.Data.(type) {
		case *js_ast.EBoolean:
			if v.Value {
				log.AddError(&tracker, js_lexer.RangeOfIdentifier(source, property.ValueOrNil.Loc),
					fmt.Sprintf("Expected %q in mangle cache file to map to either a string or false", key))
			} else {
				mangleCache[key] = false
			}

		case *js_ast.EString:
			mangleCache[key] = helpers.UTF16ToString(v.Value)

		default:
			log.AddError(&tracker, logger.Range{Loc: property.ValueOrNil.Loc},
				fmt.Sprintf("Expected %q in mangle cache file to map to either a string or false", key))
		}
	}

	if log.HasErrors() {
		return nil, nil, nil
	}
	return mangleCache, namespaceCaches, order
}

func printMangleCache(mangleCache map[string]interface{}, namespaceCaches map[string]map[string]interface{}, originalOrder []string, asciiOnly bool) []byte {
	j := helpers.Joiner{}
	j.AddString("{")

	// Build a combined map of all keys for ordering purposes.
	// Namespace keys are stored with a "#" prefix in the file.
	allKeys := make(map[string]bool, len(mangleCache)+len(namespaceCaches))
	for key := range mangleCache {
		allKeys[key] = true
	}
	for nsKey := range namespaceCaches {
		allKeys["#"+nsKey] = true
	}

	// Also preserve any "#"-prefixed keys from the original order that
	// might not be in the current namespace caches (e.g. emptied out)
	for _, key := range originalOrder {
		if strings.HasPrefix(key, "#") {
			allKeys[key] = true
		}
	}

	// Determine the order to print the keys in
	order := originalOrder
	if len(allKeys) > len(order) {
		order = make([]string, 0, len(allKeys))
		if sort.StringsAreSorted(originalOrder) {
			// If they came sorted, keep them sorted
			for key := range allKeys {
				order = append(order, key)
			}
			sort.Strings(order)
		} else {
			// Otherwise add all new keys to the end, and only sort the new keys
			originalKeySet := make(map[string]bool, len(originalOrder))
			for _, key := range originalOrder {
				originalKeySet[key] = true
			}
			order = append(order, originalOrder...)
			for key := range allKeys {
				if !originalKeySet[key] {
					order = append(order, key)
				}
			}
			sort.Strings(order[len(originalOrder):])
		}
	}

	// Print the JSON while preserving the existing order of the keys
	for i, key := range order {
		// Print the key
		if i > 0 {
			j.AddString(",\n  ")
		} else {
			j.AddString("\n  ")
		}
		j.AddBytes(helpers.QuoteForJSON(key, asciiOnly))

		// Handle namespace cache keys (prefixed with "#")
		if strings.HasPrefix(key, "#") {
			nsKey := key[1:]
			nsCache := namespaceCaches[nsKey]
			if len(nsCache) > 0 {
				j.AddString(": {")
				innerKeys := make([]string, 0, len(nsCache))
				for innerKey := range nsCache {
					innerKeys = append(innerKeys, innerKey)
				}
				sort.Strings(innerKeys)
				for ii, innerKey := range innerKeys {
					if ii > 0 {
						j.AddString(",\n    ")
					} else {
						j.AddString("\n    ")
					}
					j.AddBytes(helpers.QuoteForJSON(innerKey, asciiOnly))
					if value := nsCache[innerKey]; value != false {
						j.AddString(": ")
						j.AddBytes(helpers.QuoteForJSON(value.(string), asciiOnly))
					} else {
						j.AddString(": false")
					}
				}
				j.AddString("\n  }")
			} else {
				j.AddString(": {}")
			}
			continue
		}

		// Print the value
		if value := mangleCache[key]; value != false {
			j.AddString(": ")
			j.AddBytes(helpers.QuoteForJSON(value.(string), asciiOnly))
		} else {
			j.AddString(": false")
		}
	}

	if len(order) > 0 {
		j.AddString("\n")
	}
	j.AddString("}\n")
	return j.Done()
}
