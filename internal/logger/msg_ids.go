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
	MsgID_JS_AssertToWith
	MsgID_JS_AssertTypeJSON
	MsgID_JS_AssignToConstant
	MsgID_JS_AssignToDefine
	MsgID_JS_AssignToImport
	MsgID_JS_BigInt
	MsgID_JS_CallImportNamespace
	MsgID_JS_ClassNameWillThrow
	MsgID_JS_CommonJSVariableInESM
	MsgID_JS_ConfusingTypeScriptCast
	MsgID_JS_DeleteSuperProperty
	MsgID_JS_DirectEval
	MsgID_JS_DuplicateCase
	MsgID_JS_DuplicateClassMember
	MsgID_JS_DuplicateObjectKey
	MsgID_JS_EmptyImportMeta
	MsgID_JS_EqualsNaN
	MsgID_JS_EqualsNegativeZero
	MsgID_JS_EqualsNewObject
	MsgID_JS_HTMLCommentInJS
	MsgID_JS_ImpossibleTypeof
	MsgID_JS_IndirectRequire
	MsgID_JS_PrivateNameWillThrow
	MsgID_JS_SemicolonAfterReturn
	MsgID_JS_SuspiciousBooleanNot
	MsgID_JS_SuspiciousDefine
	MsgID_JS_SuspiciousLogicalOperator
	MsgID_JS_SuspiciousNullishCoalescing
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
	MsgID_CSS_InvalidCalc
	MsgID_CSS_JSCommentInCSS
	MsgID_CSS_UndefinedComposesFrom
	MsgID_CSS_UnsupportedAtCharset
	MsgID_CSS_UnsupportedAtNamespace
	MsgID_CSS_UnsupportedCSSProperty
	MsgID_CSS_UnsupportedCSSNesting

	// Bundler
	MsgID_Bundler_AmbiguousReexport
	MsgID_Bundler_DifferentPathCase
	MsgID_Bundler_EmptyGlob
	MsgID_Bundler_IgnoredBareImport
	MsgID_Bundler_IgnoredDynamicImport
	MsgID_Bundler_ImportIsUndefined
	MsgID_Bundler_RequireResolveNotExternal

	// Source maps
	MsgID_SourceMap_InvalidSourceMappings
	MsgID_SourceMap_MissingSourceMap
	MsgID_SourceMap_UnsupportedSourceMapComment

	// package.json
	MsgID_PackageJSON_FIRST // Keep this first
	MsgID_PackageJSON_DeadCondition
	MsgID_PackageJSON_InvalidBrowser
	MsgID_PackageJSON_InvalidImportsOrExports
	MsgID_PackageJSON_InvalidSideEffects
	MsgID_PackageJSON_InvalidType
	MsgID_PackageJSON_LAST // Keep this last

	// tsconfig.json
	MsgID_TSConfigJSON_FIRST // Keep this first
	MsgID_TSConfigJSON_Cycle
	MsgID_TSConfigJSON_InvalidImportsNotUsedAsValues
	MsgID_TSConfigJSON_InvalidJSX
	MsgID_TSConfigJSON_InvalidPaths
	MsgID_TSConfigJSON_InvalidTarget
	MsgID_TSConfigJSON_InvalidTopLevelOption
	MsgID_TSConfigJSON_Missing
	MsgID_TSConfigJSON_LAST // Keep this last

	MsgID_END // Keep this at the end (used only for tests)
)

func StringToMsgIDs(str string, logLevel LogLevel, overrides map[MsgID]LogLevel) {
	switch str {
	// JS
	case "assert-to-with":
		overrides[MsgID_JS_AssertToWith] = logLevel
	case "assert-type-json":
		overrides[MsgID_JS_AssertTypeJSON] = logLevel
	case "assign-to-constant":
		overrides[MsgID_JS_AssignToConstant] = logLevel
	case "assign-to-define":
		overrides[MsgID_JS_AssignToDefine] = logLevel
	case "assign-to-import":
		overrides[MsgID_JS_AssignToImport] = logLevel
	case "bigint":
		overrides[MsgID_JS_BigInt] = logLevel
	case "call-import-namespace":
		overrides[MsgID_JS_CallImportNamespace] = logLevel
	case "class-name-will-throw":
		overrides[MsgID_JS_ClassNameWillThrow] = logLevel
	case "commonjs-variable-in-esm":
		overrides[MsgID_JS_CommonJSVariableInESM] = logLevel
	case "confusing-typescript-cast":
		overrides[MsgID_JS_ConfusingTypeScriptCast] = logLevel
	case "delete-super-property":
		overrides[MsgID_JS_DeleteSuperProperty] = logLevel
	case "direct-eval":
		overrides[MsgID_JS_DirectEval] = logLevel
	case "duplicate-case":
		overrides[MsgID_JS_DuplicateCase] = logLevel
	case "duplicate-class-member":
		overrides[MsgID_JS_DuplicateClassMember] = logLevel
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
	case "indirect-require":
		overrides[MsgID_JS_IndirectRequire] = logLevel
	case "private-name-will-throw":
		overrides[MsgID_JS_PrivateNameWillThrow] = logLevel
	case "semicolon-after-return":
		overrides[MsgID_JS_SemicolonAfterReturn] = logLevel
	case "suspicious-boolean-not":
		overrides[MsgID_JS_SuspiciousBooleanNot] = logLevel
	case "suspicious-define":
		overrides[MsgID_JS_SuspiciousDefine] = logLevel
	case "suspicious-logical-operator":
		overrides[MsgID_JS_SuspiciousLogicalOperator] = logLevel
	case "suspicious-nullish-coalescing":
		overrides[MsgID_JS_SuspiciousNullishCoalescing] = logLevel
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
	case "invalid-@layer":
		overrides[MsgID_CSS_InvalidAtLayer] = logLevel
	case "invalid-calc":
		overrides[MsgID_CSS_InvalidCalc] = logLevel
	case "js-comment-in-css":
		overrides[MsgID_CSS_JSCommentInCSS] = logLevel
	case "undefined-composes-from":
		overrides[MsgID_CSS_UndefinedComposesFrom] = logLevel
	case "unsupported-@charset":
		overrides[MsgID_CSS_UnsupportedAtCharset] = logLevel
	case "unsupported-@namespace":
		overrides[MsgID_CSS_UnsupportedAtNamespace] = logLevel
	case "unsupported-css-property":
		overrides[MsgID_CSS_UnsupportedCSSProperty] = logLevel
	case "unsupported-css-nesting":
		overrides[MsgID_CSS_UnsupportedCSSNesting] = logLevel

	// Bundler
	case "ambiguous-reexport":
		overrides[MsgID_Bundler_AmbiguousReexport] = logLevel
	case "different-path-case":
		overrides[MsgID_Bundler_DifferentPathCase] = logLevel
	case "empty-glob":
		overrides[MsgID_Bundler_EmptyGlob] = logLevel
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
	case "missing-source-map":
		overrides[MsgID_SourceMap_MissingSourceMap] = logLevel
	case "unsupported-source-map-comment":
		overrides[MsgID_SourceMap_UnsupportedSourceMapComment] = logLevel

	case "package.json":
		for i := MsgID_PackageJSON_FIRST; i <= MsgID_PackageJSON_LAST; i++ {
			overrides[i] = logLevel
		}

	case "tsconfig.json":
		for i := MsgID_TSConfigJSON_FIRST; i <= MsgID_TSConfigJSON_LAST; i++ {
			overrides[i] = logLevel
		}

	default:
		// Ignore invalid entries since this message id may have
		// been renamed/removed since when this code was written
	}
}

// These are stored together to make it less accident-prone to edit them
type msgIDInfo struct {
	name string
	vsID vsID
}

func MsgIDToString(id MsgID) string {
	return msgIDToInfo(id).name
}

func msgIDToInfo(id MsgID) msgIDInfo {
	switch id {
	// JS
	case MsgID_JS_AssertToWith:
		return msgIDInfo{name: "assert-to-with", vsID: vsID_JS_AssertToWith}
	case MsgID_JS_AssertTypeJSON:
		return msgIDInfo{name: "assert-type-json", vsID: vsID_JS_AssertTypeJSON}
	case MsgID_JS_AssignToConstant:
		return msgIDInfo{name: "assign-to-constant", vsID: vsID_JS_AssignToConstant}
	case MsgID_JS_AssignToDefine:
		return msgIDInfo{name: "assign-to-define", vsID: vsID_JS_AssignToDefine}
	case MsgID_JS_AssignToImport:
		return msgIDInfo{name: "assign-to-import", vsID: vsID_JS_AssignToImport}
	case MsgID_JS_BigInt:
		return msgIDInfo{name: "bigint", vsID: vsID_JS_BigInt}
	case MsgID_JS_CallImportNamespace:
		return msgIDInfo{name: "call-import-namespace", vsID: vsID_JS_CallImportNamespace}
	case MsgID_JS_ClassNameWillThrow:
		return msgIDInfo{name: "class-name-will-throw", vsID: vsID_JS_ClassNameWillThrow}
	case MsgID_JS_CommonJSVariableInESM:
		return msgIDInfo{name: "commonjs-variable-in-esm", vsID: vsID_JS_CommonJSVariableInESM}
	case MsgID_JS_ConfusingTypeScriptCast:
		return msgIDInfo{name: "confusing-typescript-cast", vsID: vsID_JS_ConfusingTypeScriptCast}
	case MsgID_JS_DeleteSuperProperty:
		return msgIDInfo{name: "delete-super-property", vsID: vsID_JS_DeleteSuperProperty}
	case MsgID_JS_DirectEval:
		return msgIDInfo{name: "direct-eval", vsID: vsID_JS_DirectEval}
	case MsgID_JS_DuplicateCase:
		return msgIDInfo{name: "duplicate-case", vsID: vsID_JS_DuplicateCase}
	case MsgID_JS_DuplicateClassMember:
		return msgIDInfo{name: "duplicate-class-member", vsID: vsID_JS_DuplicateClassMember}
	case MsgID_JS_DuplicateObjectKey:
		return msgIDInfo{name: "duplicate-object-key", vsID: vsID_JS_DuplicateObjectKey}
	case MsgID_JS_EmptyImportMeta:
		return msgIDInfo{name: "empty-import-meta", vsID: vsID_JS_EmptyImportMeta}
	case MsgID_JS_EqualsNaN:
		return msgIDInfo{name: "equals-nan", vsID: vsID_JS_EqualsNaN}
	case MsgID_JS_EqualsNegativeZero:
		return msgIDInfo{name: "equals-negative-zero", vsID: vsID_JS_EqualsNegativeZero}
	case MsgID_JS_EqualsNewObject:
		return msgIDInfo{name: "equals-new-object", vsID: vsID_JS_EqualsNewObject}
	case MsgID_JS_HTMLCommentInJS:
		return msgIDInfo{name: "html-comment-in-js", vsID: vsID_JS_HTMLCommentInJS}
	case MsgID_JS_ImpossibleTypeof:
		return msgIDInfo{name: "impossible-typeof", vsID: vsID_JS_ImpossibleTypeof}
	case MsgID_JS_IndirectRequire:
		return msgIDInfo{name: "indirect-require", vsID: vsID_JS_IndirectRequire}
	case MsgID_JS_PrivateNameWillThrow:
		return msgIDInfo{name: "private-name-will-throw", vsID: vsID_JS_PrivateNameWillThrow}
	case MsgID_JS_SemicolonAfterReturn:
		return msgIDInfo{name: "semicolon-after-return", vsID: vsID_JS_SemicolonAfterReturn}
	case MsgID_JS_SuspiciousBooleanNot:
		return msgIDInfo{name: "suspicious-boolean-not", vsID: vsID_JS_SuspiciousBooleanNot}
	case MsgID_JS_SuspiciousDefine:
		return msgIDInfo{name: "suspicious-define", vsID: vsID_JS_SuspiciousDefine}
	case MsgID_JS_SuspiciousLogicalOperator:
		return msgIDInfo{name: "suspicious-logical-operator", vsID: vsID_JS_SuspiciousLogicalOperator}
	case MsgID_JS_SuspiciousNullishCoalescing:
		return msgIDInfo{name: "suspicious-nullish-coalescing", vsID: vsID_JS_SuspiciousNullishCoalescing}
	case MsgID_JS_ThisIsUndefinedInESM:
		return msgIDInfo{name: "this-is-undefined-in-esm", vsID: vsID_JS_ThisIsUndefinedInESM}
	case MsgID_JS_UnsupportedDynamicImport:
		return msgIDInfo{name: "unsupported-dynamic-import", vsID: vsID_JS_UnsupportedDynamicImport}
	case MsgID_JS_UnsupportedJSXComment:
		return msgIDInfo{name: "unsupported-jsx-comment", vsID: vsID_JS_UnsupportedJSXComment}
	case MsgID_JS_UnsupportedRegExp:
		return msgIDInfo{name: "unsupported-regexp", vsID: vsID_JS_UnsupportedRegExp}
	case MsgID_JS_UnsupportedRequireCall:
		return msgIDInfo{name: "unsupported-require-call", vsID: vsID_JS_UnsupportedRequireCall}

	// CSS
	case MsgID_CSS_CSSSyntaxError:
		return msgIDInfo{name: "css-syntax-error", vsID: vsID_CSS_CSSSyntaxError}
	case MsgID_CSS_InvalidAtCharset:
		return msgIDInfo{name: "invalid-@charset", vsID: vsID_CSS_InvalidAtCharset}
	case MsgID_CSS_InvalidAtImport:
		return msgIDInfo{name: "invalid-@import", vsID: vsID_CSS_InvalidAtImport}
	case MsgID_CSS_InvalidAtLayer:
		return msgIDInfo{name: "invalid-@layer", vsID: vsID_CSS_InvalidAtLayer}
	case MsgID_CSS_InvalidCalc:
		return msgIDInfo{name: "invalid-calc", vsID: vsID_CSS_InvalidCalc}
	case MsgID_CSS_JSCommentInCSS:
		return msgIDInfo{name: "js-comment-in-css", vsID: vsID_CSS_JSCommentInCSS}
	case MsgID_CSS_UndefinedComposesFrom:
		return msgIDInfo{name: "undefined-composes-from", vsID: vsID_CSS_UndefinedComposesFrom}
	case MsgID_CSS_UnsupportedAtCharset:
		return msgIDInfo{name: "unsupported-@charset", vsID: vsID_CSS_UnsupportedAtCharset}
	case MsgID_CSS_UnsupportedAtNamespace:
		return msgIDInfo{name: "unsupported-@namespace", vsID: vsID_CSS_UnsupportedAtNamespace}
	case MsgID_CSS_UnsupportedCSSProperty:
		return msgIDInfo{name: "unsupported-css-property", vsID: vsID_CSS_UnsupportedCSSProperty}
	case MsgID_CSS_UnsupportedCSSNesting:
		return msgIDInfo{name: "unsupported-css-nesting", vsID: vsID_CSS_UnsupportedCSSNesting}

	// Bundler
	case MsgID_Bundler_AmbiguousReexport:
		return msgIDInfo{name: "ambiguous-reexport", vsID: vsID_Bundler_AmbiguousReexport}
	case MsgID_Bundler_DifferentPathCase:
		return msgIDInfo{name: "different-path-case", vsID: vsID_Bundler_DifferentPathCase}
	case MsgID_Bundler_EmptyGlob:
		return msgIDInfo{name: "empty-glob", vsID: vsID_Bundler_EmptyGlob}
	case MsgID_Bundler_IgnoredBareImport:
		return msgIDInfo{name: "ignored-bare-import", vsID: vsID_Bundler_IgnoredBareImport}
	case MsgID_Bundler_IgnoredDynamicImport:
		return msgIDInfo{name: "ignored-dynamic-import", vsID: vsID_Bundler_IgnoredDynamicImport}
	case MsgID_Bundler_ImportIsUndefined:
		return msgIDInfo{name: "import-is-undefined", vsID: vsID_Bundler_ImportIsUndefined}
	case MsgID_Bundler_RequireResolveNotExternal:
		return msgIDInfo{name: "require-resolve-not-external", vsID: vsID_Bundler_RequireResolveNotExternal}

	// Source maps
	case MsgID_SourceMap_InvalidSourceMappings:
		return msgIDInfo{name: "invalid-source-mappings", vsID: vsID_SourceMap_InvalidSourceMappings}
	case MsgID_SourceMap_MissingSourceMap:
		return msgIDInfo{name: "missing-source-map", vsID: vsID_SourceMap_MissingSourceMap}
	case MsgID_SourceMap_UnsupportedSourceMapComment:
		return msgIDInfo{name: "unsupported-source-map-comment", vsID: vsID_SourceMap_UnsupportedSourceMapComment}

	default:
		if id >= MsgID_PackageJSON_FIRST && id <= MsgID_PackageJSON_LAST {
			return msgIDInfo{name: "package.json", vsID: vsID_PackageJSON}
		}
		if id >= MsgID_TSConfigJSON_FIRST && id <= MsgID_TSConfigJSON_LAST {
			return msgIDInfo{name: "tsconfig.json", vsID: vsID_TSConfigJSON}
		}
	}

	return msgIDInfo{}
}

// Some message IDs are more diverse internally than externally (in case we
// want to expand the set of them later on). So just map these to the largest
// one arbitrarily since you can't tell the difference externally anyway.
func StringToMaximumMsgID(id string) MsgID {
	overrides := make(map[MsgID]LogLevel)
	maxID := MsgID_None
	StringToMsgIDs(id, LevelInfo, overrides)
	for id := range overrides {
		if id > maxID {
			maxID = id
		}
	}
	return maxID
}
