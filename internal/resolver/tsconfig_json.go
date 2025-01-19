package resolver

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
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

	tsTargetKey    tsTargetKey
	TSStrict       *config.TSAlwaysStrict
	TSAlwaysStrict *config.TSAlwaysStrict
	JSXSettings    config.TSConfigJSX
	Settings       config.TSConfig
}

func (derived *TSConfigJSON) applyExtendedConfig(base TSConfigJSON) {
	if base.tsTargetKey.Range.Len > 0 {
		derived.tsTargetKey = base.tsTargetKey
	}
	if base.TSStrict != nil {
		derived.TSStrict = base.TSStrict
	}
	if base.TSAlwaysStrict != nil {
		derived.TSAlwaysStrict = base.TSAlwaysStrict
	}
	if base.BaseURL != nil {
		derived.BaseURL = base.BaseURL
	}
	if base.Paths != nil {
		derived.Paths = base.Paths
		derived.BaseURLForPaths = base.BaseURLForPaths
	}
	derived.JSXSettings.ApplyExtendedConfig(base.JSXSettings)
	derived.Settings.ApplyExtendedConfig(base.Settings)
}

func (config *TSConfigJSON) TSAlwaysStrictOrStrict() *config.TSAlwaysStrict {
	if config.TSAlwaysStrict != nil {
		return config.TSAlwaysStrict
	}

	// If "alwaysStrict" is absent, it defaults to "strict" instead
	return config.TSStrict
}

// This information is only used for error messages
type tsTargetKey struct {
	LowerValue string
	Source     logger.Source
	Range      logger.Range
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
	fs fs.FS,
	fileDir string,
	configDir string,
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
	json, ok := jsonCache.Parse(log, source, js_parser.JSONOptions{Flavor: js_lexer.TSConfigJSON})
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
					result.applyExtendedConfig(*base)
				}
			} else if array, ok := valueJSON.Data.(*js_ast.EArray); ok {
				for _, item := range array.Items {
					if str, ok := getString(item); ok {
						if base := extends(str, source.RangeOfString(item.Loc)); base != nil {
							result.applyExtendedConfig(*base)
						}
					}
				}
			}
		}
	}

	// Parse "compilerOptions"
	if compilerOptionsJSON, _, ok := getProperty(json, "compilerOptions"); ok {
		// Parse "baseUrl"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "baseUrl"); ok {
			if value, ok := getString(valueJSON); ok {
				value = getSubstitutedPathWithConfigDirTemplate(fs, value, configDir)
				if !fs.IsAbs(value) {
					value = fs.Join(fileDir, value)
				}
				result.BaseURL = &value
			}
		}

		// Parse "jsx"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsx"); ok {
			if value, ok := getString(valueJSON); ok {
				switch strings.ToLower(value) {
				case "preserve":
					result.JSXSettings.JSX = config.TSJSXPreserve
				case "react-native":
					result.JSXSettings.JSX = config.TSJSXReactNative
				case "react":
					result.JSXSettings.JSX = config.TSJSXReact
				case "react-jsx":
					result.JSXSettings.JSX = config.TSJSXReactJSX
				case "react-jsxdev":
					result.JSXSettings.JSX = config.TSJSXReactJSXDev
				}
			}
		}

		// Parse "jsxFactory"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsxFactory"); ok {
			if value, ok := getString(valueJSON); ok {
				result.JSXSettings.JSXFactory = parseMemberExpressionForJSX(log, &source, &tracker, valueJSON.Loc, value)
			}
		}

		// Parse "jsxFragmentFactory"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsxFragmentFactory"); ok {
			if value, ok := getString(valueJSON); ok {
				result.JSXSettings.JSXFragmentFactory = parseMemberExpressionForJSX(log, &source, &tracker, valueJSON.Loc, value)
			}
		}

		// Parse "jsxImportSource"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "jsxImportSource"); ok {
			if value, ok := getString(valueJSON); ok {
				result.JSXSettings.JSXImportSource = &value
			}
		}

		// Parse "experimentalDecorators"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "experimentalDecorators"); ok {
			if value, ok := getBool(valueJSON); ok {
				if value {
					result.Settings.ExperimentalDecorators = config.True
				} else {
					result.Settings.ExperimentalDecorators = config.False
				}
			}
		}

		// Parse "useDefineForClassFields"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "useDefineForClassFields"); ok {
			if value, ok := getBool(valueJSON); ok {
				if value {
					result.Settings.UseDefineForClassFields = config.True
				} else {
					result.Settings.UseDefineForClassFields = config.False
				}
			}
		}

		// Parse "target"
		if valueJSON, keyLoc, ok := getProperty(compilerOptionsJSON, "target"); ok {
			if value, ok := getString(valueJSON); ok {
				lowerValue := strings.ToLower(value)
				ok := true

				// See https://www.typescriptlang.org/tsconfig#target
				switch lowerValue {
				case "es3", "es5", "es6", "es2015", "es2016", "es2017", "es2018", "es2019", "es2020", "es2021":
					result.Settings.Target = config.TSTargetBelowES2022
				case "es2022", "es2023", "es2024", "esnext":
					result.Settings.Target = config.TSTargetAtOrAboveES2022
				default:
					ok = false
					if !helpers.IsInsideNodeModules(source.KeyPath.Text) {
						log.AddID(logger.MsgID_TSConfigJSON_InvalidTarget, logger.Warning, &tracker, source.RangeOfString(valueJSON.Loc),
							fmt.Sprintf("Unrecognized target environment %q", value))
					}
				}

				if ok {
					result.tsTargetKey = tsTargetKey{
						Source:     source,
						Range:      source.RangeOfString(keyLoc),
						LowerValue: lowerValue,
					}
				}
			}
		}

		// Parse "strict"
		if valueJSON, keyLoc, ok := getProperty(compilerOptionsJSON, "strict"); ok {
			if value, ok := getBool(valueJSON); ok {
				valueRange := js_lexer.RangeOfIdentifier(source, valueJSON.Loc)
				result.TSStrict = &config.TSAlwaysStrict{
					Name:   "strict",
					Value:  value,
					Source: source,
					Range:  logger.Range{Loc: keyLoc, Len: valueRange.End() - keyLoc.Start},
				}
			}
		}

		// Parse "alwaysStrict"
		if valueJSON, keyLoc, ok := getProperty(compilerOptionsJSON, "alwaysStrict"); ok {
			if value, ok := getBool(valueJSON); ok {
				valueRange := js_lexer.RangeOfIdentifier(source, valueJSON.Loc)
				result.TSAlwaysStrict = &config.TSAlwaysStrict{
					Name:   "alwaysStrict",
					Value:  value,
					Source: source,
					Range:  logger.Range{Loc: keyLoc, Len: valueRange.End() - keyLoc.Start},
				}
			}
		}

		// Parse "importsNotUsedAsValues"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "importsNotUsedAsValues"); ok {
			if value, ok := getString(valueJSON); ok {
				switch value {
				case "remove":
					result.Settings.ImportsNotUsedAsValues = config.TSImportsNotUsedAsValues_Remove
				case "preserve":
					result.Settings.ImportsNotUsedAsValues = config.TSImportsNotUsedAsValues_Preserve
				case "error":
					result.Settings.ImportsNotUsedAsValues = config.TSImportsNotUsedAsValues_Error
				default:
					log.AddID(logger.MsgID_TSConfigJSON_InvalidImportsNotUsedAsValues, logger.Warning, &tracker, source.RangeOfString(valueJSON.Loc),
						fmt.Sprintf("Invalid value %q for \"importsNotUsedAsValues\"", value))
				}
			}
		}

		// Parse "preserveValueImports"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "preserveValueImports"); ok {
			if value, ok := getBool(valueJSON); ok {
				if value {
					result.Settings.PreserveValueImports = config.True
				} else {
					result.Settings.PreserveValueImports = config.False
				}
			}
		}

		// Parse "verbatimModuleSyntax"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "verbatimModuleSyntax"); ok {
			if value, ok := getBool(valueJSON); ok {
				if value {
					result.Settings.VerbatimModuleSyntax = config.True
				} else {
					result.Settings.VerbatimModuleSyntax = config.False
				}
			}
		}

		// Parse "paths"
		if valueJSON, _, ok := getProperty(compilerOptionsJSON, "paths"); ok {
			if paths, ok := valueJSON.Data.(*js_ast.EObject); ok {
				result.BaseURLForPaths = fileDir
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
										str = getSubstitutedPathWithConfigDirTemplate(fs, str, configDir)
										result.Paths.Map[key] = append(result.Paths.Map[key], TSConfigPath{Text: str, Loc: item.Loc})
									}
								}
							}
						} else {
							log.AddID(logger.MsgID_TSConfigJSON_InvalidPaths, logger.Warning, &tracker, source.RangeOfString(prop.ValueOrNil.Loc), fmt.Sprintf(
								"Substitutions for pattern %q should be an array", key))
						}
					}
				}
			}
		}
	}

	// Warn about compiler options not wrapped in "compilerOptions".
	// For example: https://github.com/evanw/esbuild/issues/3301
	if obj, ok := json.Data.(*js_ast.EObject); ok {
	loop:
		for _, prop := range obj.Properties {
			if key, ok := prop.Key.Data.(*js_ast.EString); ok && key.Value != nil {
				key := helpers.UTF16ToString(key.Value)
				switch key {
				case "alwaysStrict",
					"baseUrl",
					"experimentalDecorators",
					"importsNotUsedAsValues",
					"jsx",
					"jsxFactory",
					"jsxFragmentFactory",
					"jsxImportSource",
					"paths",
					"preserveValueImports",
					"strict",
					"target",
					"useDefineForClassFields",
					"verbatimModuleSyntax":
					log.AddIDWithNotes(logger.MsgID_TSConfigJSON_InvalidTopLevelOption, logger.Warning, &tracker, source.RangeOfString(prop.Key.Loc),
						fmt.Sprintf("Expected the %q option to be nested inside a \"compilerOptions\" object", key),
						[]logger.MsgData{})
					break loop
				}
			}
		}
	}

	return &result
}

// See: https://github.com/microsoft/TypeScript/pull/58042
func getSubstitutedPathWithConfigDirTemplate(fs fs.FS, value string, basePath string) string {
	if strings.HasPrefix(value, "${configDir}") {
		return fs.Join(basePath, "./"+value[12:])
	}
	return value
}

func parseMemberExpressionForJSX(log logger.Log, source *logger.Source, tracker *logger.LineColumnTracker, loc logger.Loc, text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !js_ast.IsIdentifier(part) {
			warnRange := source.RangeOfString(loc)
			log.AddID(logger.MsgID_TSConfigJSON_InvalidJSX, logger.Warning, tracker, warnRange, fmt.Sprintf("Invalid JSX member expression: %q", text))
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
				log.AddID(logger.MsgID_TSConfigJSON_InvalidPaths, logger.Warning, tracker, r, fmt.Sprintf(
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
	log.AddID(logger.MsgID_TSConfigJSON_InvalidPaths, logger.Warning, *tracker, r, fmt.Sprintf(
		"Non-relative path %q is not allowed when \"baseUrl\" is not set (did you forget a leading \"./\"?)", text))
	return false
}
