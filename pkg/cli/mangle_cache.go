package cli

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
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
)

func parseMangleCache(osArgs []string, fs fs.FS, absPath string) (map[string]interface{}, []string) {
	// Log problems with the mangle cache to stderr
	log := logger.NewStderrLog(logger.OutputOptionsForArgs(osArgs))
	defer log.Done()
	// log.AddMsg(msg)

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
			return make(map[string]interface{}), []string{}
		}

		// Otherwise, report the error
		log.Add(logger.Error, nil, logger.Range{},
			fmt.Sprintf("Failed to read from mangle cache file %q: %s", prettyPath, originalError.Error()))
		return nil, nil
	}

	// Use our JSON parser so we get pretty-printed error messages
	source := logger.Source{
		KeyPath:    logger.Path{Text: absPath, Namespace: "file"},
		PrettyPath: prettyPath,
		Contents:   string(bytes),
	}
	result, ok := js_parser.ParseJSON(log, source, js_parser.JSONOptions{})
	if !ok || log.HasErrors() {
		// Stop if there were any errors so we don't continue and then overwrite this file
		return nil, nil
	}
	tracker := logger.MakeLineColumnTracker(&source)

	// Validate the top-level object
	root, ok := result.Data.(*js_ast.EObject)
	if !ok {
		log.Add(logger.Error, &tracker, logger.Range{Loc: result.Loc},
			"Expected a top-level object in mangle cache file")
		return nil, nil
	}

	mangleCache := make(map[string]interface{}, len(root.Properties))
	order := make([]string, 0, len(root.Properties))

	for _, property := range root.Properties {
		key := helpers.UTF16ToString(property.Key.Data.(*js_ast.EString).Value)
		order = append(order, key)

		switch v := property.ValueOrNil.Data.(type) {
		case *js_ast.EBoolean:
			if v.Value {
				log.Add(logger.Error, &tracker, js_lexer.RangeOfIdentifier(source, property.ValueOrNil.Loc),
					fmt.Sprintf("Expected %q in mangle cache file to map to either a string or false", key))
			} else {
				mangleCache[key] = false
			}

		case *js_ast.EString:
			mangleCache[key] = helpers.UTF16ToString(v.Value)

		default:
			log.Add(logger.Error, &tracker, logger.Range{Loc: property.ValueOrNil.Loc},
				fmt.Sprintf("Expected %q in mangle cache file to map to either a string or false", key))
		}
	}

	if log.HasErrors() {
		return nil, nil
	}
	return mangleCache, order
}

func printMangleCache(mangleCache map[string]interface{}, originalOrder []string, asciiOnly bool) []byte {
	j := helpers.Joiner{}
	j.AddString("{")

	// Determine the order to print the keys in
	order := originalOrder
	if len(mangleCache) > len(order) {
		order = make([]string, 0, len(mangleCache))
		if sort.StringsAreSorted(originalOrder) {
			// If they came sorted, keep them sorted
			for key := range mangleCache {
				order = append(order, key)
			}
			sort.Strings(order)
		} else {
			// Otherwise add all new keys to the end, and only sort the new keys
			originalKeys := make(map[string]bool, len(originalOrder))
			for _, key := range originalOrder {
				originalKeys[key] = true
			}
			order = append(order, originalOrder...)
			for key := range mangleCache {
				if !originalKeys[key] {
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
		j.AddBytes(js_printer.QuoteForJSON(key, asciiOnly))

		// Print the value
		if value := mangleCache[key]; value != false {
			j.AddString(": ")
			j.AddBytes(js_printer.QuoteForJSON(value.(string), asciiOnly))
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
