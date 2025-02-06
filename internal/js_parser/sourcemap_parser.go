package js_parser

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/sourcemap"
)

// Specification: https://sourcemaps.info/spec.html
func ParseSourceMap(log logger.Log, source logger.Source) *sourcemap.SourceMap {
	expr, ok := ParseJSON(log, source, JSONOptions{ErrorSuffix: " in source map"})
	if !ok {
		return nil
	}

	obj, ok := expr.Data.(*js_ast.EObject)
	tracker := logger.MakeLineColumnTracker(&source)
	if !ok {
		log.AddError(&tracker, logger.Range{Loc: expr.Loc}, "Invalid source map")
		return nil
	}

	var sources []string
	var sourcesArray []js_ast.Expr
	var sourcesContent []sourcemap.SourceContent
	var names []string
	var mappingsRaw []uint16
	var mappingsStart int32
	var sourceRoot string
	hasVersion := false

	for _, prop := range obj.Properties {
		keyRange := source.RangeOfString(prop.Key.Loc)

		switch helpers.UTF16ToString(prop.Key.Data.(*js_ast.EString).Value) {
		case "sections":
			log.AddID(logger.MsgID_SourceMap_SectionsInSourceMap, logger.Warning, &tracker, keyRange, "Source maps with \"sections\" are not supported")
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

		case "sourceRoot":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EString); ok {
				sourceRoot = helpers.UTF16ToString(value.Value)
			}

		case "sources":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
				sources = make([]string, len(value.Items))
				sourcesArray = value.Items
				for i, item := range value.Items {
					if element, ok := item.Data.(*js_ast.EString); ok {
						sources[i] = helpers.UTF16ToString(element.Value)
					}
				}
			}

		case "sourcesContent":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
				sourcesContent = []sourcemap.SourceContent{}
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

		case "names":
			if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
				names = []string{}
				for _, item := range value.Items {
					if element, ok := item.Data.(*js_ast.EString); ok {
						names = append(names, helpers.UTF16ToString(element.Value))
					} else {
						names = append(names, "")
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
	namesLen := len(names)
	var generatedLine int32
	var generatedColumn int32
	var sourceIndex int32
	var originalLine int32
	var originalColumn int32
	var originalName int32
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
		if sourceIndex < 0 || sourceIndex >= int32(sourcesLen) {
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

		// Read the original name
		var optionalName ast.Index32
		if originalNameDelta, i, ok := sourcemap.DecodeVLQUTF16(mappingsRaw[current:]); ok {
			originalName += originalNameDelta
			if originalName < 0 || originalName >= int32(namesLen) {
				errorText = fmt.Sprintf("Invalid name index value: %d", originalName)
				errorLen = i
				break
			}
			optionalName = ast.MakeIndex32(uint32(originalName))
			current += i
		}

		// Handle the next character
		if current < mappingsLen {
			if c := mappingsRaw[current]; c == ',' {
				current++
			} else if c != ';' {
				errorText = fmt.Sprintf("Invalid character after mapping: %q",
					helpers.UTF16ToString(mappingsRaw[current:current+1]))
				errorLen = 1
				break
			}
		}

		mappings = append(mappings, sourcemap.Mapping{
			GeneratedLine:   generatedLine,
			GeneratedColumn: generatedColumn,
			SourceIndex:     sourceIndex,
			OriginalLine:    originalLine,
			OriginalColumn:  originalColumn,
			OriginalName:    optionalName,
		})
	}

	if errorText != "" {
		r := logger.Range{Loc: logger.Loc{Start: mappingsStart + int32(current)}, Len: int32(errorLen)}
		log.AddID(logger.MsgID_SourceMap_InvalidSourceMappings, logger.Warning, &tracker, r,
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

	// Try resolving relative source URLs into absolute source URLs.
	// See https://tc39.es/ecma426/#resolving-sources for details.
	var sourceURLPrefix string
	var baseURL *url.URL
	if sourceRoot != "" {
		if index := strings.LastIndexByte(sourceRoot, '/'); index != -1 {
			sourceURLPrefix = sourceRoot[:index+1]
		} else {
			sourceURLPrefix = sourceRoot + "/"
		}
	}
	if source.KeyPath.Namespace == "file" {
		baseURL = helpers.FileURLFromFilePath(source.KeyPath.Text)
	}
	for i, sourcePath := range sources {
		if sourcePath == "" {
			continue // Skip null entries
		}
		sourcePath = sourceURLPrefix + sourcePath
		sourceURL, err := url.Parse(sourcePath)

		// Report URL parse errors (such as "%XY" being an invalid escape)
		if err != nil {
			if urlErr, ok := err.(*url.Error); ok {
				err = urlErr.Err // Use the underlying error to reduce noise
			}
			log.AddID(logger.MsgID_SourceMap_InvalidSourceURL, logger.Warning, &tracker, source.RangeOfString(sourcesArray[i].Loc),
				fmt.Sprintf("Invalid source URL: %s", err.Error()))
			continue
		}

		// Resolve this URL relative to the enclosing directory
		if baseURL != nil {
			sourceURL = baseURL.ResolveReference(sourceURL)
		}
		sources[i] = sourceURL.String()
	}

	return &sourcemap.SourceMap{
		Sources:        sources,
		SourcesContent: sourcesContent,
		Mappings:       mappings,
		Names:          names,
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
