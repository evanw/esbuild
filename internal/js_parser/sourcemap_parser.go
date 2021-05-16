package js_parser

import (
	"fmt"
	"sort"

	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/sourcemap"
)

// Specification: https://sourcemaps.info/spec.html
func ParseSourceMap(log logger.Log, source logger.Source) *sourcemap.SourceMap {
	expr, ok := ParseJSON(log, source, JSONOptions{})
	if !ok {
		return nil
	}

	obj, ok := expr.Data.(*js_ast.EObject)
	tracker := logger.MakeLineColumnTracker(&source)
	if !ok {
		log.AddError(&tracker, expr.Loc, "Invalid source map")
		return nil
	}

	var sources []string
	var sourcesContent []sourcemap.SourceContent
	var mappingsRaw []uint16
	var mappingsStart int32
	hasVersion := false

	for _, prop := range obj.Properties {
		keyRange := source.RangeOfString(prop.Key.Loc)

		switch js_lexer.UTF16ToString(prop.Key.Data.(*js_ast.EString).Value) {
		case "sections":
			log.AddRangeWarning(&tracker, keyRange, "Source maps with \"sections\" are not supported")
			return nil

		case "version":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.ENumber); ok && value.Value == 3 {
				hasVersion = true
			}

		case "mappings":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EString); ok {
				mappingsRaw = value.Value
				mappingsStart = prop.ValueOrNil.Loc.Start + 1
			}

		case "sources":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
				sources = nil
				for _, item := range value.Items {
					if element, ok := item.Data.(*js_ast.EString); ok {
						sources = append(sources, js_lexer.UTF16ToString(element.Value))
					} else {
						sources = append(sources, "")
					}
				}
			}

		case "sourcesContent":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
				sourcesContent = nil
				for _, item := range value.Items {
					if element, ok := item.Data.(*js_ast.EString); ok {
						sourcesContent = append(sourcesContent, sourcemap.SourceContent{
							Value:  element.Value,
							Quoted: source.TextForRange(source.RangeOfString(item.Loc)),
						})
					} else {
						sourcesContent = append(sourcesContent, sourcemap.SourceContent{})
					}
				}
			}
		}
	}

	// Silently fail if the version was missing or incorrect
	if !hasVersion {
		return nil
	}

	// Silently fail if the source map is pointless (i.e. empty)
	if len(sources) == 0 || len(mappingsRaw) == 0 {
		return nil
	}

	var mappings mappingArray
	mappingsLen := len(mappingsRaw)
	sourcesLen := len(sources)
	generatedLine := 0
	generatedColumn := 0
	sourceIndex := 0
	originalLine := 0
	originalColumn := 0
	current := 0
	errorText := ""
	errorLen := 0
	needSort := false

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
			errorLen = i
			break
		}
		if generatedColumnDelta < 0 {
			// This would mess up binary search
			needSort = true
		}
		generatedColumn += generatedColumnDelta
		if generatedColumn < 0 {
			errorText = fmt.Sprintf("Invalid generated column value: %d", generatedColumn)
			errorLen = i
			break
		}
		current += i

		// According to the specification, it's valid for a mapping to have 1,
		// 4, or 5 variable-length fields. Having one field means there's no
		// original location information, which is pretty useless. Just ignore
		// those entries.
		if current == mappingsLen {
			break
		}
		switch mappingsRaw[current] {
		case ',':
			current++
			continue
		case ';':
			continue
		}

		// Read the original source
		sourceIndexDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing source index"
			errorLen = i
			break
		}
		sourceIndex += sourceIndexDelta
		if sourceIndex < 0 || sourceIndex >= sourcesLen {
			errorText = fmt.Sprintf("Invalid source index value: %d", sourceIndex)
			errorLen = i
			break
		}
		current += i

		// Read the original line
		originalLineDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing original line"
			errorLen = i
			break
		}
		originalLine += originalLineDelta
		if originalLine < 0 {
			errorText = fmt.Sprintf("Invalid original line value: %d", originalLine)
			errorLen = i
			break
		}
		current += i

		// Read the original column
		originalColumnDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:])
		if !ok {
			errorText = "Missing original column"
			errorLen = i
			break
		}
		originalColumn += originalColumnDelta
		if originalColumn < 0 {
			errorText = fmt.Sprintf("Invalid original column value: %d", originalColumn)
			errorLen = i
			break
		}
		current += i

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
					js_lexer.UTF16ToString(mappingsRaw[current:current+1]))
				errorLen = 1
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
		r := logger.Range{Loc: logger.Loc{Start: mappingsStart + int32(current)}, Len: int32(errorLen)}
		log.AddRangeWarning(&tracker, r,
			fmt.Sprintf("Bad \"mappings\" data in source map at character %d: %s", current, errorText))
		return nil
	}

	if needSort {
		// If we get here, some mappings are out of order. Lines can't be out of
		// order by construction but columns can. This is a pretty rare situation
		// because almost all source map generators always write out mappings in
		// order as they write the output instead of scrambling the order.
		sort.Stable(mappings)
	}

	return &sourcemap.SourceMap{
		Sources:        sources,
		SourcesContent: sourcesContent,
		Mappings:       mappings,
	}
}

// This type is just so we can use Go's native sort function
type mappingArray []sourcemap.Mapping

func (a mappingArray) Len() int          { return len(a) }
func (a mappingArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a mappingArray) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]
	return ai.GeneratedLine < aj.GeneratedLine || (ai.GeneratedLine == aj.GeneratedLine && ai.GeneratedColumn <= aj.GeneratedColumn)
}
