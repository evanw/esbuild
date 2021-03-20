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
)

func (features CSSFeature) Has(feature CSSFeature) bool {
	return (features & feature) != 0
}

var cssTable = map[CSSFeature]map[Engine][]int{
	// Data from: https://developer.mozilla.org/en-US/docs/Web/CSS/color_value
	HexRGBA: {
		Chrome:  {62},
		Edge:    {79},
		Firefox: {49},
		IOS:     {9, 3},
		Safari:  {9, 1},
	},
	RebeccaPurple: {
		Chrome:  {38},
		Edge:    {12},
		Firefox: {33},
		IOS:     {8},
		Safari:  {9},
	},
	Modern_RGB_HSL: {
		Chrome:  {66},
		Edge:    {79},
		Firefox: {52},
		IOS:     {12, 2},
		Safari:  {12, 1},
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
			if minVersion, ok := engines[engine]; !ok || isVersionLessThan(version, minVersion) {
				unsupported |= feature
			}
		}
	}
	return
}
