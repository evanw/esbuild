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

	MsgID_END // Keep this at the end (used only for tests)
)

func StringToMsgIDs(str string, logLevel LogLevel, overrides map[MsgID]LogLevel) {
	switch str {
	// JS
	case "assign-to-constant":
		overrides[MsgID_JS_AssignToConstant] = logLevel
	case "call-import-namespace":
		overrides[MsgID_JS_CallImportNamespace] = logLevel
	case "commonjs-variable-in-esm":
		overrides[MsgID_JS_CommonJSVariableInESM] = logLevel
	case "delete-super-property":
		overrides[MsgID_JS_DeleteSuperProperty] = logLevel
	case "direct-eval":
		overrides[MsgID_JS_DirectEval] = logLevel
	case "duplicate-case":
		overrides[MsgID_JS_DuplicateCase] = logLevel
	case "duplicate-object-key":
		overrides[MsgID_JS_DuplicateObjectKey] = logLevel
	case "empty-import-meta":
		overrides[MsgID_JS_EmptyImportMeta] = logLevel
	case "equals-nan":
		overrides[MsgID_JS_EqualsNaN] = logLevel
	case "equals-negative-zero":
		overrides[MsgID_JS_EqualsNegativeZero] = logLevel
	case "equals-new-object":
		overrides[MsgID_JS_EqualsNewObject] = logLevel
	case "html-comment-in-js":
		overrides[MsgID_JS_HTMLCommentInJS] = logLevel
	case "impossible-typeof":
		overrides[MsgID_JS_ImpossibleTypeof] = logLevel
	case "private-name-will-throw":
		overrides[MsgID_JS_PrivateNameWillThrow] = logLevel
	case "semicolon-after-return":
		overrides[MsgID_JS_SemicolonAfterReturn] = logLevel
	case "suspicious-boolean-not":
		overrides[MsgID_JS_SuspiciousBooleanNot] = logLevel
	case "this-is-undefined-in-esm":
		overrides[MsgID_JS_ThisIsUndefinedInESM] = logLevel
	case "unsupported-dynamic-import":
		overrides[MsgID_JS_UnsupportedDynamicImport] = logLevel
	case "unsupported-jsx-comment":
		overrides[MsgID_JS_UnsupportedJSXComment] = logLevel
	case "unsupported-regexp":
		overrides[MsgID_JS_UnsupportedRegExp] = logLevel
	case "unsupported-require-call":
		overrides[MsgID_JS_UnsupportedRequireCall] = logLevel

	// CSS
	case "css-syntax-error":
		overrides[MsgID_CSS_CSSSyntaxError] = logLevel
	case "invalid-@charset":
		overrides[MsgID_CSS_InvalidAtCharset] = logLevel
	case "invalid-@import":
		overrides[MsgID_CSS_InvalidAtImport] = logLevel
	case "invalid-@nest":
		overrides[MsgID_CSS_InvalidAtNest] = logLevel
	case "invalid-@layer":
		overrides[MsgID_CSS_InvalidAtLayer] = logLevel
	case "invalid-calc":
		overrides[MsgID_CSS_InvalidCalc] = logLevel
	case "js-comment-in-css":
		overrides[MsgID_CSS_JSCommentInCSS] = logLevel
	case "unsupported-@charset":
		overrides[MsgID_CSS_UnsupportedAtCharset] = logLevel
	case "unsupported-@namespace":
		overrides[MsgID_CSS_UnsupportedAtNamespace] = logLevel
	case "unsupported-css-property":
		overrides[MsgID_CSS_UnsupportedCSSProperty] = logLevel

	// Bundler
	case "different-path-case":
		overrides[MsgID_Bundler_DifferentPathCase] = logLevel
	case "ignored-bare-import":
		overrides[MsgID_Bundler_IgnoredBareImport] = logLevel
	case "ignored-dynamic-import":
		overrides[MsgID_Bundler_IgnoredDynamicImport] = logLevel
	case "import-is-undefined":
		overrides[MsgID_Bundler_ImportIsUndefined] = logLevel
	case "require-resolve-not-external":
		overrides[MsgID_Bundler_RequireResolveNotExternal] = logLevel

	// Source maps
	case "invalid-source-mappings":
		overrides[MsgID_SourceMap_InvalidSourceMappings] = logLevel
	case "sections-in-source-map":
		overrides[MsgID_SourceMap_SectionsInSourceMap] = logLevel
	case "missing-source-map":
		overrides[MsgID_SourceMap_MissingSourceMap] = logLevel
	case "unsupported-source-map-comment":
		overrides[MsgID_SourceMap_UnsupportedSourceMapComment] = logLevel

	case "package.json":
		for i := MsgID_PackageJSON_FIRST; i <= MsgID_PackageJSON_LAST; i++ {
			overrides[i] = logLevel
		}

	case "tsconfig.json":
		for i := MsgID_TsconfigJSON_FIRST; i <= MsgID_TsconfigJSON_LAST; i++ {
			overrides[i] = logLevel
		}

	default:
		// Ignore invalid entries since this message id may have
		// been renamed/removed since when this code was written
	}
}

func MsgIDToString(id MsgID) string {
	switch id {
	// JS
	case MsgID_JS_AssignToConstant:
		return "assign-to-constant"
	case MsgID_JS_CallImportNamespace:
		return "call-import-namespace"
	case MsgID_JS_CommonJSVariableInESM:
		return "commonjs-variable-in-esm"
	case MsgID_JS_DeleteSuperProperty:
		return "delete-super-property"
	case MsgID_JS_DirectEval:
		return "direct-eval"
	case MsgID_JS_DuplicateCase:
		return "duplicate-case"
	case MsgID_JS_DuplicateObjectKey:
		return "duplicate-object-key"
	case MsgID_JS_EmptyImportMeta:
		return "empty-import-meta"
	case MsgID_JS_EqualsNaN:
		return "equals-nan"
	case MsgID_JS_EqualsNegativeZero:
		return "equals-negative-zero"
	case MsgID_JS_EqualsNewObject:
		return "equals-new-object"
	case MsgID_JS_HTMLCommentInJS:
		return "html-comment-in-js"
	case MsgID_JS_ImpossibleTypeof:
		return "impossible-typeof"
	case MsgID_JS_PrivateNameWillThrow:
		return "private-name-will-throw"
	case MsgID_JS_SemicolonAfterReturn:
		return "semicolon-after-return"
	case MsgID_JS_SuspiciousBooleanNot:
		return "suspicious-boolean-not"
	case MsgID_JS_ThisIsUndefinedInESM:
		return "this-is-undefined-in-esm"
	case MsgID_JS_UnsupportedDynamicImport:
		return "unsupported-dynamic-import"
	case MsgID_JS_UnsupportedJSXComment:
		return "unsupported-jsx-comment"
	case MsgID_JS_UnsupportedRegExp:
		return "unsupported-regexp"
	case MsgID_JS_UnsupportedRequireCall:
		return "unsupported-require-call"

	// CSS
	case MsgID_CSS_CSSSyntaxError:
		return "css-syntax-error"
	case MsgID_CSS_InvalidAtCharset:
		return "invalid-@charset"
	case MsgID_CSS_InvalidAtImport:
		return "invalid-@import"
	case MsgID_CSS_InvalidAtNest:
		return "invalid-@nest"
	case MsgID_CSS_InvalidAtLayer:
		return "invalid-@layer"
	case MsgID_CSS_InvalidCalc:
		return "invalid-calc"
	case MsgID_CSS_JSCommentInCSS:
		return "js-comment-in-css"
	case MsgID_CSS_UnsupportedAtCharset:
		return "unsupported-@charset"
	case MsgID_CSS_UnsupportedAtNamespace:
		return "unsupported-@namespace"
	case MsgID_CSS_UnsupportedCSSProperty:
		return "unsupported-css-property"

	// Bundler
	case MsgID_Bundler_DifferentPathCase:
		return "different-path-case"
	case MsgID_Bundler_IgnoredBareImport:
		return "ignored-bare-import"
	case MsgID_Bundler_IgnoredDynamicImport:
		return "ignored-dynamic-import"
	case MsgID_Bundler_ImportIsUndefined:
		return "import-is-undefined"
	case MsgID_Bundler_RequireResolveNotExternal:
		return "require-resolve-not-external"

	// Source maps
	case MsgID_SourceMap_InvalidSourceMappings:
		return "invalid-source-mappings"
	case MsgID_SourceMap_SectionsInSourceMap:
		return "sections-in-source-map"
	case MsgID_SourceMap_MissingSourceMap:
		return "missing-source-map"
	case MsgID_SourceMap_UnsupportedSourceMapComment:
		return "unsupported-source-map-comment"

	default:
		if id >= MsgID_PackageJSON_FIRST && id <= MsgID_PackageJSON_LAST {
			return "package.json"
		}
		if id >= MsgID_TsconfigJSON_FIRST && id <= MsgID_TsconfigJSON_LAST {
			return "tsconfig.json"
		}
	}

	return ""
}
