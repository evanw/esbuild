package resolver

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

type TSConfigJSON struct {
	AbsPath string

	// The absolute path of "compilerOptions.baseUrl"
	BaseURL *string

	// This is used if "Paths" is non-nil. It's equal to "BaseURL" except if
	// "BaseURL" is missing, in which case it is as if "BaseURL" was ".". This
	// is to implement the "paths without baseUrl" feature from TypeScript 4.1.
	// More info: https://github.com/microsoft/TypeScript/issues/31869
	BaseURLForPaths string

	// The verbatim values of "compilerOptions.paths". The keys are patterns to
	// match and the values are arrays of fallback paths to search. Each key and
	// each fallback path can optionally have a single "*" wildcard character.
	// If both the key and the value have a wildcard, the substring matched by
	// the wildcard is substituted into the fallback path. The keys represent
	// module-style path names and the fallback paths are relative to the
	// "baseUrl" value in the "tsconfig.json" file.
	Paths map[string][]string

	JSXFactory                     []string
	JSXFragmentFactory             []string
	UseDefineForClassFields        bool
	PreserveImportsNotUsedAsValues bool
}

func ParseTSConfigJSON(
	log logger.Log,
	source logger.Source,
	jsonCache *cache.JSONCache,
	extends func(string, logger.Range) *TSConfigJSON,
) *TSConfigJSON {
	// Unfortunately "tsconfig.json" isn't actually JSON. It's some other
	// format that appears to be defined by the implementation details of the
	// TypeScript compiler.
	//
	// Attempt to parse it anyway by modifying the JSON parser, but just for
	// these particular files. This is likely not a completely accurate
	// emulation of what the TypeScript compiler does (e.g. string escape
	// behavior may also be different).
	json, ok := jsonCache.Parse(log, source, js_parser.JSONOptions{
		AllowComments:       true, // https://github.com/microsoft/TypeScript/issues/4987
		AllowTrailingCommas: true,
	})
	if !ok {
		return nil
	}

	var result TSConfigJSON
	result.AbsPath = source.KeyPath.Text

	// Parse "extends"
	if extends != nil {
		if valueJSON, _, ok := getProperty(json, "extends"); ok {
			if value, ok := getString(valueJSON); ok {
				if base := extends(value, source.RangeOfString(valueJSON.Loc)); base != nil {
					result = *base
				}
			}
		}
	}

	// Parse "compilerOptions"
	if compilerOptionsJSON, _, ok := getProperty(json, "compilerOptions"); ok {
		// Parse "baseUrl"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "baseUrl"); ok {
			if value, ok := getString(valueJSON); ok {
				result.BaseURL = &value
			}
		}

		// Parse "jsxFactory"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsxFactory"); ok {
			if value, ok := getString(valueJSON); ok {
				result.JSXFactory = parseMemberExpressionForJSX(log, source, valueJSON.Loc, value)
			}
		}

		// Parse "jsxFragmentFactory"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsxFragmentFactory"); ok {
			if value, ok := getString(valueJSON); ok {
				result.JSXFragmentFactory = parseMemberExpressionForJSX(log, source, valueJSON.Loc, value)
			}
		}

		// Parse "useDefineForClassFields"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "useDefineForClassFields"); ok {
			if value, ok := getBool(valueJSON); ok {
				result.UseDefineForClassFields = value
			}
		}

		// Parse "importsNotUsedAsValues"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "importsNotUsedAsValues"); ok {
			if value, ok := getString(valueJSON); ok {
				switch value {
				case "preserve", "error":
					result.PreserveImportsNotUsedAsValues = true
				case "remove":
				default:
					log.AddRangeWarning(&source, source.RangeOfString(valueJSON.Loc),
						fmt.Sprintf("Invalid value %q for \"importsNotUsedAsValues\"", value))
				}
			}
		}

		// Parse "paths"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "paths"); ok {
			if paths, ok := valueJSON.Data.(*js_ast.EObject); ok {
				hasBaseURL := result.BaseURL != nil
				if hasBaseURL {
					result.BaseURLForPaths = *result.BaseURL
				} else {
					result.BaseURLForPaths = "."
				}
				result.Paths = make(map[string][]string)
				for _, prop := range paths.Properties {
					if key, ok := getString(prop.Key); ok {
						if !isValidTSConfigPathPattern(key, log, source, prop.Key.Loc) {
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
									if isValidTSConfigPathPattern(str, log, source, item.Loc) &&
										(hasBaseURL || isValidTSConfigPathNoBaseURLPattern(str, log, source, item.Loc)) {
										result.Paths[key] = append(result.Paths[key], str)
									}
								}
							}
						} else {
							log.AddRangeWarning(&source, source.RangeOfString(prop.Value.Loc), fmt.Sprintf(
								"Substitutions for pattern %q should be an array", key))
						}
					}
				}
			}
		}
	}

	return &result
}

func parseMemberExpressionForJSX(log logger.Log, source logger.Source, loc logger.Loc, text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !js_lexer.IsIdentifier(part) {
			warnRange := source.RangeOfString(loc)
			log.AddRangeWarning(&source, warnRange, fmt.Sprintf("Invalid JSX member expression: %q", text))
			return nil
		}
	}
	return parts
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

func isSlash(c byte) bool {
	return c == '/' || c == '\\'
}

func isValidTSConfigPathNoBaseURLPattern(text string, log logger.Log, source logger.Source, loc logger.Loc) bool {
	var c0 byte
	var c1 byte
	var c2 byte
	n := len(text)

	if n > 0 {
		c0 = text[0]
		if n > 1 {
			c1 = text[1]
			if n > 2 {
				c2 = text[2]
			}
		}
	}

	// Relative "." or ".."
	if c0 == '.' && (n == 1 || (n == 2 && c1 == '.')) {
		return true
	}

	// Relative "./" or "../" or ".\\" or "..\\"
	if c0 == '.' && (isSlash(c1) || (c1 == '.' && isSlash(c2))) {
		return true
	}

	// Absolute POSIX "/" or UNC "\\"
	if isSlash(c0) {
		return true
	}

	// Absolute DOS "c:/" or "c:\\"
	if ((c0 >= 'a' && c0 <= 'z') || (c0 >= 'A' && c0 <= 'Z')) && c1 == ':' && isSlash(c2) {
		return true
	}

	r := source.RangeOfString(loc)
	log.AddRangeWarning(&source, r, fmt.Sprintf(
		"Non-relative path %q is not allowed when \"baseUrl\" is not set (did you forget a leading \"./\"?)", text))
	return false
}
