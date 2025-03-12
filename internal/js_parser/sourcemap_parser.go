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

// New specification: https://tc39.es/ecma426/
// Old specification: https://sourcemaps.info/spec.html
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

	type sourceMapSection struct {
		lineOffset   int32
		columnOffset int32
		sourceMap    *js_ast.EObject
	}

	var sections []sourceMapSection
	hasSections := false

	for _, prop := range obj.Properties {
		if !helpers.UTF16EqualsString(prop.Key.Data.(*js_ast.EString).Value, "sections") {
			continue
		}

		if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
			for _, item := range value.Items {
				if element, ok := item.Data.(*js_ast.EObject); ok {
					var sectionLineOffset int32
					var sectionColumnOffset int32
					var sectionSourceMap *js_ast.EObject

					for _, sectionProp := range element.Properties {
						switch helpers.UTF16ToString(sectionProp.Key.Data.(*js_ast.EString).Value) {
						case "offset":
							if offsetValue, ok := sectionProp.ValueOrNil.Data.(*js_ast.EObject); ok {
								for _, offsetProp := range offsetValue.Properties {
									switch helpers.UTF16ToString(offsetProp.Key.Data.(*js_ast.EString).Value) {
									case "line":
										if lineValue, ok := offsetProp.ValueOrNil.Data.(*js_ast.ENumber); ok {
											sectionLineOffset = int32(lineValue.Value)
										}

									case "column":
										if columnValue, ok := offsetProp.ValueOrNil.Data.(*js_ast.ENumber); ok {
											sectionColumnOffset = int32(columnValue.Value)
										}
									}
								}
							} else {
								log.AddError(&tracker, logger.Range{Loc: sectionProp.ValueOrNil.Loc}, "Expected \"offset\" to be an object")
								return nil
							}

						case "map":
							if mapValue, ok := sectionProp.ValueOrNil.Data.(*js_ast.EObject); ok {
								sectionSourceMap = mapValue
							} else {
								log.AddError(&tracker, logger.Range{Loc: sectionProp.ValueOrNil.Loc}, "Expected \"map\" to be an object")
								return nil
							}
						}
					}

					if sectionSourceMap != nil {
						sections = append(sections, sourceMapSection{
							lineOffset:   sectionLineOffset,
							columnOffset: sectionColumnOffset,
							sourceMap:    sectionSourceMap,
						})
					}
				}
			}
		} else {
			log.AddError(&tracker, logger.Range{Loc: prop.ValueOrNil.Loc}, "Expected \"sections\" to be an array")
			return nil
		}

		hasSections = true
		break
	}

	if !hasSections {
		sections = append(sections, sourceMapSection{
			sourceMap: obj,
		})
	}

	var sources []string
	var sourcesContent []sourcemap.SourceContent
	var names []string
	var mappings mappingArray
	var generatedLine int32
	var generatedColumn int32
	needSort := false

	for _, section := range sections {
		var sourcesArray []js_ast.Expr
		var sourcesContentArray []js_ast.Expr
		var namesArray []js_ast.Expr
		var mappingsRaw []uint16
		var mappingsStart int32
		var sourceRoot string
		hasVersion := false

		for _, prop := range section.sourceMap.Properties {
			switch helpers.UTF16ToString(prop.Key.Data.(*js_ast.EString).Value) {
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
					sourcesArray = value.Items
				}

			case "sourcesContent":
				if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
					sourcesContentArray = value.Items
				}

			case "names":
				if value, ok := prop.ValueOrNil.Data.(*js_ast.EArray); ok {
					namesArray = value.Items
				}
			}
		}

		// Silently ignore the section if the version was missing or incorrect
		if !hasVersion {
			continue
		}

		mappingsLen := len(mappingsRaw)
		sourcesLen := len(sourcesArray)
		namesLen := len(namesArray)

		// Silently ignore the section if the source map is pointless (i.e. empty)
		if mappingsLen == 0 || sourcesLen == 0 {
			continue
		}

		if section.lineOffset < generatedLine || (section.lineOffset == generatedLine && section.columnOffset < generatedColumn) {
			needSort = true
		}

		lineOffset := section.lineOffset
		columnOffset := section.columnOffset
		sourceOffset := int32(len(sources))
		nameOffset := int32(len(names))

		generatedLine = lineOffset
		generatedColumn = columnOffset
		sourceIndex := sourceOffset
		var originalLine int32
		var originalColumn int32
		originalName := nameOffset

		current := 0
		errorText := ""
		errorLen := 0

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
			if (generatedLine == lineOffset && generatedColumn < columnOffset) || generatedColumn < 0 {
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
			if sourceIndex < sourceOffset || sourceIndex >= sourceOffset+int32(sourcesLen) {
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
				if originalName < nameOffset || originalName >= nameOffset+int32(namesLen) {
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

		for _, item := range sourcesArray {
			if element, ok := item.Data.(*js_ast.EString); ok {
				sourcePath := sourceURLPrefix + helpers.UTF16ToString(element.Value)
				sourceURL, err := url.Parse(sourcePath)

				// Report URL parse errors (such as "%XY" being an invalid escape)
				if err != nil {
					if urlErr, ok := err.(*url.Error); ok {
						err = urlErr.Err // Use the underlying error to reduce noise
					}
					log.AddID(logger.MsgID_SourceMap_InvalidSourceURL, logger.Warning, &tracker, source.RangeOfString(item.Loc),
						fmt.Sprintf("Invalid source URL: %s", err.Error()))
					sources = append(sources, "")
					continue
				}

				// Resolve this URL relative to the enclosing directory
				if baseURL != nil {
					sourceURL = baseURL.ResolveReference(sourceURL)
				}
				sources = append(sources, sourceURL.String())
			} else {
				sources = append(sources, "")
			}
		}

		if len(sourcesContentArray) > 0 {
			// It's possible that one of the source maps inside "sections" has
			// different lengths for the "sources" and "sourcesContent" arrays.
			// This is bad because we need to us a single index to get the name
			// of the source from "sources[i]" and the content of the source
			// from "sourcesContent[i]".
			//
			// So if a previous source map had a shorter "sourcesContent" array
			// than its "sources" array (or if the previous source map just had
			// no "sourcesContent" array), expand our aggregated array to the
			// right length by padding it out with empty entries.
			sourcesContent = append(sourcesContent, make([]sourcemap.SourceContent, int(sourceOffset)-len(sourcesContent))...)

			for i, item := range sourcesContentArray {
				// Make sure we don't ever record more "sourcesContent" entries
				// than there are "sources" entries, which is possible because
				// these are two separate arrays in the source map JSON. We need
				// to avoid this because that would mess up our shared indexing
				// of the "sources" and "sourcesContent" arrays. See the above
				// comment for more details.
				if i == sourcesLen {
					break
				}

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

		for _, item := range namesArray {
			if element, ok := item.Data.(*js_ast.EString); ok {
				names = append(names, helpers.UTF16ToString(element.Value))
			} else {
				names = append(names, "")
			}
		}
	}

	// Silently fail if the source map is pointless (i.e. empty)
	if len(sources) == 0 || len(mappings) == 0 {
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
