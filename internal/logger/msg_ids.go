package logger

// Most non-error log messages are given a message ID that can be used to set
// the log level for that message. Errors do not get a message ID because you
// cannot turn errors into non-errors (otherwise the build would incorrectly
// succeed). Some internal log messages do not get a message ID because they
// are part of verbose and/or internal debugging output. These messages use
// "MsgID_None" instead.
type MsgID = uint8

const (
	MsgID_None MsgID = iota

	// JavaScript
	MsgID_JS_AssignToConstant
	MsgID_JS_CallImportNamespace
	MsgID_JS_CommonJSVariableInESM
	MsgID_JS_DeleteSuperProperty
	MsgID_JS_DirectEval
	MsgID_JS_DuplicateCase
	MsgID_JS_DuplicateObjectKey
	MsgID_JS_EmptyImportMeta
	MsgID_JS_EqualsNaN
	MsgID_JS_EqualsNegativeZero
	MsgID_JS_EqualsNewObject
	MsgID_JS_HTMLCommentInJS
	MsgID_JS_ImpossibleTypeof
	MsgID_JS_PrivateNameWillThrow
	MsgID_JS_SemicolonAfterReturn
	MsgID_JS_SuspiciousBooleanNot
	MsgID_JS_ThisIsUndefinedInESM
	MsgID_JS_UnsupportedDynamicImport
	MsgID_JS_UnsupportedJSXComment
	MsgID_JS_UnsupportedRegExp
	MsgID_JS_UnsupportedRequireCall

	// CSS
	MsgID_CSS_CSSSyntaxError
	MsgID_CSS_InvalidAtCharset
	MsgID_CSS_InvalidAtImport
	MsgID_CSS_InvalidAtLayer
	MsgID_CSS_InvalidAtNest
	MsgID_CSS_InvalidCalc
	MsgID_CSS_JSCommentInCSS
	MsgID_CSS_UnsupportedAtCharset
	MsgID_CSS_UnsupportedAtNamespace
	MsgID_CSS_UnsupportedCSSProperty

	// Bundler
	MsgID_Bundler_DifferentPathCase
	MsgID_Bundler_IgnoredBareImport
	MsgID_Bundler_IgnoredDynamicImport
	MsgID_Bundler_ImportIsUndefined
	MsgID_Bundler_RequireResolveNotExternal

	// Source maps
	MsgID_SourceMap_InvalidSourceMappings
	MsgID_SourceMap_SectionsInSourceMap
	MsgID_SourceMap_MissingSourceMap
	MsgID_SourceMap_UnsupportedSourceMapComment

	// package.json
	MsgID_PackageJSON_FIRST // Keep this first
	MsgID_PackageJSON_InvalidBrowser
	MsgID_PackageJSON_InvalidImportsOrExports
	MsgID_PackageJSON_InvalidSideEffects
	MsgID_PackageJSON_InvalidType
	MsgID_PackageJSON_LAST // Keep this last

	// tsconfig.json
	MsgID_TsconfigJSON_FIRST // Keep this first
	MsgID_TsconfigJSON_Cycle
	MsgID_TsconfigJSON_InvalidImportsNotUsedAsValues
	MsgID_TsconfigJSON_InvalidJSX
	MsgID_TsconfigJSON_InvalidModuleSuffixes
	MsgID_TsconfigJSON_InvalidPaths
	MsgID_TsconfigJSON_InvalidTarget
	MsgID_TsconfigJSON_Missing
	MsgID_TsconfigJSON_LAST // Keep this last
)
