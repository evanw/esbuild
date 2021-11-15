package compat

type CSSFeature uint32

const (
	HexRGBA CSSFeature = 1 << iota

	RebeccaPurple

	// This feature includes all of the following:
	// - Allow floats in rgb() and rgba()
	// - hsl() can accept alpha values
	// - rgb() can accept alpha values
	// - Space-separated functional color notations
	Modern_RGB_HSL

	InsetProperty
)

func (features CSSFeature) Has(feature CSSFeature) bool {
	return (features & feature) != 0
}

var cssTable = map[CSSFeature]map[Engine][]versionRange{
	// Data from: https://developer.mozilla.org/en-US/docs/Web/CSS/color_value
	HexRGBA: {
		Chrome:  {{start: v{62, 0, 0}}},
		Edge:    {{start: v{79, 0, 0}}},
		Firefox: {{start: v{49, 0, 0}}},
		IOS:     {{start: v{9, 3, 0}}},
		Safari:  {{start: v{9, 1, 0}}},
	},
	RebeccaPurple: {
		Chrome:  {{start: v{38, 0, 0}}},
		Edge:    {{start: v{12, 0, 0}}},
		Firefox: {{start: v{33, 0, 0}}},
		IOS:     {{start: v{8, 0, 0}}},
		Safari:  {{start: v{9, 0, 0}}},
	},
	Modern_RGB_HSL: {
		Chrome:  {{start: v{66, 0, 0}}},
		Edge:    {{start: v{79, 0, 0}}},
		Firefox: {{start: v{52, 0, 0}}},
		IOS:     {{start: v{12, 2, 0}}},
		Safari:  {{start: v{12, 1, 0}}},
	},

	// Data from: https://developer.mozilla.org/en-US/docs/Web/CSS/inset
	InsetProperty: {
		Chrome:  {{start: v{87, 0, 0}}},
		Edge:    {{start: v{87, 0, 0}}},
		Firefox: {{start: v{66, 0, 0}}},
		IOS:     {{start: v{14, 5, 0}}},
		Safari:  {{start: v{14, 1, 0}}},
	},
}

// Return all features that are not available in at least one environment
func UnsupportedCSSFeatures(constraints map[Engine][]int) (unsupported CSSFeature) {
	for feature, engines := range cssTable {
		for engine, version := range constraints {
			if engine == ES || engine == Node {
				// Specifying "--target=es2020" shouldn't affect CSS
				continue
			}
			if versionRanges, ok := engines[engine]; !ok || !isVersionSupported(versionRanges, version) {
				unsupported |= feature
			}
		}
	}
	return
}
