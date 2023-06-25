package compat

import (
	"github.com/evanw/esbuild/internal/css_ast"
)

type CSSFeature uint8

const (
	HexRGBA CSSFeature = 1 << iota
	InlineStyle
	RebeccaPurple

	// This feature includes all of the following:
	// - Allow floats in rgb() and rgba()
	// - hsl() can accept alpha values
	// - rgb() can accept alpha values
	// - Space-separated functional color notations
	Modern_RGB_HSL

	InsetProperty
	Nesting
	IsPseudoClass
)

var StringToCSSFeature = map[string]CSSFeature{
	"hex-rgba":        HexRGBA,
	"inline-style":    InlineStyle,
	"rebecca-purple":  RebeccaPurple,
	"modern-rgb-hsl":  Modern_RGB_HSL,
	"inset-property":  InsetProperty,
	"nesting":         Nesting,
	"is-pseudo-class": IsPseudoClass,
}

func (features CSSFeature) Has(feature CSSFeature) bool {
	return (features & feature) != 0
}

func (features CSSFeature) ApplyOverrides(overrides CSSFeature, mask CSSFeature) CSSFeature {
	return (features & ^mask) | (overrides & mask)
}

var cssTable = map[CSSFeature]map[Engine][]versionRange{
	// Data from: https://developer.mozilla.org/en-US/docs/Web/CSS/color_value
	HexRGBA: {
		Chrome:  {{start: v{62, 0, 0}}},
		Edge:    {{start: v{79, 0, 0}}},
		Firefox: {{start: v{49, 0, 0}}},
		IOS:     {{start: v{9, 3, 0}}},
		Opera:   {{start: v{49, 0, 0}}},
		Safari:  {{start: v{9, 1, 0}}},
	},
	RebeccaPurple: {
		Chrome:  {{start: v{38, 0, 0}}},
		Edge:    {{start: v{12, 0, 0}}},
		Firefox: {{start: v{33, 0, 0}}},
		IE:      {{start: v{11, 0, 0}}},
		IOS:     {{start: v{8, 0, 0}}},
		Opera:   {{start: v{25, 0, 0}}},
		Safari:  {{start: v{9, 0, 0}}},
	},
	Modern_RGB_HSL: {
		Chrome:  {{start: v{66, 0, 0}}},
		Edge:    {{start: v{79, 0, 0}}},
		Firefox: {{start: v{52, 0, 0}}},
		IOS:     {{start: v{12, 2, 0}}},
		Opera:   {{start: v{53, 0, 0}}},
		Safari:  {{start: v{12, 1, 0}}},
	},

	// Data from: https://developer.mozilla.org/en-US/docs/Web/CSS/inset
	InsetProperty: {
		Chrome:  {{start: v{87, 0, 0}}},
		Edge:    {{start: v{87, 0, 0}}},
		Firefox: {{start: v{66, 0, 0}}},
		IOS:     {{start: v{14, 5, 0}}},
		Opera:   {{start: v{73, 0, 0}}},
		Safari:  {{start: v{14, 1, 0}}},
	},

	// Data from: https://caniuse.com/css-nesting
	Nesting: {
		Chrome: {{start: v{112, 0, 0}}},
	},

	// Data from: https://caniuse.com/css-matches-pseudo
	IsPseudoClass: {
		Chrome:  {{start: v{88, 0, 0}}},
		Edge:    {{start: v{88, 0, 0}}},
		Firefox: {{start: v{78, 0, 0}}},
		IOS:     {{start: v{14, 0, 0}}},
		Opera:   {{start: v{75, 0, 0}}},
		Safari:  {{start: v{14, 0, 0}}},
	},
}

// Return all features that are not available in at least one environment
func UnsupportedCSSFeatures(constraints map[Engine][]int) (unsupported CSSFeature) {
	for feature, engines := range cssTable {
		if feature == InlineStyle {
			continue // This is purely user-specified
		}
		for engine, version := range constraints {
			if !engine.IsBrowser() {
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

type CSSPrefix uint8

const (
	WebkitPrefix CSSPrefix = 1 << iota
	MozPrefix
	MsPrefix
	OPrefix

	NoPrefix CSSPrefix = 0
)

type prefixData struct {
	// Note: In some cases, earlier versions did not require a prefix but later
	// ones do. This is the case for Microsoft Edge for example, which switched
	// the underlying browser engine from a custom one to the one from Chrome.
	// However, we assume that users specifying a browser version for CSS mean
	// "works in this version or newer", so we still add a prefix when a target
	// is an old Edge version.
	withoutPrefix v
	prefix        CSSPrefix
}

var cssMaskPrefixTable = map[Engine]prefixData{
	Chrome: {prefix: WebkitPrefix},
	Edge:   {prefix: WebkitPrefix},
	IOS:    {prefix: WebkitPrefix, withoutPrefix: v{15, 4, 0}},
	Opera:  {prefix: WebkitPrefix},
	Safari: {prefix: WebkitPrefix, withoutPrefix: v{15, 4, 0}},
}

var cssPrefixTable = map[css_ast.D]map[Engine]prefixData{
	// https://caniuse.com/css-appearance
	css_ast.DAppearance: {
		Chrome:  {prefix: WebkitPrefix, withoutPrefix: v{84, 0, 0}},
		Edge:    {prefix: WebkitPrefix, withoutPrefix: v{84, 0, 0}},
		Firefox: {prefix: MozPrefix, withoutPrefix: v{80, 4, 0}},
		IOS:     {prefix: WebkitPrefix, withoutPrefix: v{15, 4, 0}},
		Opera:   {prefix: WebkitPrefix, withoutPrefix: v{73, 4, 0}},
		Safari:  {prefix: WebkitPrefix, withoutPrefix: v{15, 4, 0}},
	},

	// https://caniuse.com/css-backdrop-filter
	css_ast.DBackdropFilter: {
		IOS:    {prefix: WebkitPrefix},
		Safari: {prefix: WebkitPrefix},
	},

	// https://caniuse.com/background-clip-text (Note: only for "background-clip: text")
	css_ast.DBackgroundClip: {
		Chrome: {prefix: WebkitPrefix},
		Edge:   {prefix: WebkitPrefix},
		IOS:    {prefix: WebkitPrefix, withoutPrefix: v{14, 0, 0}},
		Opera:  {prefix: WebkitPrefix},
		Safari: {prefix: WebkitPrefix, withoutPrefix: v{14, 0, 0}},
	},

	// https://caniuse.com/css-clip-path
	css_ast.DClipPath: {
		Chrome: {prefix: WebkitPrefix, withoutPrefix: v{55, 0, 0}},
		IOS:    {prefix: WebkitPrefix, withoutPrefix: v{13, 0, 0}},
		Opera:  {prefix: WebkitPrefix, withoutPrefix: v{42, 0, 0}},
		Safari: {prefix: WebkitPrefix, withoutPrefix: v{13, 1, 0}},
	},

	// https://caniuse.com/font-kerning
	css_ast.DFontKerning: {
		Chrome: {prefix: WebkitPrefix, withoutPrefix: v{33, 0, 0}},
		IOS:    {prefix: WebkitPrefix, withoutPrefix: v{12, 0, 0}},
		Opera:  {prefix: WebkitPrefix, withoutPrefix: v{20, 0, 0}},
		Safari: {prefix: WebkitPrefix, withoutPrefix: v{9, 1, 0}},
	},

	// https://caniuse.com/css-hyphens
	css_ast.DHyphens: {
		Edge:    {prefix: MsPrefix, withoutPrefix: v{79, 0, 0}},
		Firefox: {prefix: MozPrefix, withoutPrefix: v{43, 0, 0}},
		IE:      {prefix: MsPrefix},
		IOS:     {prefix: WebkitPrefix},
		Safari:  {prefix: WebkitPrefix},
	},

	// https://caniuse.com/css-initial-letter
	css_ast.DInitialLetter: {
		IOS:    {prefix: WebkitPrefix},
		Safari: {prefix: WebkitPrefix},
	},

	css_ast.DMaskImage:    cssMaskPrefixTable, // https://caniuse.com/mdn-css_properties_mask-image
	css_ast.DMaskOrigin:   cssMaskPrefixTable, // https://caniuse.com/mdn-css_properties_mask-origin
	css_ast.DMaskPosition: cssMaskPrefixTable, // https://caniuse.com/mdn-css_properties_mask-position
	css_ast.DMaskRepeat:   cssMaskPrefixTable, // https://caniuse.com/mdn-css_properties_mask-repeat
	css_ast.DMaskSize:     cssMaskPrefixTable, // https://caniuse.com/mdn-css_properties_mask-size

	// https://caniuse.com/css-sticky
	css_ast.DPosition: {
		IOS:    {prefix: WebkitPrefix, withoutPrefix: v{13, 0, 0}},
		Safari: {prefix: WebkitPrefix, withoutPrefix: v{13, 0, 0}},
	},

	// https://caniuse.com/css-color-adjust
	css_ast.DPrintColorAdjust: {
		Chrome: {prefix: WebkitPrefix},
		Edge:   {prefix: WebkitPrefix},
		Opera:  {prefix: WebkitPrefix},
		Safari: {prefix: WebkitPrefix, withoutPrefix: v{15, 4, 0}},
	},

	// https://caniuse.com/css3-tabsize
	css_ast.DTabSize: {
		Firefox: {prefix: MozPrefix, withoutPrefix: v{91, 0, 0}},
		Opera:   {prefix: OPrefix, withoutPrefix: v{15, 0, 0}},
	},

	// https://caniuse.com/css-text-orientation
	css_ast.DTextOrientation: {
		Safari: {prefix: WebkitPrefix, withoutPrefix: v{14, 0, 0}},
	},

	// https://caniuse.com/text-size-adjust
	css_ast.DTextSizeAdjust: {
		Edge: {prefix: MsPrefix, withoutPrefix: v{79, 0, 0}},
		IOS:  {prefix: WebkitPrefix},
	},

	// https://caniuse.com/mdn-css_properties_user-select
	css_ast.DUserSelect: {
		Chrome:  {prefix: WebkitPrefix, withoutPrefix: v{54, 0, 0}},
		Edge:    {prefix: MsPrefix, withoutPrefix: v{79, 0, 0}},
		Firefox: {prefix: MozPrefix, withoutPrefix: v{69, 0, 0}},
		IOS:     {prefix: WebkitPrefix},
		Opera:   {prefix: WebkitPrefix, withoutPrefix: v{41, 0, 0}},
		Safari:  {prefix: WebkitPrefix},
		IE:      {prefix: MsPrefix},
	},
}

func CSSPrefixData(constraints map[Engine][]int) (entries map[css_ast.D]CSSPrefix) {
	for property, engines := range cssPrefixTable {
		prefixes := NoPrefix
		for engine, version := range constraints {
			if !engine.IsBrowser() {
				// Specifying "--target=es2020" shouldn't affect CSS
				continue
			}
			if data, ok := engines[engine]; ok && (data.withoutPrefix == v{} || compareVersions(data.withoutPrefix, version) > 0) {
				prefixes |= data.prefix
			}
		}
		if prefixes != NoPrefix {
			if entries == nil {
				entries = make(map[css_ast.D]CSSPrefix)
			}
			entries[property] = prefixes
		}
	}
	return
}
