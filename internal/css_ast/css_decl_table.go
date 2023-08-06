package css_ast

import (
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/helpers"
)

type D uint16

const (
	DUnknown D = iota
	DAlignContent
	DAlignItems
	DAlignSelf
	DAlignmentBaseline
	DAll
	DAnimation
	DAnimationDelay
	DAnimationDirection
	DAnimationDuration
	DAnimationFillMode
	DAnimationIterationCount
	DAnimationName
	DAnimationPlayState
	DAnimationTimingFunction
	DAppearance
	DBackdropFilter
	DBackfaceVisibility
	DBackground
	DBackgroundAttachment
	DBackgroundClip
	DBackgroundColor
	DBackgroundImage
	DBackgroundOrigin
	DBackgroundPosition
	DBackgroundPositionX
	DBackgroundPositionY
	DBackgroundRepeat
	DBackgroundSize
	DBaselineShift
	DBlockSize
	DBorder
	DBorderBlockEnd
	DBorderBlockEndColor
	DBorderBlockEndStyle
	DBorderBlockEndWidth
	DBorderBlockStart
	DBorderBlockStartColor
	DBorderBlockStartStyle
	DBorderBlockStartWidth
	DBorderBottom
	DBorderBottomColor
	DBorderBottomLeftRadius
	DBorderBottomRightRadius
	DBorderBottomStyle
	DBorderBottomWidth
	DBorderCollapse
	DBorderColor
	DBorderImage
	DBorderImageOutset
	DBorderImageRepeat
	DBorderImageSlice
	DBorderImageSource
	DBorderImageWidth
	DBorderInlineEnd
	DBorderInlineEndColor
	DBorderInlineEndStyle
	DBorderInlineEndWidth
	DBorderInlineStart
	DBorderInlineStartColor
	DBorderInlineStartStyle
	DBorderInlineStartWidth
	DBorderLeft
	DBorderLeftColor
	DBorderLeftStyle
	DBorderLeftWidth
	DBorderRadius
	DBorderRight
	DBorderRightColor
	DBorderRightStyle
	DBorderRightWidth
	DBorderSpacing
	DBorderStyle
	DBorderTop
	DBorderTopColor
	DBorderTopLeftRadius
	DBorderTopRightRadius
	DBorderTopStyle
	DBorderTopWidth
	DBorderWidth
	DBottom
	DBoxDecorationBreak
	DBoxShadow
	DBoxSizing
	DBreakAfter
	DBreakBefore
	DBreakInside
	DCaptionSide
	DCaretColor
	DClear
	DClip
	DClipPath
	DClipRule
	DColor
	DColorInterpolation
	DColorInterpolationFilters
	DColumnCount
	DColumnFill
	DColumnGap
	DColumnRule
	DColumnRuleColor
	DColumnRuleStyle
	DColumnRuleWidth
	DColumnSpan
	DColumnWidth
	DColumns
	DComposes
	DContainer
	DContainerName
	DContainerType
	DContent
	DCounterIncrement
	DCounterReset
	DCssFloat
	DCssText
	DCursor
	DDirection
	DDisplay
	DDominantBaseline
	DEmptyCells
	DFill
	DFillOpacity
	DFillRule
	DFilter
	DFlex
	DFlexBasis
	DFlexDirection
	DFlexFlow
	DFlexGrow
	DFlexShrink
	DFlexWrap
	DFloat
	DFloodColor
	DFloodOpacity
	DFont
	DFontFamily
	DFontFeatureSettings
	DFontKerning
	DFontSize
	DFontSizeAdjust
	DFontStretch
	DFontStyle
	DFontSynthesis
	DFontVariant
	DFontVariantCaps
	DFontVariantEastAsian
	DFontVariantLigatures
	DFontVariantNumeric
	DFontVariantPosition
	DFontWeight
	DGap
	DGlyphOrientationVertical
	DGrid
	DGridArea
	DGridAutoColumns
	DGridAutoFlow
	DGridAutoRows
	DGridColumn
	DGridColumnEnd
	DGridColumnGap
	DGridColumnStart
	DGridGap
	DGridRow
	DGridRowEnd
	DGridRowGap
	DGridRowStart
	DGridTemplate
	DGridTemplateAreas
	DGridTemplateColumns
	DGridTemplateRows
	DHeight
	DHyphens
	DImageOrientation
	DImageRendering
	DInitialLetter
	DInlineSize
	DInset
	DJustifyContent
	DJustifyItems
	DJustifySelf
	DLeft
	DLetterSpacing
	DLightingColor
	DLineBreak
	DLineHeight
	DListStyle
	DListStyleImage
	DListStylePosition
	DListStyleType
	DMargin
	DMarginBlockEnd
	DMarginBlockStart
	DMarginBottom
	DMarginInlineEnd
	DMarginInlineStart
	DMarginLeft
	DMarginRight
	DMarginTop
	DMarker
	DMarkerEnd
	DMarkerMid
	DMarkerStart
	DMask
	DMaskComposite
	DMaskImage
	DMaskOrigin
	DMaskPosition
	DMaskRepeat
	DMaskSize
	DMaskType
	DMaxBlockSize
	DMaxHeight
	DMaxInlineSize
	DMaxWidth
	DMinBlockSize
	DMinHeight
	DMinInlineSize
	DMinWidth
	DObjectFit
	DObjectPosition
	DOpacity
	DOrder
	DOrphans
	DOutline
	DOutlineColor
	DOutlineOffset
	DOutlineStyle
	DOutlineWidth
	DOverflow
	DOverflowAnchor
	DOverflowWrap
	DOverflowX
	DOverflowY
	DOverscrollBehavior
	DOverscrollBehaviorBlock
	DOverscrollBehaviorInline
	DOverscrollBehaviorX
	DOverscrollBehaviorY
	DPadding
	DPaddingBlockEnd
	DPaddingBlockStart
	DPaddingBottom
	DPaddingInlineEnd
	DPaddingInlineStart
	DPaddingLeft
	DPaddingRight
	DPaddingTop
	DPageBreakAfter
	DPageBreakBefore
	DPageBreakInside
	DPaintOrder
	DPerspective
	DPerspectiveOrigin
	DPlaceContent
	DPlaceItems
	DPlaceSelf
	DPointerEvents
	DPosition
	DPrintColorAdjust
	DQuotes
	DResize
	DRight
	DRotate
	DRowGap
	DRubyAlign
	DRubyPosition
	DScale
	DScrollBehavior
	DShapeRendering
	DStopColor
	DStopOpacity
	DStroke
	DStrokeDasharray
	DStrokeDashoffset
	DStrokeLinecap
	DStrokeLinejoin
	DStrokeMiterlimit
	DStrokeOpacity
	DStrokeWidth
	DTabSize
	DTableLayout
	DTextAlign
	DTextAlignLast
	DTextAnchor
	DTextCombineUpright
	DTextDecoration
	DTextDecorationColor
	DTextDecorationLine
	DTextDecorationSkip
	DTextDecorationStyle
	DTextEmphasis
	DTextEmphasisColor
	DTextEmphasisPosition
	DTextEmphasisStyle
	DTextIndent
	DTextJustify
	DTextOrientation
	DTextOverflow
	DTextRendering
	DTextShadow
	DTextSizeAdjust
	DTextTransform
	DTextUnderlinePosition
	DTop
	DTouchAction
	DTransform
	DTransformBox
	DTransformOrigin
	DTransformStyle
	DTransition
	DTransitionDelay
	DTransitionDuration
	DTransitionProperty
	DTransitionTimingFunction
	DTranslate
	DUnicodeBidi
	DUserSelect
	DVerticalAlign
	DVisibility
	DWhiteSpace
	DWidows
	DWidth
	DWillChange
	DWordBreak
	DWordSpacing
	DWordWrap
	DWritingMode
	DZIndex
	DZoom
)

var KnownDeclarations = map[string]D{
	"align-content":               DAlignContent,
	"align-items":                 DAlignItems,
	"align-self":                  DAlignSelf,
	"alignment-baseline":          DAlignmentBaseline,
	"all":                         DAll,
	"animation":                   DAnimation,
	"animation-delay":             DAnimationDelay,
	"animation-direction":         DAnimationDirection,
	"animation-duration":          DAnimationDuration,
	"animation-fill-mode":         DAnimationFillMode,
	"animation-iteration-count":   DAnimationIterationCount,
	"animation-name":              DAnimationName,
	"animation-play-state":        DAnimationPlayState,
	"animation-timing-function":   DAnimationTimingFunction,
	"appearance":                  DAppearance,
	"backdrop-filter":             DBackdropFilter,
	"backface-visibility":         DBackfaceVisibility,
	"background":                  DBackground,
	"background-attachment":       DBackgroundAttachment,
	"background-clip":             DBackgroundClip,
	"background-color":            DBackgroundColor,
	"background-image":            DBackgroundImage,
	"background-origin":           DBackgroundOrigin,
	"background-position":         DBackgroundPosition,
	"background-position-x":       DBackgroundPositionX,
	"background-position-y":       DBackgroundPositionY,
	"background-repeat":           DBackgroundRepeat,
	"background-size":             DBackgroundSize,
	"baseline-shift":              DBaselineShift,
	"block-size":                  DBlockSize,
	"border":                      DBorder,
	"border-block-end":            DBorderBlockEnd,
	"border-block-end-color":      DBorderBlockEndColor,
	"border-block-end-style":      DBorderBlockEndStyle,
	"border-block-end-width":      DBorderBlockEndWidth,
	"border-block-start":          DBorderBlockStart,
	"border-block-start-color":    DBorderBlockStartColor,
	"border-block-start-style":    DBorderBlockStartStyle,
	"border-block-start-width":    DBorderBlockStartWidth,
	"border-bottom":               DBorderBottom,
	"border-bottom-color":         DBorderBottomColor,
	"border-bottom-left-radius":   DBorderBottomLeftRadius,
	"border-bottom-right-radius":  DBorderBottomRightRadius,
	"border-bottom-style":         DBorderBottomStyle,
	"border-bottom-width":         DBorderBottomWidth,
	"border-collapse":             DBorderCollapse,
	"border-color":                DBorderColor,
	"border-image":                DBorderImage,
	"border-image-outset":         DBorderImageOutset,
	"border-image-repeat":         DBorderImageRepeat,
	"border-image-slice":          DBorderImageSlice,
	"border-image-source":         DBorderImageSource,
	"border-image-width":          DBorderImageWidth,
	"border-inline-end":           DBorderInlineEnd,
	"border-inline-end-color":     DBorderInlineEndColor,
	"border-inline-end-style":     DBorderInlineEndStyle,
	"border-inline-end-width":     DBorderInlineEndWidth,
	"border-inline-start":         DBorderInlineStart,
	"border-inline-start-color":   DBorderInlineStartColor,
	"border-inline-start-style":   DBorderInlineStartStyle,
	"border-inline-start-width":   DBorderInlineStartWidth,
	"border-left":                 DBorderLeft,
	"border-left-color":           DBorderLeftColor,
	"border-left-style":           DBorderLeftStyle,
	"border-left-width":           DBorderLeftWidth,
	"border-radius":               DBorderRadius,
	"border-right":                DBorderRight,
	"border-right-color":          DBorderRightColor,
	"border-right-style":          DBorderRightStyle,
	"border-right-width":          DBorderRightWidth,
	"border-spacing":              DBorderSpacing,
	"border-style":                DBorderStyle,
	"border-top":                  DBorderTop,
	"border-top-color":            DBorderTopColor,
	"border-top-left-radius":      DBorderTopLeftRadius,
	"border-top-right-radius":     DBorderTopRightRadius,
	"border-top-style":            DBorderTopStyle,
	"border-top-width":            DBorderTopWidth,
	"border-width":                DBorderWidth,
	"bottom":                      DBottom,
	"box-decoration-break":        DBoxDecorationBreak,
	"box-shadow":                  DBoxShadow,
	"box-sizing":                  DBoxSizing,
	"break-after":                 DBreakAfter,
	"break-before":                DBreakBefore,
	"break-inside":                DBreakInside,
	"caption-side":                DCaptionSide,
	"caret-color":                 DCaretColor,
	"clear":                       DClear,
	"clip":                        DClip,
	"clip-path":                   DClipPath,
	"clip-rule":                   DClipRule,
	"color":                       DColor,
	"color-interpolation":         DColorInterpolation,
	"color-interpolation-filters": DColorInterpolationFilters,
	"column-count":                DColumnCount,
	"column-fill":                 DColumnFill,
	"column-gap":                  DColumnGap,
	"column-rule":                 DColumnRule,
	"column-rule-color":           DColumnRuleColor,
	"column-rule-style":           DColumnRuleStyle,
	"column-rule-width":           DColumnRuleWidth,
	"column-span":                 DColumnSpan,
	"column-width":                DColumnWidth,
	"columns":                     DColumns,
	"composes":                    DComposes,
	"container":                   DContainer,
	"container-name":              DContainerName,
	"container-type":              DContainerType,
	"content":                     DContent,
	"counter-increment":           DCounterIncrement,
	"counter-reset":               DCounterReset,
	"css-float":                   DCssFloat,
	"css-text":                    DCssText,
	"cursor":                      DCursor,
	"direction":                   DDirection,
	"display":                     DDisplay,
	"dominant-baseline":           DDominantBaseline,
	"empty-cells":                 DEmptyCells,
	"fill":                        DFill,
	"fill-opacity":                DFillOpacity,
	"fill-rule":                   DFillRule,
	"filter":                      DFilter,
	"flex":                        DFlex,
	"flex-basis":                  DFlexBasis,
	"flex-direction":              DFlexDirection,
	"flex-flow":                   DFlexFlow,
	"flex-grow":                   DFlexGrow,
	"flex-shrink":                 DFlexShrink,
	"flex-wrap":                   DFlexWrap,
	"float":                       DFloat,
	"flood-color":                 DFloodColor,
	"flood-opacity":               DFloodOpacity,
	"font":                        DFont,
	"font-family":                 DFontFamily,
	"font-feature-settings":       DFontFeatureSettings,
	"font-kerning":                DFontKerning,
	"font-size":                   DFontSize,
	"font-size-adjust":            DFontSizeAdjust,
	"font-stretch":                DFontStretch,
	"font-style":                  DFontStyle,
	"font-synthesis":              DFontSynthesis,
	"font-variant":                DFontVariant,
	"font-variant-caps":           DFontVariantCaps,
	"font-variant-east-asian":     DFontVariantEastAsian,
	"font-variant-ligatures":      DFontVariantLigatures,
	"font-variant-numeric":        DFontVariantNumeric,
	"font-variant-position":       DFontVariantPosition,
	"font-weight":                 DFontWeight,
	"gap":                         DGap,
	"glyph-orientation-vertical":  DGlyphOrientationVertical,
	"grid":                        DGrid,
	"grid-area":                   DGridArea,
	"grid-auto-columns":           DGridAutoColumns,
	"grid-auto-flow":              DGridAutoFlow,
	"grid-auto-rows":              DGridAutoRows,
	"grid-column":                 DGridColumn,
	"grid-column-end":             DGridColumnEnd,
	"grid-column-gap":             DGridColumnGap,
	"grid-column-start":           DGridColumnStart,
	"grid-gap":                    DGridGap,
	"grid-row":                    DGridRow,
	"grid-row-end":                DGridRowEnd,
	"grid-row-gap":                DGridRowGap,
	"grid-row-start":              DGridRowStart,
	"grid-template":               DGridTemplate,
	"grid-template-areas":         DGridTemplateAreas,
	"grid-template-columns":       DGridTemplateColumns,
	"grid-template-rows":          DGridTemplateRows,
	"height":                      DHeight,
	"hyphens":                     DHyphens,
	"image-orientation":           DImageOrientation,
	"image-rendering":             DImageRendering,
	"initial-letter":              DInitialLetter,
	"inline-size":                 DInlineSize,
	"inset":                       DInset,
	"justify-content":             DJustifyContent,
	"justify-items":               DJustifyItems,
	"justify-self":                DJustifySelf,
	"left":                        DLeft,
	"letter-spacing":              DLetterSpacing,
	"lighting-color":              DLightingColor,
	"line-break":                  DLineBreak,
	"line-height":                 DLineHeight,
	"list-style":                  DListStyle,
	"list-style-image":            DListStyleImage,
	"list-style-position":         DListStylePosition,
	"list-style-type":             DListStyleType,
	"margin":                      DMargin,
	"margin-block-end":            DMarginBlockEnd,
	"margin-block-start":          DMarginBlockStart,
	"margin-bottom":               DMarginBottom,
	"margin-inline-end":           DMarginInlineEnd,
	"margin-inline-start":         DMarginInlineStart,
	"margin-left":                 DMarginLeft,
	"margin-right":                DMarginRight,
	"margin-top":                  DMarginTop,
	"marker":                      DMarker,
	"marker-end":                  DMarkerEnd,
	"marker-mid":                  DMarkerMid,
	"marker-start":                DMarkerStart,
	"mask":                        DMask,
	"mask-composite":              DMaskComposite,
	"mask-image":                  DMaskImage,
	"mask-origin":                 DMaskOrigin,
	"mask-position":               DMaskPosition,
	"mask-repeat":                 DMaskRepeat,
	"mask-size":                   DMaskSize,
	"mask-type":                   DMaskType,
	"max-block-size":              DMaxBlockSize,
	"max-height":                  DMaxHeight,
	"max-inline-size":             DMaxInlineSize,
	"max-width":                   DMaxWidth,
	"min-block-size":              DMinBlockSize,
	"min-height":                  DMinHeight,
	"min-inline-size":             DMinInlineSize,
	"min-width":                   DMinWidth,
	"object-fit":                  DObjectFit,
	"object-position":             DObjectPosition,
	"opacity":                     DOpacity,
	"order":                       DOrder,
	"orphans":                     DOrphans,
	"outline":                     DOutline,
	"outline-color":               DOutlineColor,
	"outline-offset":              DOutlineOffset,
	"outline-style":               DOutlineStyle,
	"outline-width":               DOutlineWidth,
	"overflow":                    DOverflow,
	"overflow-anchor":             DOverflowAnchor,
	"overflow-wrap":               DOverflowWrap,
	"overflow-x":                  DOverflowX,
	"overflow-y":                  DOverflowY,
	"overscroll-behavior":         DOverscrollBehavior,
	"overscroll-behavior-block":   DOverscrollBehaviorBlock,
	"overscroll-behavior-inline":  DOverscrollBehaviorInline,
	"overscroll-behavior-x":       DOverscrollBehaviorX,
	"overscroll-behavior-y":       DOverscrollBehaviorY,
	"padding":                     DPadding,
	"padding-block-end":           DPaddingBlockEnd,
	"padding-block-start":         DPaddingBlockStart,
	"padding-bottom":              DPaddingBottom,
	"padding-inline-end":          DPaddingInlineEnd,
	"padding-inline-start":        DPaddingInlineStart,
	"padding-left":                DPaddingLeft,
	"padding-right":               DPaddingRight,
	"padding-top":                 DPaddingTop,
	"page-break-after":            DPageBreakAfter,
	"page-break-before":           DPageBreakBefore,
	"page-break-inside":           DPageBreakInside,
	"paint-order":                 DPaintOrder,
	"perspective":                 DPerspective,
	"perspective-origin":          DPerspectiveOrigin,
	"place-content":               DPlaceContent,
	"place-items":                 DPlaceItems,
	"place-self":                  DPlaceSelf,
	"pointer-events":              DPointerEvents,
	"position":                    DPosition,
	"print-color-adjust":          DPrintColorAdjust,
	"quotes":                      DQuotes,
	"resize":                      DResize,
	"right":                       DRight,
	"rotate":                      DRotate,
	"row-gap":                     DRowGap,
	"ruby-align":                  DRubyAlign,
	"ruby-position":               DRubyPosition,
	"scale":                       DScale,
	"scroll-behavior":             DScrollBehavior,
	"shape-rendering":             DShapeRendering,
	"stop-color":                  DStopColor,
	"stop-opacity":                DStopOpacity,
	"stroke":                      DStroke,
	"stroke-dasharray":            DStrokeDasharray,
	"stroke-dashoffset":           DStrokeDashoffset,
	"stroke-linecap":              DStrokeLinecap,
	"stroke-linejoin":             DStrokeLinejoin,
	"stroke-miterlimit":           DStrokeMiterlimit,
	"stroke-opacity":              DStrokeOpacity,
	"stroke-width":                DStrokeWidth,
	"tab-size":                    DTabSize,
	"table-layout":                DTableLayout,
	"text-align":                  DTextAlign,
	"text-align-last":             DTextAlignLast,
	"text-anchor":                 DTextAnchor,
	"text-combine-upright":        DTextCombineUpright,
	"text-decoration":             DTextDecoration,
	"text-decoration-color":       DTextDecorationColor,
	"text-decoration-line":        DTextDecorationLine,
	"text-decoration-skip":        DTextDecorationSkip,
	"text-decoration-style":       DTextDecorationStyle,
	"text-emphasis":               DTextEmphasis,
	"text-emphasis-color":         DTextEmphasisColor,
	"text-emphasis-position":      DTextEmphasisPosition,
	"text-emphasis-style":         DTextEmphasisStyle,
	"text-indent":                 DTextIndent,
	"text-justify":                DTextJustify,
	"text-orientation":            DTextOrientation,
	"text-overflow":               DTextOverflow,
	"text-rendering":              DTextRendering,
	"text-shadow":                 DTextShadow,
	"text-size-adjust":            DTextSizeAdjust,
	"text-transform":              DTextTransform,
	"text-underline-position":     DTextUnderlinePosition,
	"top":                         DTop,
	"touch-action":                DTouchAction,
	"transform":                   DTransform,
	"transform-box":               DTransformBox,
	"transform-origin":            DTransformOrigin,
	"transform-style":             DTransformStyle,
	"transition":                  DTransition,
	"transition-delay":            DTransitionDelay,
	"transition-duration":         DTransitionDuration,
	"transition-property":         DTransitionProperty,
	"transition-timing-function":  DTransitionTimingFunction,
	"translate":                   DTranslate,
	"unicode-bidi":                DUnicodeBidi,
	"user-select":                 DUserSelect,
	"vertical-align":              DVerticalAlign,
	"visibility":                  DVisibility,
	"white-space":                 DWhiteSpace,
	"widows":                      DWidows,
	"width":                       DWidth,
	"will-change":                 DWillChange,
	"word-break":                  DWordBreak,
	"word-spacing":                DWordSpacing,
	"word-wrap":                   DWordWrap,
	"writing-mode":                DWritingMode,
	"z-index":                     DZIndex,
	"zoom":                        DZoom,
}

var typoDetector *helpers.TypoDetector
var typoDetectorMutex sync.Mutex

func MaybeCorrectDeclarationTypo(text string) (string, bool) {
	// Ignore CSS variables, which should not be corrected to CSS properties
	if strings.HasPrefix(text, "--") {
		return "", false
	}

	typoDetectorMutex.Lock()
	defer typoDetectorMutex.Unlock()

	// Lazily-initialize the typo detector for speed when it's not needed
	if typoDetector == nil {
		valid := make([]string, 0, len(KnownDeclarations))
		for key := range KnownDeclarations {
			valid = append(valid, key)
		}
		detector := helpers.MakeTypoDetector(valid)
		typoDetector = &detector
	}

	return typoDetector.MaybeCorrectTypo(text)
}
