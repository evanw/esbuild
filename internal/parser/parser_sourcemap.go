package parser

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/sourcemap"
)

// Specification: https://sourcemaps.info/spec.html
func ParseSourceMap(log logging.Log, source logging.Source) *sourcemap.SourceMap {
	expr, ok := ParseJSON(log, source, ParseJSONOptions{})
	if !ok {
		return nil
	}

	obj, ok := expr.Data.(*ast.EObject)
	if !ok {
		log.AddError(&source, expr.Loc, "Invalid source map")
		return nil
	}

	var sourcesContent []*string
	var sources []string
	var mappingsRaw []uint16
	var mappingsRange ast.Range
	hasVersion := false

	// Treat the paths in the source map as relative to the directory containing the source map
	var sourcesPrefix string
	if slash := strings.LastIndexAny(source.PrettyPath, "/\\"); slash != -1 {
		sourcesPrefix = source.PrettyPath[:slash+1]
	}

	for _, prop := range obj.Properties {
		keyRange := source.RangeOfString(prop.Key.Loc)

		switch lexer.UTF16ToString(prop.Key.Data.(*ast.EString).Value) {
		case "sections":
			log.AddRangeError(&source, keyRange, "Source maps with \"sections\" are not supported")
			return nil

		case "version":
			if value, ok := prop.Value.Data.(*ast.ENumber); !ok {
				log.AddRangeError(&source, keyRange, "The value for the \"version\" field must be a number")
				return nil
			} else {
				if value.Value != 3 {
					log.AddRangeError(&source, keyRange, "The source map \"version\" must be 3")
					return nil
				}
				hasVersion = true
			}

		case "mappings":
			if value, ok := prop.Value.Data.(*ast.EString); !ok {
				log.AddRangeError(&source, keyRange, "The value for the \"mappings\" field must be a string")
				return nil
			} else {
				mappingsRaw = value.Value
				mappingsRange = keyRange
			}

		case "sources":
			if value, ok := prop.Value.Data.(*ast.EArray); !ok {
				log.AddRangeError(&source, keyRange, "The value for the \"sources\" field must be an array")
				return nil
			} else {
				sources = nil
				for _, item := range value.Items {
					if element, ok := item.Data.(*ast.EString); !ok {
						log.AddError(&source, item.Loc, "Each element in the \"sources\" array must be a string")
						return nil
					} else {
						sources = append(sources, sourcesPrefix+lexer.UTF16ToString(element.Value))
					}
				}
			}

		case "sourcesContent":
			if value, ok := prop.Value.Data.(*ast.EArray); !ok {
				log.AddRangeError(&source, keyRange, "The value for the \"sourcesContent\" field must be an array")
				return nil
			} else {
				sourcesContent = nil
				for _, item := range value.Items {
					switch element := item.Data.(type) {
					case *ast.EString:
						str := lexer.UTF16ToString(element.Value)
						sourcesContent = append(sourcesContent, &str)
					case *ast.ENull:
						sourcesContent = append(sourcesContent, nil)
					default:
						log.AddError(&source, item.Loc, "Each element in the \"sourcesContent\" array must be a string or null")
						return nil
					}
				}
			}
		}
	}

	if !hasVersion {
		log.AddError(&source, expr.Loc, "This source map is missing the \"version\" field")
		return nil
	}

	// Silently fail if the source map is pointless (i.e. empty)
	if len(sources) == 0 || len(mappingsRaw) == 0 {
		return nil
	}

	var mappings []sourcemap.Mapping
	mappingsLen := len(mappingsRaw)
	sourcesLen := len(sources)
	generatedLine := 0
	generatedColumn := 0
	sourceIndex := 0
	originalLine := 0
	originalColumn := 0
	current := 0
	errorText := ""

	// Parse the mappings
	for current < mappingsLen {
		// Handle a line break
		if mappingsRaw[current] == ';' {
			generatedLine++
			generatedColumn = 0
			current++
			continue
		}

		// Read the generated column
		generatedColumnDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing generated column"
			break
		}
		current += i
		if generatedColumnDelta < 0 {
			// This would mess up binary search
			errorText = "Unexpected generated column decrease"
			break
		}
		generatedColumn += generatedColumnDelta
		if generatedColumn < 0 {
			errorText = fmt.Sprintf("Invalid generated column value: %d", generatedColumn)
			break
		}

		// According to the specification, it's valid for a mapping to have 1,
		// 4, or 5 variable-length fields. Having one field means there's no
		// original location information, which is pretty useless. Just ignore
		// those entries.
		if current == mappingsLen {
			break
		}
		c := mappingsRaw[current]
		if c == ',' || c == ';' {
			current++
			continue
		}

		// Read the original source
		sourceIndexDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing source index"
			break
		}
		current += i
		sourceIndex += sourceIndexDelta
		if sourceIndex < 0 || sourceIndex >= sourcesLen {
			errorText = fmt.Sprintf("Invalid source index value: %d", sourceIndex)
			break
		}

		// Read the original line
		originalLineDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing original line"
			break
		}
		current += i
		originalLine += originalLineDelta
		if originalLine < 0 {
			errorText = fmt.Sprintf("Invalid original line value: %d", originalLine)
			break
		}

		// Read the original column
		originalColumnDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing original column"
			break
		}
		current += i
		originalColumn += originalColumnDelta
		if originalColumn < 0 {
			errorText = fmt.Sprintf("Invalid original column value: %d", originalColumn)
			break
		}

		// Ignore the optional name index
		if _, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:]); ok {
			current += i
		}

		// Handle the next character
		if current < mappingsLen {
			if c := mappingsRaw[current]; c == ',' {
				current++
			} else if c != ';' {
				errorText = fmt.Sprintf("Invalid character after mapping: %q",
					lexer.UTF16ToString(mappingsRaw[current:current+1]))
				break
			}
		}

		mappings = append(mappings, sourcemap.Mapping{
			GeneratedLine:   int32(generatedLine),
			GeneratedColumn: int32(generatedColumn),
			SourceIndex:     int32(sourceIndex),
			OriginalLine:    int32(originalLine),
			OriginalColumn:  int32(originalColumn),
		})
	}

	if errorText != "" {
		log.AddRangeError(&source, mappingsRange,
			fmt.Sprintf("Bad \"mappings\" data in source map at character %d: %s", current, errorText))
		return nil
	}

	return &sourcemap.SourceMap{
		Sources:        sources,
		SourcesContent: sourcesContent,
		Mappings:       mappings,
	}
}
