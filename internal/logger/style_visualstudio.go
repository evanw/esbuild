package logger

import (
	"fmt"
	"strings"
)

// This log style is intended to help integrate esbuild with Visual Studio's
// built-in problem matcher. More information:
//
// - https://learn.microsoft.com/en-us/cpp/build/formatting-the-output-of-a-custom-build-step-or-build-event
// - https://learn.microsoft.com/en-us/visualstudio/msbuild/msbuild-diagnostic-format-for-tasks
func msgToStringVisualStudio(msg Msg, options OutputOptions, terminalInfo TerminalInfo) string {
	var text strings.Builder

	// Write the origin
	if loc := msg.Data.Location; loc != nil {
		// Potentially this needs to always be an absolute path?
		text.WriteString(loc.File.Abs)

		// Note: Need to adjust the column from 0-based to 1-based
		if loc.Line > 0 {
			text.WriteString(fmt.Sprintf("(%d,%d)", loc.Line, loc.Column+1))
		}
		text.WriteString(": ")
	} else {
		// If there is no file, then we must write a tool name
		text.WriteString("esbuild: ")
	}

	// The code appears to be required, so make something up
	code := fmt.Sprintf("ES%04d", msgIDToInfo(msg.ID).vsID)

	// The only valid options for the category seem to be "error" and "warning"
	if msg.Kind == Error {
		text.WriteString(fmt.Sprintf("error %s: ", code))
	} else {
		text.WriteString(fmt.Sprintf("warning %s: ", code))
	}

	text.WriteString(msg.Data.Text)
	text.WriteByte('\n')
	return text.String()
}

type vsID = uint8

// These are separate from "MsgID" because they are external and "MsgID" is
// internal. Don't ever delete or reorder one of these. Only append to them.
const (
	vsID_JS_AssertToWith                       = 1
	vsID_JS_AssertTypeJSON                     = 2
	vsID_JS_AssignToConstant                   = 3
	vsID_JS_AssignToDefine                     = 4
	vsID_JS_AssignToImport                     = 5
	vsID_JS_BigInt                             = 6
	vsID_JS_CallImportNamespace                = 7
	vsID_JS_ClassNameWillThrow                 = 8
	vsID_JS_CommonJSVariableInESM              = 9
	vsID_JS_ConfusingTypeScriptCast            = 10
	vsID_JS_DeleteSuperProperty                = 11
	vsID_JS_DirectEval                         = 12
	vsID_JS_DuplicateCase                      = 13
	vsID_JS_DuplicateClassMember               = 14
	vsID_JS_DuplicateObjectKey                 = 15
	vsID_JS_EmptyImportMeta                    = 16
	vsID_JS_EqualsNaN                          = 17
	vsID_JS_EqualsNegativeZero                 = 18
	vsID_JS_EqualsNewObject                    = 19
	vsID_JS_HTMLCommentInJS                    = 20
	vsID_JS_ImpossibleTypeof                   = 21
	vsID_JS_IndirectRequire                    = 22
	vsID_JS_PrivateNameWillThrow               = 23
	vsID_JS_SemicolonAfterReturn               = 24
	vsID_JS_SuspiciousBooleanNot               = 25
	vsID_JS_SuspiciousDefine                   = 26
	vsID_JS_SuspiciousLogicalOperator          = 27
	vsID_JS_SuspiciousNullishCoalescing        = 28
	vsID_JS_ThisIsUndefinedInESM               = 29
	vsID_JS_UnsupportedDynamicImport           = 30
	vsID_JS_UnsupportedJSXComment              = 31
	vsID_JS_UnsupportedRegExp                  = 32
	vsID_JS_UnsupportedRequireCall             = 33
	vsID_CSS_CSSSyntaxError                    = 34
	vsID_CSS_InvalidAtCharset                  = 35
	vsID_CSS_InvalidAtImport                   = 36
	vsID_CSS_InvalidAtLayer                    = 37
	vsID_CSS_InvalidCalc                       = 38
	vsID_CSS_JSCommentInCSS                    = 39
	vsID_CSS_UndefinedComposesFrom             = 40
	vsID_CSS_UnsupportedAtCharset              = 41
	vsID_CSS_UnsupportedAtNamespace            = 42
	vsID_CSS_UnsupportedCSSProperty            = 43
	vsID_CSS_UnsupportedCSSNesting             = 44
	vsID_Bundler_AmbiguousReexport             = 45
	vsID_Bundler_DifferentPathCase             = 46
	vsID_Bundler_EmptyGlob                     = 47
	vsID_Bundler_IgnoredBareImport             = 48
	vsID_Bundler_IgnoredDynamicImport          = 49
	vsID_Bundler_ImportIsUndefined             = 50
	vsID_Bundler_RequireResolveNotExternal     = 51
	vsID_SourceMap_InvalidSourceMappings       = 52
	vsID_SourceMap_MissingSourceMap            = 53
	vsID_SourceMap_UnsupportedSourceMapComment = 54
	vsID_PackageJSON                           = 55
	vsID_TSConfigJSON                          = 56
)
