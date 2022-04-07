package resolver

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
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
	Paths *TSConfigPaths

	TSTarget                       *config.TSTarget
	JSXFactory                     []string
	JSXFragmentFactory             []string
	UseDefineForClassFields        config.MaybeBool
	PreserveImportsNotUsedAsValues bool
	PreserveValueImports           bool
}

type TSConfigPath struct {
	Text string
	Loc  logger.Loc
}

type TSConfigPaths struct {
	Map map[string][]TSConfigPath

	// This may be different from the original "tsconfig.json" source if the
	// "paths" value is from another file via an "extends" clause.
	Source logger.Source
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
	tracker := logger.MakeLineColumnTracker(&source)

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
				result.JSXFactory = parseMemberExpressionForJSX(log, &source, &tracker, valueJSON.Loc, value)
			}
		}

		// Parse "jsxFragmentFactory"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsxFragmentFactory"); ok {
			if value, ok := getString(valueJSON); ok {
				result.JSXFragmentFactory = parseMemberExpressionForJSX(log, &source, &tracker, valueJSON.Loc, value)
			}
		}

		// Parse "useDefineForClassFields"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "useDefineForClassFields"); ok {
			if value, ok := getBool(valueJSON); ok {
				if value {
					result.UseDefineForClassFields = config.True
				} else {
					result.UseDefineForClassFields = config.False
				}
			}
		}

		// Parse "target"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "target"); ok {
			if value, ok := getString(valueJSON); ok {
				constraints := make(map[compat.Engine][]int)
				r := source.RangeOfString(valueJSON.Loc)
				ok := true

				// See https://www.typescriptlang.org/tsconfig#target
				targetIsAtLeastES2022 := false
				switch strings.ToLower(value) {
				case "es5":
					constraints[compat.ES] = []int{5}
				case "es6", "es2015":
					constraints[compat.ES] = []int{2015}
				case "es2016":
					constraints[compat.ES] = []int{2016}
				case "es2017":
					constraints[compat.ES] = []int{2017}
				case "es2018":
					constraints[compat.ES] = []int{2018}
				case "es2019":
					constraints[compat.ES] = []int{2019}
				case "es2020":
					constraints[compat.ES] = []int{2020}
				case "es2021":
					constraints[compat.ES] = []int{2021}
				case "es2022":
					constraints[compat.ES] = []int{2022}
					targetIsAtLeastES2022 = true
				case "esnext":
					targetIsAtLeastES2022 = true
				default:
					ok = false
					if !helpers.IsInsideNodeModules(source.KeyPath.Text) {
						log.Add(logger.Warning, &tracker, r,
							fmt.Sprintf("Unrecognized target environment %q", value))
					}
				}

				// These feature restrictions are merged with esbuild's own restrictions
				if ok {
					result.TSTarget = &config.TSTarget{
						Source:                source,
						Range:                 r,
						Target:                value,
						UnsupportedJSFeatures: compat.UnsupportedJSFeatures(constraints),
						TargetIsAtLeastES2022: targetIsAtLeastES2022,
					}
				}
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
					log.Add(logger.Warning, &tracker, source.RangeOfString(valueJSON.Loc),
						fmt.Sprintf("Invalid value %q for \"importsNotUsedAsValues\"", value))
				}
			}
		}

		// Parse "preserveValueImports"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "preserveValueImports"); ok {
			if value, ok := getBool(valueJSON); ok {
				result.PreserveValueImports = value
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
				result.Paths = &TSConfigPaths{Source: source, Map: make(map[string][]TSConfigPath)}
				for _, prop := range paths.Properties {
					if key, ok := getString(prop.Key); ok {
						if !isValidTSConfigPathPattern(key, log, &source, &tracker, prop.Key.Loc) {
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
						if array, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
							for _, item := range array.Items {
								if str, ok := getString(item); ok {
									if isValidTSConfigPathPattern(str, log, &source, &tracker, item.Loc) {
										result.Paths.Map[key] = append(result.Paths.Map[key], TSConfigPath{Text: str, Loc: item.Loc})
									}
								}
							}
						} else {
							log.Add(logger.Warning, &tracker, source.RangeOfString(prop.ValueOrNil.Loc), fmt.Sprintf(
								"Substitutions for pattern %q should be an array", key))
						}
					}
				}
			}
		}
	}

	return &result
}

func parseMemberExpressionForJSX(log logger.Log, source *logger.Source, tracker *logger.LineColumnTracker, loc logger.Loc, text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !js_lexer.IsIdentifier(part) {
			warnRange := source.RangeOfString(loc)
			log.Add(logger.Warning, tracker, warnRange, fmt.Sprintf("Invalid JSX member expression: %q", text))
			return nil
		}
	}
	return parts
}

func isValidTSConfigPathPattern(text string, log logger.Log, source *logger.Source, tracker *logger.LineColumnTracker, loc logger.Loc) bool {
	foundAsterisk := false
	for i := 0; i < len(text); i++ {
		if text[i] == '*' {
			if foundAsterisk {
				r := source.RangeOfString(loc)
				log.Add(logger.Warning, tracker, r, fmt.Sprintf(
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

func isValidTSConfigPathNoBaseURLPattern(text string, log logger.Log, source *logger.Source, tracker **logger.LineColumnTracker, loc logger.Loc) bool {
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
	if *tracker == nil {
		t := logger.MakeLineColumnTracker(source)
		*tracker = &t
	}
	log.Add(logger.Warning, *tracker, r, fmt.Sprintf(
		"Non-relative path %q is not allowed when \"baseUrl\" is not set (did you forget a leading \"./\"?)", text))
	return false
}
