package js_parser

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/runtime"
)

// This parser does two passes:
//
// 1. Parse the source into an AST, create the scope tree, and declare symbols.
//
//  2. Visit each node in the AST, bind identifiers to declared symbols, do
//     constant folding, substitute compile-time variable definitions, and
//     lower certain syntactic constructs as appropriate given the language
//     target.
//
// So many things have been put in so few passes because we want to minimize
// the number of full-tree passes to improve performance. However, we need
// to have at least two separate passes to handle variable hoisting. See the
// comment about scopesInOrder below for more information.
type parser struct {
	options                    Options
	log                        logger.Log
	source                     logger.Source
	tracker                    logger.LineColumnTracker
	fnOrArrowDataParse         fnOrArrowDataParse
	fnOnlyDataVisit            fnOnlyDataVisit
	allocatedNames             []string
	currentScope               *js_ast.Scope
	scopesForCurrentPart       []*js_ast.Scope
	symbols                    []ast.Symbol
	astHelpers                 js_ast.HelperContext
	tsUseCounts                []uint32
	injectedDefineSymbols      []ast.Ref
	injectedSymbolSources      map[ast.Ref]injectedSymbolSource
	injectedDotNames           map[string][]injectedDotName
	dropLabelsMap              map[string]struct{}
	exprComments               map[logger.Loc][]string
	mangledProps               map[string]ast.Ref
	reservedProps              map[string]bool
	symbolUses                 map[ast.Ref]js_ast.SymbolUse
	importSymbolPropertyUses   map[ast.Ref]map[string]js_ast.SymbolUse
	symbolCallUses             map[ast.Ref]js_ast.SymbolCallUse
	declaredSymbols            []js_ast.DeclaredSymbol
	globPatternImports         []globPatternImport
	runtimeImports             map[string]ast.LocRef
	duplicateCaseChecker       duplicateCaseChecker
	unrepresentableIdentifiers map[string]bool
	legacyOctalLiterals        map[js_ast.E]logger.Range
	scopesInOrderForEnum       map[logger.Loc][]scopeOrder
	binaryExprStack            []binaryExprVisitor

	// For strict mode handling
	hoistedRefForSloppyModeBlockFn map[ast.Ref]ast.Ref

	// For lowering private methods
	privateGetters map[ast.Ref]ast.Ref
	privateSetters map[ast.Ref]ast.Ref

	// These are for TypeScript
	//
	// We build up enough information about the TypeScript namespace hierarchy to
	// be able to resolve scope lookups and property accesses for TypeScript enum
	// and namespace features. Each JavaScript scope object inside a namespace
	// has a reference to a map of exported namespace members from sibling scopes.
	//
	// In addition, there is a map from each relevant symbol reference to the data
	// associated with that namespace or namespace member: "refToTSNamespaceMemberData".
	// This gives enough info to be able to resolve queries into the namespace.
	//
	// When visiting expressions, namespace metadata is associated with the most
	// recently visited node. If namespace metadata is present, "tsNamespaceTarget"
	// will be set to the most recently visited node (as a way to mark that this
	// node has metadata) and "tsNamespaceMemberData" will be set to the metadata.
	refToTSNamespaceMemberData map[ast.Ref]js_ast.TSNamespaceMemberData
	tsNamespaceTarget          js_ast.E
	tsNamespaceMemberData      js_ast.TSNamespaceMemberData
	emittedNamespaceVars       map[ast.Ref]bool
	isExportedInsideNamespace  map[ast.Ref]ast.Ref
	localTypeNames             map[string]bool
	tsEnums                    map[ast.Ref]map[string]js_ast.TSEnumValue
	constValues                map[ast.Ref]js_ast.ConstValue
	propDerivedCtorValue       js_ast.E
	propMethodDecoratorScope   *js_ast.Scope

	// This is the reference to the generated function argument for the namespace,
	// which is different than the reference to the namespace itself:
	//
	//   namespace ns {
	//   }
	//
	// The code above is transformed into something like this:
	//
	//   var ns1;
	//   (function(ns2) {
	//   })(ns1 || (ns1 = {}));
	//
	// This variable is "ns2" not "ns1". It is only used during the second
	// "visit" pass.
	enclosingNamespaceArgRef *ast.Ref

	// Imports (both ES6 and CommonJS) are tracked at the top level
	importRecords               []ast.ImportRecord
	importRecordsForCurrentPart []uint32
	exportStarImportRecords     []uint32

	// These are for handling ES6 imports and exports
	importItemsForNamespace map[ast.Ref]namespaceImportItems
	isImportItem            map[ast.Ref]bool
	namedImports            map[ast.Ref]js_ast.NamedImport
	namedExports            map[string]js_ast.NamedExport
	topLevelSymbolToParts   map[ast.Ref][]uint32
	importNamespaceCCMap    map[importNamespaceCall]bool

	// The parser does two passes and we need to pass the scope tree information
	// from the first pass to the second pass. That's done by tracking the calls
	// to pushScopeForParsePass() and popScope() during the first pass in
	// scopesInOrder.
	//
	// Then, when the second pass calls pushScopeForVisitPass() and popScope(),
	// we consume entries from scopesInOrder and make sure they are in the same
	// order. This way the second pass can efficiently use the same scope tree
	// as the first pass without having to attach the scope tree to the AST.
	//
	// We need to split this into two passes because the pass that declares the
	// symbols must be separate from the pass that binds identifiers to declared
	// symbols to handle declaring a hoisted "var" symbol in a nested scope and
	// binding a name to it in a parent or sibling scope.
	scopesInOrder []scopeOrder

	// These propagate the name from the parent context into an anonymous child
	// expression. For example:
	//
	//   let foo = function() {}
	//   assert.strictEqual(foo.name, 'foo')
	//
	nameToKeep      string
	nameToKeepIsFor js_ast.E

	// These properties are for the visit pass, which runs after the parse pass.
	// The visit pass binds identifiers to declared symbols, does constant
	// folding, substitutes compile-time variable definitions, and lowers certain
	// syntactic constructs as appropriate.
	stmtExprValue                        js_ast.E
	callTarget                           js_ast.E
	dotOrIndexTarget                     js_ast.E
	templateTag                          js_ast.E
	deleteTarget                         js_ast.E
	loopBody                             js_ast.S
	suspiciousLogicalOperatorInsideArrow js_ast.E
	moduleScope                          *js_ast.Scope

	// This is internal-only data used for the implementation of Yarn PnP
	manifestForYarnPnP     js_ast.Expr
	stringLocalsForYarnPnP map[ast.Ref]stringLocalForYarnPnP

	// This helps recognize the "await import()" pattern. When this is present,
	// warnings about non-string import paths will be omitted inside try blocks.
	awaitTarget js_ast.E

	// This helps recognize the "import().catch()" pattern. We also try to avoid
	// warning about this just like the "try { await import() }" pattern.
	thenCatchChain thenCatchChain

	// When bundling, hoisted top-level local variables declared with "var" in
	// nested scopes are moved up to be declared in the top-level scope instead.
	// The old "var" statements are turned into regular assignments instead. This
	// makes it easier to quickly scan the top-level statements for "var" locals
	// with the guarantee that all will be found.
	relocatedTopLevelVars []ast.LocRef

	// We need to lower private names such as "#foo" if they are used in a brand
	// check such as "#foo in x" even if the private name syntax would otherwise
	// be supported. This is because private names are a newly-added feature.
	//
	// However, this parser operates in only two passes for speed. The first pass
	// parses things and declares variables, and the second pass lowers things and
	// resolves references to declared variables. So the existence of a "#foo in x"
	// expression for a specific "#foo" cannot be used to decide to lower "#foo"
	// because it's too late by that point. There may be another expression such
	// as "x.#foo" before that point and that must be lowered as well even though
	// it has already been visited.
	//
	// Instead what we do is track just the names of fields used in private brand
	// checks during the first pass. This tracks the names themselves, not symbol
	// references. Then, during the second pass when we are about to enter into
	// a class, we conservatively decide to lower all private names in that class
	// which are used in a brand check anywhere in the file.
	lowerAllOfThesePrivateNames map[string]bool

	// Temporary variables used for lowering
	tempLetsToDeclare         []ast.Ref
	tempRefsToDeclare         []tempRef
	topLevelTempRefsToDeclare []tempRef

	lexer js_lexer.Lexer

	// Private field access in a decorator lowers all private fields in that class
	parseExperimentalDecoratorNesting int

	// Temporary variables used for lowering
	tempRefCount         int
	topLevelTempRefCount int

	// We need to scan over the source contents to recover the line and column offsets
	jsxSourceLoc    int
	jsxSourceLine   int
	jsxSourceColumn int

	exportsRef    ast.Ref
	requireRef    ast.Ref
	moduleRef     ast.Ref
	importMetaRef ast.Ref
	promiseRef    ast.Ref
	regExpRef     ast.Ref
	bigIntRef     ast.Ref
	superCtorRef  ast.Ref

	// Imports from "react/jsx-runtime" and "react", respectively.
	// (Or whatever was specified in the "importSource" option)
	jsxRuntimeImports map[string]ast.LocRef
	jsxLegacyImports  map[string]ast.LocRef

	// For lowering private methods
	weakMapRef ast.Ref
	weakSetRef ast.Ref

	esmImportStatementKeyword logger.Range
	esmImportMeta             logger.Range
	esmExportKeyword          logger.Range
	enclosingClassKeyword     logger.Range
	topLevelAwaitKeyword      logger.Range
	liveTopLevelAwaitKeyword  logger.Range

	latestArrowArgLoc      logger.Loc
	forbidSuffixAfterAsLoc logger.Loc
	firstJSXElementLoc     logger.Loc

	fnOrArrowDataVisit fnOrArrowDataVisit

	// ArrowFunction is a special case in the grammar. Although it appears to be
	// a PrimaryExpression, it's actually an AssignmentExpression. This means if
	// a AssignmentExpression ends up producing an ArrowFunction then nothing can
	// come after it other than the comma operator, since the comma operator is
	// the only thing above AssignmentExpression under the Expression rule:
	//
	//   AssignmentExpression:
	//     ArrowFunction
	//     ConditionalExpression
	//     LeftHandSideExpression = AssignmentExpression
	//     LeftHandSideExpression AssignmentOperator AssignmentExpression
	//
	//   Expression:
	//     AssignmentExpression
	//     Expression , AssignmentExpression
	//
	afterArrowBodyLoc logger.Loc

	// Setting this to true disables warnings about code that is very likely to
	// be a bug. This is used to ignore issues inside "node_modules" directories.
	// This has caught real issues in the past. However, it's not esbuild's job
	// to find bugs in other libraries, and these warnings are problematic for
	// people using these libraries with esbuild. The only fix is to either
	// disable all esbuild warnings and not get warnings about your own code, or
	// to try to get the warning fixed in the affected library. This is
	// especially annoying if the warning is a false positive as was the case in
	// https://github.com/firebase/firebase-js-sdk/issues/3814. So these warnings
	// are now disabled for code inside "node_modules" directories.
	suppressWarningsAboutWeirdCode bool

	// A file is considered to be an ECMAScript module if it has any of the
	// features of one (e.g. the "export" keyword), otherwise it's considered
	// a CommonJS module.
	//
	// However, we have a single exception: a file where the only ESM feature
	// is the "import" keyword is allowed to have CommonJS exports. This feature
	// is necessary to be able to synchronously import ESM code into CommonJS,
	// which we need to enable in a few important cases. Some examples are:
	// our runtime code, injected files (the "inject" feature is ESM-only),
	// and certain automatically-generated virtual modules from plugins.
	isFileConsideredToHaveESMExports bool // Use only for export-related stuff
	isFileConsideredESM              bool // Use for all other stuff

	// Inside a TypeScript namespace, an "export declare" statement can be used
	// to cause a namespace to be emitted even though it has no other observable
	// effect. This flag is used to implement this feature.
	//
	// Specifically, namespaces should be generated for all of the following
	// namespaces below except for "f", which should not be generated:
	//
	//   namespace a { export declare const a }
	//   namespace b { export declare let [[b]] }
	//   namespace c { export declare function c() }
	//   namespace d { export declare class d {} }
	//   namespace e { export declare enum e {} }
	//   namespace f { export declare namespace f {} }
	//
	// The TypeScript compiler compiles this into the following code (notice "f"
	// is missing):
	//
	//   var a; (function (a_1) {})(a || (a = {}));
	//   var b; (function (b_1) {})(b || (b = {}));
	//   var c; (function (c_1) {})(c || (c = {}));
	//   var d; (function (d_1) {})(d || (d = {}));
	//   var e; (function (e_1) {})(e || (e = {}));
	//
	// Note that this should not be implemented by declaring symbols for "export
	// declare" statements because the TypeScript compiler doesn't generate any
	// code for these statements, so these statements are actually references to
	// global variables. There is one exception, which is that local variables
	// *should* be declared as symbols because they are replaced with. This seems
	// like very arbitrary behavior but it's what the TypeScript compiler does,
	// so we try to match it.
	//
	// Specifically, in the following code below "a" and "b" should be declared
	// and should be substituted with "ns.a" and "ns.b" but the other symbols
	// shouldn't. References to the other symbols actually refer to global
	// variables instead of to symbols that are exported from the namespace.
	// This is the case as of TypeScript 4.3. I assume this is a TypeScript bug:
	//
	//   namespace ns {
	//     export declare const a
	//     export declare let [[b]]
	//     export declare function c()
	//     export declare class d { }
	//     export declare enum e { }
	//     console.log(a, b, c, d, e)
	//   }
	//
	// The TypeScript compiler compiles this into the following code:
	//
	//   var ns;
	//   (function (ns) {
	//       console.log(ns.a, ns.b, c, d, e);
	//   })(ns || (ns = {}));
	//
	// Relevant issue: https://github.com/evanw/esbuild/issues/1158
	hasNonLocalExportDeclareInsideNamespace bool

	// When this flag is enabled, we attempt to fold all expressions that
	// TypeScript would consider to be "constant expressions". This flag is
	// enabled inside each enum body block since TypeScript requires numeric
	// constant folding in enum definitions.
	//
	// We also enable this flag in certain cases in JavaScript files such as when
	// parsing "const" declarations at the top of a non-ESM file, but we still
	// reuse TypeScript's notion of "constant expressions" for our own convenience.
	//
	// As of TypeScript 5.0, a "constant expression" is defined as follows:
	//
	//   An expression is considered a constant expression if it is
	//
	//   * a number or string literal,
	//   * a unary +, -, or ~ applied to a numeric constant expression,
	//   * a binary +, -, *, /, %, **, <<, >>, >>>, |, &, ^ applied to two numeric constant expressions,
	//   * a binary + applied to two constant expressions whereof at least one is a string,
	//   * a template expression where each substitution expression is a constant expression,
	//   * a parenthesized constant expression,
	//   * a dotted name (e.g. x.y.z) that references a const variable with a constant expression initializer and no type annotation,
	//   * a dotted name that references an enum member with an enum literal type, or
	//   * a dotted name indexed by a string literal (e.g. x.y["z"]) that references an enum member with an enum literal type.
	//
	// More detail: https://github.com/microsoft/TypeScript/pull/50528. Note that
	// we don't implement certain items in this list. For example, we don't do all
	// number-to-string conversions since ours might differ from how JavaScript
	// would do it, which would be a correctness issue.
	shouldFoldTypeScriptConstantExpressions bool

	allowIn                     bool
	allowPrivateIdentifiers     bool
	hasTopLevelReturn           bool
	latestReturnHadSemicolon    bool
	messageAboutThisIsUndefined bool
	isControlFlowDead           bool
	shouldAddKeyComment         bool

	// If this is true, then all top-level statements are wrapped in a try/catch
	willWrapModuleInTryCatchForUsing bool
}

type globPatternImport struct {
	assertOrWith     *ast.ImportAssertOrWith
	parts            []helpers.GlobPart
	name             string
	approximateRange logger.Range
	ref              ast.Ref
	kind             ast.ImportKind
}

type namespaceImportItems struct {
	entries           map[string]ast.LocRef
	importRecordIndex uint32
}

type stringLocalForYarnPnP struct {
	value []uint16
	loc   logger.Loc
}

type injectedSymbolSource struct {
	source logger.Source
	loc    logger.Loc
}

type injectedDotName struct {
	parts               []string
	injectedDefineIndex uint32
}

type importNamespaceCallKind uint8

const (
	exprKindCall importNamespaceCallKind = iota
	exprKindNew
	exprKindJSXTag
)

type importNamespaceCall struct {
	ref  ast.Ref
	kind importNamespaceCallKind
}

type thenCatchChain struct {
	nextTarget      js_ast.E
	catchLoc        logger.Loc
	hasMultipleArgs bool
	hasCatch        bool
}

// This is used as part of an incremental build cache key. Some of these values
// can potentially change between builds if they are derived from nearby
// "package.json" or "tsconfig.json" files that were changed since the last
// build.
type Options struct {
	injectedFiles  []config.InjectedFile
	jsx            config.JSXOptions
	tsAlwaysStrict *config.TSAlwaysStrict
	mangleProps    *regexp.Regexp
	reserveProps   *regexp.Regexp
	dropLabels     []string

	// This pointer will always be different for each build but the contents
	// shouldn't ever behave different semantically. We ignore this field for the
	// equality comparison.
	defines *config.ProcessedDefines

	// This is an embedded struct. Always access these directly instead of off
	// the name "optionsThatSupportStructuralEquality". This is only grouped like
	// this to make the equality comparison easier and safer (and hopefully faster).
	optionsThatSupportStructuralEquality
}

type optionsThatSupportStructuralEquality struct {
	originalTargetEnv                 string
	moduleTypeData                    js_ast.ModuleTypeData
	unsupportedJSFeatures             compat.JSFeature
	unsupportedJSFeatureOverrides     compat.JSFeature
	unsupportedJSFeatureOverridesMask compat.JSFeature

	// Byte-sized values go here (gathered together here to keep this object compact)
	ts                     config.TSOptions
	mode                   config.Mode
	platform               config.Platform
	outputFormat           config.Format
	asciiOnly              bool
	keepNames              bool
	minifySyntax           bool
	minifyIdentifiers      bool
	minifyWhitespace       bool
	omitRuntimeForTests    bool
	omitJSXRuntimeForTests bool
	ignoreDCEAnnotations   bool
	treeShaking            bool
	dropDebugger           bool
	mangleQuoted           bool

	// This is an internal-only option used for the implementation of Yarn PnP
	decodeHydrateRuntimeStateYarnPnP bool
}

func OptionsForYarnPnP() Options {
	return Options{
		optionsThatSupportStructuralEquality: optionsThatSupportStructuralEquality{
			decodeHydrateRuntimeStateYarnPnP: true,
		},
	}
}

func OptionsFromConfig(options *config.Options) Options {
	return Options{
		injectedFiles:  options.InjectedFiles,
		jsx:            options.JSX,
		defines:        options.Defines,
		tsAlwaysStrict: options.TSAlwaysStrict,
		mangleProps:    options.MangleProps,
		reserveProps:   options.ReserveProps,
		dropLabels:     options.DropLabels,

		optionsThatSupportStructuralEquality: optionsThatSupportStructuralEquality{
			unsupportedJSFeatures:             options.UnsupportedJSFeatures,
			unsupportedJSFeatureOverrides:     options.UnsupportedJSFeatureOverrides,
			unsupportedJSFeatureOverridesMask: options.UnsupportedJSFeatureOverridesMask,
			originalTargetEnv:                 options.OriginalTargetEnv,
			ts:                                options.TS,
			mode:                              options.Mode,
			platform:                          options.Platform,
			outputFormat:                      options.OutputFormat,
			moduleTypeData:                    options.ModuleTypeData,
			asciiOnly:                         options.ASCIIOnly,
			keepNames:                         options.KeepNames,
			minifySyntax:                      options.MinifySyntax,
			minifyIdentifiers:                 options.MinifyIdentifiers,
			minifyWhitespace:                  options.MinifyWhitespace,
			omitRuntimeForTests:               options.OmitRuntimeForTests,
			omitJSXRuntimeForTests:            options.OmitJSXRuntimeForTests,
			ignoreDCEAnnotations:              options.IgnoreDCEAnnotations,
			treeShaking:                       options.TreeShaking,
			dropDebugger:                      options.DropDebugger,
			mangleQuoted:                      options.MangleQuoted,
		},
	}
}

func (a *Options) Equal(b *Options) bool {
	// Compare "optionsThatSupportStructuralEquality"
	if a.optionsThatSupportStructuralEquality != b.optionsThatSupportStructuralEquality {
		return false
	}

	// Compare "tsAlwaysStrict"
	if (a.tsAlwaysStrict == nil && b.tsAlwaysStrict != nil) || (a.tsAlwaysStrict != nil && b.tsAlwaysStrict == nil) ||
		(a.tsAlwaysStrict != nil && b.tsAlwaysStrict != nil && *a.tsAlwaysStrict != *b.tsAlwaysStrict) {
		return false
	}

	// Compare "mangleProps" and "reserveProps"
	if !isSameRegexp(a.mangleProps, b.mangleProps) || !isSameRegexp(a.reserveProps, b.reserveProps) {
		return false
	}

	// Compare "dropLabels"
	if !helpers.StringArraysEqual(a.dropLabels, b.dropLabels) {
		return false
	}

	// Compare "injectedFiles"
	if len(a.injectedFiles) != len(b.injectedFiles) {
		return false
	}
	for i, x := range a.injectedFiles {
		y := b.injectedFiles[i]
		if x.Source != y.Source || x.DefineName != y.DefineName || len(x.Exports) != len(y.Exports) {
			return false
		}
		for j := range x.Exports {
			if x.Exports[j] != y.Exports[j] {
				return false
			}
		}
	}

	// Compare "jsx"
	if a.jsx.Parse != b.jsx.Parse || !jsxExprsEqual(a.jsx.Factory, b.jsx.Factory) || !jsxExprsEqual(a.jsx.Fragment, b.jsx.Fragment) {
		return false
	}

	// Do a cheap assert that the defines object hasn't changed
	if (a.defines != nil || b.defines != nil) && (a.defines == nil || b.defines == nil ||
		len(a.defines.IdentifierDefines) != len(b.defines.IdentifierDefines) ||
		len(a.defines.DotDefines) != len(b.defines.DotDefines)) {
		panic("Internal error")
	}

	return true
}

func isSameRegexp(a *regexp.Regexp, b *regexp.Regexp) bool {
	if a == nil {
		return b == nil
	} else {
		return b != nil && a.String() == b.String()
	}
}

func jsxExprsEqual(a config.DefineExpr, b config.DefineExpr) bool {
	if !helpers.StringArraysEqual(a.Parts, b.Parts) {
		return false
	}

	if a.Constant != nil {
		if b.Constant == nil || !js_ast.ValuesLookTheSame(a.Constant, b.Constant) {
			return false
		}
	} else if b.Constant != nil {
		return false
	}

	return true
}

type tempRef struct {
	valueOrNil js_ast.Expr
	ref        ast.Ref
}

const (
	locModuleScope = -1
)

type scopeOrder struct {
	scope *js_ast.Scope
	loc   logger.Loc
}

type awaitOrYield uint8

const (
	// The keyword is used as an identifier, not a special expression
	allowIdent awaitOrYield = iota

	// Declaring the identifier is forbidden, and the keyword is used as a special expression
	allowExpr

	// Declaring the identifier is forbidden, and using the identifier is also forbidden
	forbidAll
)

// This is function-specific information used during parsing. It is saved and
// restored on the call stack around code that parses nested functions and
// arrow expressions.
type fnOrArrowDataParse struct {
	arrowArgErrors      *deferredArrowArgErrors
	decoratorScope      *js_ast.Scope
	asyncRange          logger.Range
	needsAsyncLoc       logger.Loc
	await               awaitOrYield
	yield               awaitOrYield
	allowSuperCall      bool
	allowSuperProperty  bool
	isTopLevel          bool
	isConstructor       bool
	isTypeScriptDeclare bool
	isThisDisallowed    bool
	isReturnDisallowed  bool

	// In TypeScript, forward declarations of functions have no bodies
	allowMissingBodyForTypeScript bool
}

// This is function-specific information used during visiting. It is saved and
// restored on the call stack around code that parses nested functions and
// arrow expressions.
type fnOrArrowDataVisit struct {
	// This is used to silence unresolvable imports due to "require" calls inside
	// a try/catch statement. The assumption is that the try/catch statement is
	// there to handle the case where the reference to "require" crashes.
	tryBodyCount int32
	tryCatchLoc  logger.Loc

	isArrow                        bool
	isAsync                        bool
	isGenerator                    bool
	isInsideLoop                   bool
	isInsideSwitch                 bool
	isDerivedClassCtor             bool
	isOutsideFnOrArrow             bool
	shouldLowerSuperPropertyAccess bool
}

// This is function-specific information used during visiting. It is saved and
// restored on the call stack around code that parses nested functions (but not
// nested arrow functions).
type fnOnlyDataVisit struct {
	// This is a reference to the magic "arguments" variable that exists inside
	// functions in JavaScript. It will be non-nil inside functions and nil
	// otherwise.
	argumentsRef *ast.Ref

	// Arrow functions don't capture the value of "this" and "arguments". Instead,
	// the values are inherited from the surrounding context. If arrow functions
	// are turned into regular functions due to lowering, we will need to generate
	// local variables to capture these values so they are preserved correctly.
	thisCaptureRef      *ast.Ref
	argumentsCaptureRef *ast.Ref

	// If true, we're inside a static class context where "this" expressions
	// should be replaced with the class name.
	shouldReplaceThisWithInnerClassNameRef bool

	// This is true if "this" is equal to the class name. It's true if we're in a
	// static class field initializer, a static class method, or a static class
	// block.
	isInStaticClassContext bool

	// This is a reference to the enclosing class name if there is one. It's used
	// to implement "this" and "super" references. A name is automatically generated
	// if one is missing so this will always be present inside a class body.
	innerClassNameRef *ast.Ref

	// If we're inside an async arrow function and async functions are not
	// supported, then we will have to convert that arrow function to a generator
	// function. That means references to "arguments" inside the arrow function
	// will have to reference a captured variable instead of the real variable.
	isInsideAsyncArrowFn bool

	// If false, disallow "new.target" expressions. We disallow all "new.target"
	// expressions at the top-level of the file (i.e. not inside a function or
	// a class field). Technically since CommonJS files are wrapped in a function
	// you can use "new.target" in node as an alias for "undefined" but we don't
	// support that.
	isNewTargetAllowed bool

	// If false, the value for "this" is the top-level module scope "this" value.
	// That means it's "undefined" for ECMAScript modules and "exports" for
	// CommonJS modules. We track this information so that we can substitute the
	// correct value for these top-level "this" references at compile time instead
	// of passing the "this" expression through to the output and leaving the
	// interpretation up to the run-time behavior of the generated code.
	//
	// If true, the value for "this" is nested inside something (either a function
	// or a class declaration). That means the top-level module scope "this" value
	// has been shadowed and is now inaccessible.
	isThisNested bool

	// Do not warn about "this" being undefined for code that the TypeScript
	// compiler generates that looks like this:
	//
	//   var __rest = (this && this.__rest) || function (s, e) {
	//     ...
	//   };
	//
	silenceMessageAboutThisBeingUndefined bool
}

type livenessStatus int8

const (
	alwaysDead      livenessStatus = -1
	livenessUnknown livenessStatus = 0
	alwaysLive      livenessStatus = 1
)

type switchCaseLiveness struct {
	status         livenessStatus
	canFallThrough bool
}

func analyzeSwitchCasesForLiveness(s *js_ast.SSwitch) []switchCaseLiveness {
	cases := make([]switchCaseLiveness, 0, len(s.Cases))
	defaultIndex := -1

	// Determine the status of the individual cases independently
	maxStatus := alwaysDead
	for i, c := range s.Cases {
		if c.ValueOrNil.Data == nil {
			defaultIndex = i
		}

		// Check the value for strict equality
		var status livenessStatus
		if maxStatus == alwaysLive {
			status = alwaysDead // Everything after an always-live case is always dead
		} else if c.ValueOrNil.Data == nil {
			status = alwaysDead // This is the default case, and will be filled in later
		} else if isEqualToTest, ok := js_ast.CheckEqualityIfNoSideEffects(s.Test.Data, c.ValueOrNil.Data, js_ast.StrictEquality); ok {
			if isEqualToTest {
				status = alwaysLive // This branch will always be matched, and will be taken unless an earlier branch was taken
			} else {
				status = alwaysDead // This branch will never be matched, and will not be taken unless there was fall-through
			}
		} else {
			status = livenessUnknown // This branch depends on run-time values and may or may not be matched
		}
		if maxStatus < status {
			maxStatus = status
		}

		// Check for potential fall-through by checking for a jump at the end of the body
		canFallThrough := true
		stmts := c.Body
		for len(stmts) > 0 {
			switch s := stmts[len(stmts)-1].Data.(type) {
			case *js_ast.SBlock:
				stmts = s.Stmts // If this ends with a block, check the block's body next
				continue
			case *js_ast.SBreak, *js_ast.SContinue, *js_ast.SReturn, *js_ast.SThrow:
				canFallThrough = false
			}
			break
		}

		cases = append(cases, switchCaseLiveness{
			status:         status,
			canFallThrough: canFallThrough,
		})
	}

	// Set the liveness for the default case last based on the other cases
	if defaultIndex != -1 {
		// The negation here transposes "always live" with "always dead"
		cases[defaultIndex].status = -maxStatus
	}

	// Then propagate fall-through information in linear fall-through order
	for i, c := range cases {
		// Propagate state forward if this isn't dead. Note that the "can fall
		// through" flag does not imply "must fall through". The body may have
		// an embedded "break" inside an if statement, for example.
		if c.status != alwaysDead {
			for j := i + 1; j < len(cases) && cases[j-1].canFallThrough; j++ {
				cases[j].status = livenessUnknown
			}
		}
	}
	return cases
}

const bloomFilterSize = 251

type duplicateCaseValue struct {
	value js_ast.Expr
	hash  uint32
}

type duplicateCaseChecker struct {
	cases       []duplicateCaseValue
	bloomFilter [(bloomFilterSize + 7) / 8]byte
}

func (dc *duplicateCaseChecker) reset() {
	// Preserve capacity
	dc.cases = dc.cases[:0]

	// This should be optimized by the compiler. See this for more information:
	// https://github.com/golang/go/issues/5373
	bytes := dc.bloomFilter
	for i := range bytes {
		bytes[i] = 0
	}
}

func (dc *duplicateCaseChecker) check(p *parser, expr js_ast.Expr) {
	if hash, ok := duplicateCaseHash(expr); ok {
		bucket := hash % bloomFilterSize
		entry := &dc.bloomFilter[bucket/8]
		mask := byte(1) << (bucket % 8)

		// Check for collisions
		if (*entry & mask) != 0 {
			for _, c := range dc.cases {
				if c.hash == hash {
					if equals, couldBeIncorrect := duplicateCaseEquals(c.value, expr); equals {
						var laterRange logger.Range
						var earlierRange logger.Range
						if _, ok := expr.Data.(*js_ast.EString); ok {
							laterRange = p.source.RangeOfString(expr.Loc)
						} else {
							laterRange = p.source.RangeOfOperatorBefore(expr.Loc, "case")
						}
						if _, ok := c.value.Data.(*js_ast.EString); ok {
							earlierRange = p.source.RangeOfString(c.value.Loc)
						} else {
							earlierRange = p.source.RangeOfOperatorBefore(c.value.Loc, "case")
						}
						text := "This case clause will never be evaluated because it duplicates an earlier case clause"
						if couldBeIncorrect {
							text = "This case clause may never be evaluated because it likely duplicates an earlier case clause"
						}
						kind := logger.Warning
						if p.suppressWarningsAboutWeirdCode {
							kind = logger.Debug
						}
						p.log.AddIDWithNotes(logger.MsgID_JS_DuplicateCase, kind, &p.tracker, laterRange, text,
							[]logger.MsgData{p.tracker.MsgData(earlierRange, "The earlier case clause is here:")})
					}
					return
				}
			}
		}

		*entry |= mask
		dc.cases = append(dc.cases, duplicateCaseValue{hash: hash, value: expr})
	}
}

func duplicateCaseHash(expr js_ast.Expr) (uint32, bool) {
	switch e := expr.Data.(type) {
	case *js_ast.EInlinedEnum:
		return duplicateCaseHash(e.Value)

	case *js_ast.ENull:
		return 0, true

	case *js_ast.EUndefined:
		return 1, true

	case *js_ast.EBoolean:
		if e.Value {
			return helpers.HashCombine(2, 1), true
		}
		return helpers.HashCombine(2, 0), true

	case *js_ast.ENumber:
		bits := math.Float64bits(e.Value)
		return helpers.HashCombine(helpers.HashCombine(3, uint32(bits)), uint32(bits>>32)), true

	case *js_ast.EString:
		hash := uint32(4)
		for _, c := range e.Value {
			hash = helpers.HashCombine(hash, uint32(c))
		}
		return hash, true

	case *js_ast.EBigInt:
		hash := uint32(5)
		for _, c := range e.Value {
			hash = helpers.HashCombine(hash, uint32(c))
		}
		return hash, true

	case *js_ast.EIdentifier:
		return helpers.HashCombine(6, e.Ref.InnerIndex), true

	case *js_ast.EDot:
		if target, ok := duplicateCaseHash(e.Target); ok {
			return helpers.HashCombineString(helpers.HashCombine(7, target), e.Name), true
		}

	case *js_ast.EIndex:
		if target, ok := duplicateCaseHash(e.Target); ok {
			if index, ok := duplicateCaseHash(e.Index); ok {
				return helpers.HashCombine(helpers.HashCombine(8, target), index), true
			}
		}
	}

	return 0, false
}

func duplicateCaseEquals(left js_ast.Expr, right js_ast.Expr) (equals bool, couldBeIncorrect bool) {
	if b, ok := right.Data.(*js_ast.EInlinedEnum); ok {
		return duplicateCaseEquals(left, b.Value)
	}

	switch a := left.Data.(type) {
	case *js_ast.EInlinedEnum:
		return duplicateCaseEquals(a.Value, right)

	case *js_ast.ENull:
		_, ok := right.Data.(*js_ast.ENull)
		return ok, false

	case *js_ast.EUndefined:
		_, ok := right.Data.(*js_ast.EUndefined)
		return ok, false

	case *js_ast.EBoolean:
		b, ok := right.Data.(*js_ast.EBoolean)
		return ok && a.Value == b.Value, false

	case *js_ast.ENumber:
		b, ok := right.Data.(*js_ast.ENumber)
		return ok && a.Value == b.Value, false

	case *js_ast.EString:
		b, ok := right.Data.(*js_ast.EString)
		return ok && helpers.UTF16EqualsUTF16(a.Value, b.Value), false

	case *js_ast.EBigInt:
		if b, ok := right.Data.(*js_ast.EBigInt); ok {
			equal, ok := js_ast.CheckEqualityBigInt(a.Value, b.Value)
			return ok && equal, false
		}

	case *js_ast.EIdentifier:
		b, ok := right.Data.(*js_ast.EIdentifier)
		return ok && a.Ref == b.Ref, false

	case *js_ast.EDot:
		if b, ok := right.Data.(*js_ast.EDot); ok && a.OptionalChain == b.OptionalChain && a.Name == b.Name {
			equals, _ := duplicateCaseEquals(a.Target, b.Target)
			return equals, true
		}

	case *js_ast.EIndex:
		if b, ok := right.Data.(*js_ast.EIndex); ok && a.OptionalChain == b.OptionalChain {
			if equals, _ := duplicateCaseEquals(a.Index, b.Index); equals {
				equals, _ := duplicateCaseEquals(a.Target, b.Target)
				return equals, true
			}
		}
	}

	return false, false
}

type duplicatePropertiesIn uint8

const (
	duplicatePropertiesInObject duplicatePropertiesIn = iota
	duplicatePropertiesInClass
)

func (p *parser) warnAboutDuplicateProperties(properties []js_ast.Property, in duplicatePropertiesIn) {
	if len(properties) < 2 {
		return
	}

	type keyKind uint8
	type existingKey struct {
		loc  logger.Loc
		kind keyKind
	}
	const (
		keyMissing keyKind = iota
		keyNormal
		keyGet
		keySet
		keyGetAndSet
	)
	instanceKeys := make(map[string]existingKey)
	staticKeys := make(map[string]existingKey)

	for _, property := range properties {
		if property.Kind != js_ast.PropertySpread {
			if str, ok := property.Key.Data.(*js_ast.EString); ok {
				var keys map[string]existingKey
				if property.Flags.Has(js_ast.PropertyIsStatic) {
					keys = staticKeys
				} else {
					keys = instanceKeys
				}
				key := helpers.UTF16ToString(str.Value)
				prevKey := keys[key]
				nextKey := existingKey{kind: keyNormal, loc: property.Key.Loc}

				if property.Kind == js_ast.PropertyGetter {
					nextKey.kind = keyGet
				} else if property.Kind == js_ast.PropertySetter {
					nextKey.kind = keySet
				}

				if prevKey.kind != keyMissing && (in != duplicatePropertiesInObject || key != "__proto__") && (in != duplicatePropertiesInClass || key != "constructor") {
					if (prevKey.kind == keyGet && nextKey.kind == keySet) || (prevKey.kind == keySet && nextKey.kind == keyGet) {
						nextKey.kind = keyGetAndSet
					} else {
						var id logger.MsgID
						var what string
						var where string
						switch in {
						case duplicatePropertiesInObject:
							id = logger.MsgID_JS_DuplicateObjectKey
							what = "key"
							where = "object literal"
						case duplicatePropertiesInClass:
							id = logger.MsgID_JS_DuplicateClassMember
							what = "member"
							where = "class body"
						}
						r := js_lexer.RangeOfIdentifier(p.source, property.Key.Loc)
						p.log.AddIDWithNotes(id, logger.Warning, &p.tracker, r,
							fmt.Sprintf("Duplicate %s %q in %s", what, key, where),
							[]logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, prevKey.loc),
								fmt.Sprintf("The original %s %q is here:", what, key))})
					}
				}

				keys[key] = nextKey
			}
		}
	}
}

func isJumpStatement(data js_ast.S) bool {
	switch data.(type) {
	case *js_ast.SBreak, *js_ast.SContinue, *js_ast.SReturn, *js_ast.SThrow:
		return true
	}

	return false
}

func jumpStmtsLookTheSame(left js_ast.S, right js_ast.S) bool {
	switch a := left.(type) {
	case *js_ast.SBreak:
		b, ok := right.(*js_ast.SBreak)
		return ok && (a.Label == nil) == (b.Label == nil) && (a.Label == nil || a.Label.Ref == b.Label.Ref)

	case *js_ast.SContinue:
		b, ok := right.(*js_ast.SContinue)
		return ok && (a.Label == nil) == (b.Label == nil) && (a.Label == nil || a.Label.Ref == b.Label.Ref)

	case *js_ast.SReturn:
		b, ok := right.(*js_ast.SReturn)
		return ok && (a.ValueOrNil.Data == nil) == (b.ValueOrNil.Data == nil) &&
			(a.ValueOrNil.Data == nil || js_ast.ValuesLookTheSame(a.ValueOrNil.Data, b.ValueOrNil.Data))

	case *js_ast.SThrow:
		b, ok := right.(*js_ast.SThrow)
		return ok && js_ast.ValuesLookTheSame(a.Value.Data, b.Value.Data)
	}

	return false
}

func (p *parser) selectLocalKind(kind js_ast.LocalKind) js_ast.LocalKind {
	// Use "var" instead of "let" and "const" if the variable declaration may
	// need to be separated from the initializer. This allows us to safely move
	// this declaration into a nested scope.
	if p.currentScope.Parent == nil && (kind == js_ast.LocalLet || kind == js_ast.LocalConst) &&
		(p.options.mode == config.ModeBundle || p.willWrapModuleInTryCatchForUsing) {
		return js_ast.LocalVar
	}

	// Optimization: use "let" instead of "const" because it's shorter. This is
	// only done when bundling because assigning to "const" is only an error when
	// bundling.
	if p.options.mode == config.ModeBundle && kind == js_ast.LocalConst && p.options.minifySyntax {
		return js_ast.LocalLet
	}

	return kind
}

func (p *parser) pushScopeForParsePass(kind js_ast.ScopeKind, loc logger.Loc) int {
	parent := p.currentScope
	scope := &js_ast.Scope{
		Kind:    kind,
		Parent:  parent,
		Members: make(map[string]js_ast.ScopeMember),
		Label:   ast.LocRef{Ref: ast.InvalidRef},
	}
	if parent != nil {
		parent.Children = append(parent.Children, scope)
		scope.StrictMode = parent.StrictMode
		scope.UseStrictLoc = parent.UseStrictLoc
	}
	p.currentScope = scope

	// Enforce that scope locations are strictly increasing to help catch bugs
	// where the pushed scopes are mismatched between the first and second passes
	if len(p.scopesInOrder) > 0 {
		prevStart := p.scopesInOrder[len(p.scopesInOrder)-1].loc.Start
		if prevStart >= loc.Start {
			panic(fmt.Sprintf("Scope location %d must be greater than %d", loc.Start, prevStart))
		}
	}

	// Copy down function arguments into the function body scope. That way we get
	// errors if a statement in the function body tries to re-declare any of the
	// arguments.
	if kind == js_ast.ScopeFunctionBody {
		if scope.Parent.Kind != js_ast.ScopeFunctionArgs {
			panic("Internal error")
		}
		for name, member := range scope.Parent.Members {
			// Don't copy down the optional function expression name. Re-declaring
			// the name of a function expression is allowed.
			kind := p.symbols[member.Ref.InnerIndex].Kind
			if kind != ast.SymbolHoistedFunction {
				scope.Members[name] = member
			}
		}
	}

	// Remember the length in case we call popAndDiscardScope() later
	scopeIndex := len(p.scopesInOrder)
	p.scopesInOrder = append(p.scopesInOrder, scopeOrder{loc: loc, scope: scope})
	return scopeIndex
}

func (p *parser) popScope() {
	// We cannot rename anything inside a scope containing a direct eval() call
	if p.currentScope.ContainsDirectEval {
		for _, member := range p.currentScope.Members {
			// Using direct eval when bundling is not a good idea in general because
			// esbuild must assume that it can potentially reach anything in any of
			// the containing scopes. We try to make it work but this isn't possible
			// in some cases.
			//
			// For example, symbols imported using an ESM import are a live binding
			// to the underlying symbol in another file. This is emulated during
			// scope hoisting by erasing the ESM import and just referencing the
			// underlying symbol in the flattened bundle directly. However, that
			// symbol may have a different name which could break uses of direct
			// eval:
			//
			//   // Before bundling
			//   import { foo as bar } from './foo.js'
			//   console.log(eval('bar'))
			//
			//   // After bundling
			//   let foo = 123 // The contents of "foo.js"
			//   console.log(eval('bar'))
			//
			// There really isn't any way to fix this. You can't just rename "foo" to
			// "bar" in the example above because there may be a third bundled file
			// that also contains direct eval and imports the same symbol with a
			// different conflicting import alias. And there is no way to store a
			// live binding to the underlying symbol in a variable with the import's
			// name so that direct eval can access it:
			//
			//   // After bundling
			//   let foo = 123 // The contents of "foo.js"
			//   const bar = /* cannot express a live binding to "foo" here */
			//   console.log(eval('bar'))
			//
			// Technically a "with" statement could potentially make this work (with
			// a big hit to performance), but they are deprecated and are unavailable
			// in strict mode. This is a non-starter since all ESM code is strict mode.
			//
			// So while we still try to obey the requirement that all symbol names are
			// pinned when direct eval is present, we make an exception for top-level
			// symbols in an ESM file when bundling is enabled. We make no guarantee
			// that "eval" will be able to reach these symbols and we allow them to be
			// renamed or removed by tree shaking.
			if p.options.mode == config.ModeBundle && p.currentScope.Parent == nil && p.isFileConsideredESM {
				continue
			}

			p.symbols[member.Ref.InnerIndex].Flags |= ast.MustNotBeRenamed
		}
	}

	p.currentScope = p.currentScope.Parent
}

func (p *parser) popAndDiscardScope(scopeIndex int) {
	// Unwind any newly-added scopes in reverse order
	for i := len(p.scopesInOrder) - 1; i >= scopeIndex; i-- {
		scope := p.scopesInOrder[i].scope
		parent := scope.Parent
		last := len(parent.Children) - 1
		if parent.Children[last] != scope {
			panic("Internal error")
		}
		parent.Children = parent.Children[:last]
	}

	// Move up to the parent scope
	p.currentScope = p.currentScope.Parent

	// Truncate the scope order where we started to pretend we never saw this scope
	p.scopesInOrder = p.scopesInOrder[:scopeIndex]
}

func (p *parser) popAndFlattenScope(scopeIndex int) {
	// Move up to the parent scope
	toFlatten := p.currentScope
	parent := toFlatten.Parent
	p.currentScope = parent

	// Erase this scope from the order. This will shift over the indices of all
	// the scopes that were created after us. However, we shouldn't have to
	// worry about other code with outstanding scope indices for these scopes.
	// These scopes were all created in between this scope's push and pop
	// operations, so they should all be child scopes and should all be popped
	// by the time we get here.
	copy(p.scopesInOrder[scopeIndex:], p.scopesInOrder[scopeIndex+1:])
	p.scopesInOrder = p.scopesInOrder[:len(p.scopesInOrder)-1]

	// Remove the last child from the parent scope
	last := len(parent.Children) - 1
	if parent.Children[last] != toFlatten {
		panic("Internal error")
	}
	parent.Children = parent.Children[:last]

	// Reparent our child scopes into our parent
	for _, scope := range toFlatten.Children {
		scope.Parent = parent
		parent.Children = append(parent.Children, scope)
	}
}

// Undo all scopes pushed and popped after this scope index. This assumes that
// the scope stack is at the same level now as it was at the given scope index.
func (p *parser) discardScopesUpTo(scopeIndex int) {
	// Remove any direct children from their parent
	children := p.currentScope.Children
	for _, child := range p.scopesInOrder[scopeIndex:] {
		if child.scope.Parent == p.currentScope {
			for i := len(children) - 1; i >= 0; i-- {
				if children[i] == child.scope {
					children = append(children[:i], children[i+1:]...)
					break
				}
			}
		}
	}
	p.currentScope.Children = children

	// Truncate the scope order where we started to pretend we never saw this scope
	p.scopesInOrder = p.scopesInOrder[:scopeIndex]
}

func (p *parser) newSymbol(kind ast.SymbolKind, name string) ast.Ref {
	ref := ast.Ref{SourceIndex: p.source.Index, InnerIndex: uint32(len(p.symbols))}
	p.symbols = append(p.symbols, ast.Symbol{
		Kind:         kind,
		OriginalName: name,
		Link:         ast.InvalidRef,
	})
	if p.options.ts.Parse {
		p.tsUseCounts = append(p.tsUseCounts, 0)
	}
	return ref
}

// This is similar to "ast.MergeSymbols" but it works with this parser's
// one-level symbol map instead of the linker's two-level symbol map. It also
// doesn't handle cycles since they shouldn't come up due to the way this
// function is used.
func (p *parser) mergeSymbols(old ast.Ref, new ast.Ref) ast.Ref {
	if old == new {
		return new
	}

	oldSymbol := &p.symbols[old.InnerIndex]
	if oldSymbol.Link != ast.InvalidRef {
		oldSymbol.Link = p.mergeSymbols(oldSymbol.Link, new)
		return oldSymbol.Link
	}

	newSymbol := &p.symbols[new.InnerIndex]
	if newSymbol.Link != ast.InvalidRef {
		newSymbol.Link = p.mergeSymbols(old, newSymbol.Link)
		return newSymbol.Link
	}

	oldSymbol.Link = new
	newSymbol.MergeContentsWith(oldSymbol)
	return new
}

type mergeResult int

const (
	mergeForbidden = iota
	mergeReplaceWithNew
	mergeOverwriteWithNew
	mergeKeepExisting
	mergeBecomePrivateGetSetPair
	mergeBecomePrivateStaticGetSetPair
)

func (p *parser) canMergeSymbols(scope *js_ast.Scope, existing ast.SymbolKind, new ast.SymbolKind) mergeResult {
	if existing == ast.SymbolUnbound {
		return mergeReplaceWithNew
	}

	// In TypeScript, imports are allowed to silently collide with symbols within
	// the module. Presumably this is because the imports may be type-only:
	//
	//   import {Foo} from 'bar'
	//   class Foo {}
	//
	if p.options.ts.Parse && existing == ast.SymbolImport {
		return mergeReplaceWithNew
	}

	// "enum Foo {} enum Foo {}"
	if new == ast.SymbolTSEnum && existing == ast.SymbolTSEnum {
		return mergeKeepExisting
	}

	// "namespace Foo { ... } enum Foo {}"
	if new == ast.SymbolTSEnum && existing == ast.SymbolTSNamespace {
		return mergeReplaceWithNew
	}

	// "namespace Foo { ... } namespace Foo { ... }"
	// "function Foo() {} namespace Foo { ... }"
	// "enum Foo {} namespace Foo { ... }"
	if new == ast.SymbolTSNamespace {
		switch existing {
		case ast.SymbolTSNamespace, ast.SymbolHoistedFunction, ast.SymbolGeneratorOrAsyncFunction, ast.SymbolTSEnum, ast.SymbolClass:
			return mergeKeepExisting
		}
	}

	// "var foo; var foo;"
	// "var foo; function foo() {}"
	// "function foo() {} var foo;"
	// "function *foo() {} function *foo() {}" but not "{ function *foo() {} function *foo() {} }"
	if new.IsHoistedOrFunction() && existing.IsHoistedOrFunction() &&
		(scope.Kind == js_ast.ScopeEntry ||
			scope.Kind == js_ast.ScopeFunctionBody ||
			scope.Kind == js_ast.ScopeFunctionArgs ||
			(new == existing && new.IsHoisted())) {
		return mergeReplaceWithNew
	}

	// "get #foo() {} set #foo() {}"
	// "set #foo() {} get #foo() {}"
	if (existing == ast.SymbolPrivateGet && new == ast.SymbolPrivateSet) ||
		(existing == ast.SymbolPrivateSet && new == ast.SymbolPrivateGet) {
		return mergeBecomePrivateGetSetPair
	}
	if (existing == ast.SymbolPrivateStaticGet && new == ast.SymbolPrivateStaticSet) ||
		(existing == ast.SymbolPrivateStaticSet && new == ast.SymbolPrivateStaticGet) {
		return mergeBecomePrivateStaticGetSetPair
	}

	// "try {} catch (e) { var e }"
	if existing == ast.SymbolCatchIdentifier && new == ast.SymbolHoisted {
		return mergeReplaceWithNew
	}

	// "function() { var arguments }"
	if existing == ast.SymbolArguments && new == ast.SymbolHoisted {
		return mergeKeepExisting
	}

	// "function() { let arguments }"
	if existing == ast.SymbolArguments && new != ast.SymbolHoisted {
		return mergeOverwriteWithNew
	}

	return mergeForbidden
}

func (p *parser) addSymbolAlreadyDeclaredError(name string, newLoc logger.Loc, oldLoc logger.Loc) {
	p.log.AddErrorWithNotes(&p.tracker,
		js_lexer.RangeOfIdentifier(p.source, newLoc),
		fmt.Sprintf("The symbol %q has already been declared", name),

		[]logger.MsgData{p.tracker.MsgData(
			js_lexer.RangeOfIdentifier(p.source, oldLoc),
			fmt.Sprintf("The symbol %q was originally declared here:", name),
		)},
	)
}

func (p *parser) declareSymbol(kind ast.SymbolKind, loc logger.Loc, name string) ast.Ref {
	p.checkForUnrepresentableIdentifier(loc, name)

	// Allocate a new symbol
	ref := p.newSymbol(kind, name)

	// Check for a collision in the declaring scope
	if existing, ok := p.currentScope.Members[name]; ok {
		symbol := &p.symbols[existing.Ref.InnerIndex]

		switch p.canMergeSymbols(p.currentScope, symbol.Kind, kind) {
		case mergeForbidden:
			p.addSymbolAlreadyDeclaredError(name, loc, existing.Loc)
			return existing.Ref

		case mergeKeepExisting:
			ref = existing.Ref

		case mergeReplaceWithNew:
			symbol.Link = ref
			p.currentScope.Replaced = append(p.currentScope.Replaced, existing)

			// If these are both functions, remove the overwritten declaration
			if p.options.minifySyntax && kind.IsFunction() && symbol.Kind.IsFunction() {
				symbol.Flags |= ast.RemoveOverwrittenFunctionDeclaration
			}

		case mergeBecomePrivateGetSetPair:
			ref = existing.Ref
			symbol.Kind = ast.SymbolPrivateGetSetPair

		case mergeBecomePrivateStaticGetSetPair:
			ref = existing.Ref
			symbol.Kind = ast.SymbolPrivateStaticGetSetPair

		case mergeOverwriteWithNew:
		}
	}

	// Overwrite this name in the declaring scope
	p.currentScope.Members[name] = js_ast.ScopeMember{Ref: ref, Loc: loc}
	return ref

}

// This type is just so we can use Go's native sort function
type scopeMemberArray []js_ast.ScopeMember

func (a scopeMemberArray) Len() int          { return len(a) }
func (a scopeMemberArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a scopeMemberArray) Less(i int, j int) bool {
	ai := a[i].Ref
	bj := a[j].Ref
	return ai.InnerIndex < bj.InnerIndex || (ai.InnerIndex == bj.InnerIndex && ai.SourceIndex < bj.SourceIndex)
}

func (p *parser) hoistSymbols(scope *js_ast.Scope) {
	// Duplicate function declarations are forbidden in nested blocks in strict
	// mode. Separately, they are also forbidden at the top-level of modules.
	// This check needs to be delayed until now instead of being done when the
	// functions are declared because we potentially need to scan the whole file
	// to know if the file is considered to be in strict mode (or is considered
	// to be a module). We might only encounter an "export {}" clause at the end
	// of the file.
	if (scope.StrictMode != js_ast.SloppyMode && scope.Kind == js_ast.ScopeBlock) || (scope.Parent == nil && p.isFileConsideredESM) {
		for _, replaced := range scope.Replaced {
			symbol := &p.symbols[replaced.Ref.InnerIndex]
			if symbol.Kind.IsFunction() {
				if member, ok := scope.Members[symbol.OriginalName]; ok && p.symbols[member.Ref.InnerIndex].Kind.IsFunction() {
					var notes []logger.MsgData
					if scope.Parent == nil && p.isFileConsideredESM {
						_, notes = p.whyESModule()
						notes[0].Text = fmt.Sprintf("Duplicate top-level function declarations are not allowed in an ECMAScript module. %s", notes[0].Text)
					} else {
						var where string
						where, notes = p.whyStrictMode(scope)
						notes[0].Text = fmt.Sprintf("Duplicate function declarations are not allowed in nested blocks %s. %s", where, notes[0].Text)
					}

					p.log.AddErrorWithNotes(&p.tracker,
						js_lexer.RangeOfIdentifier(p.source, member.Loc),
						fmt.Sprintf("The symbol %q has already been declared", symbol.OriginalName),

						append([]logger.MsgData{p.tracker.MsgData(
							js_lexer.RangeOfIdentifier(p.source, replaced.Loc),
							fmt.Sprintf("The symbol %q was originally declared here:", symbol.OriginalName),
						)}, notes...),
					)
				}
			}
		}
	}

	if !scope.Kind.StopsHoisting() {
		// We create new symbols in the loop below, so the iteration order of the
		// loop must be deterministic to avoid generating different minified names
		sortedMembers := make(scopeMemberArray, 0, len(scope.Members))
		for _, member := range scope.Members {
			sortedMembers = append(sortedMembers, member)
		}
		sort.Sort(sortedMembers)

	nextMember:
		for _, member := range sortedMembers {
			symbol := &p.symbols[member.Ref.InnerIndex]

			// Handle non-hoisted collisions between catch bindings and the catch body.
			// This implements "B.3.4 VariableStatements in Catch Blocks" from Annex B
			// of the ECMAScript standard version 6+ (except for the hoisted case, which
			// is handled later on below):
			//
			// * It is a Syntax Error if any element of the BoundNames of CatchParameter
			//   also occurs in the LexicallyDeclaredNames of Block.
			//
			// * It is a Syntax Error if any element of the BoundNames of CatchParameter
			//   also occurs in the VarDeclaredNames of Block unless CatchParameter is
			//   CatchParameter : BindingIdentifier .
			//
			if scope.Parent.Kind == js_ast.ScopeCatchBinding && symbol.Kind != ast.SymbolHoisted {
				if existingMember, ok := scope.Parent.Members[symbol.OriginalName]; ok {
					p.addSymbolAlreadyDeclaredError(symbol.OriginalName, member.Loc, existingMember.Loc)
					continue
				}
			}

			if !symbol.Kind.IsHoisted() {
				continue
			}

			// Implement "Block-Level Function Declarations Web Legacy Compatibility
			// Semantics" from Annex B of the ECMAScript standard version 6+
			isSloppyModeBlockLevelFnStmt := false
			originalMemberRef := member.Ref
			if symbol.Kind == ast.SymbolHoistedFunction {
				// Block-level function declarations behave like "let" in strict mode
				if scope.StrictMode != js_ast.SloppyMode {
					continue
				}

				// In sloppy mode, block level functions behave like "let" except with
				// an assignment to "var", sort of. This code:
				//
				//   if (x) {
				//     f();
				//     function f() {}
				//   }
				//   f();
				//
				// behaves like this code:
				//
				//   if (x) {
				//     let f2 = function() {}
				//     var f = f2;
				//     f2();
				//   }
				//   f();
				//
				hoistedRef := p.newSymbol(ast.SymbolHoisted, symbol.OriginalName)
				scope.Generated = append(scope.Generated, hoistedRef)
				if p.hoistedRefForSloppyModeBlockFn == nil {
					p.hoistedRefForSloppyModeBlockFn = make(map[ast.Ref]ast.Ref)
				}
				p.hoistedRefForSloppyModeBlockFn[member.Ref] = hoistedRef
				symbol = &p.symbols[hoistedRef.InnerIndex]
				member.Ref = hoistedRef
				isSloppyModeBlockLevelFnStmt = true
			}

			// Check for collisions that would prevent to hoisting "var" symbols up to the enclosing function scope
			s := scope.Parent
			for {
				// Variable declarations hoisted past a "with" statement may actually end
				// up overwriting a property on the target of the "with" statement instead
				// of initializing the variable. We must not rename them or we risk
				// causing a behavior change.
				//
				//   var obj = { foo: 1 }
				//   with (obj) { var foo = 2 }
				//   assert(foo === undefined)
				//   assert(obj.foo === 2)
				//
				if s.Kind == js_ast.ScopeWith {
					symbol.Flags |= ast.MustNotBeRenamed
				}

				if existingMember, ok := s.Members[symbol.OriginalName]; ok {
					existingSymbol := &p.symbols[existingMember.Ref.InnerIndex]

					// We can hoist the symbol from the child scope into the symbol in
					// this scope if:
					//
					//   - The symbol is unbound (i.e. a global variable access)
					//   - The symbol is also another hoisted variable
					//   - The symbol is a function of any kind and we're in a function or module scope
					//
					// Is this unbound (i.e. a global access) or also hoisted?
					if existingSymbol.Kind == ast.SymbolUnbound || existingSymbol.Kind == ast.SymbolHoisted ||
						(existingSymbol.Kind.IsFunction() && (s.Kind == js_ast.ScopeEntry || s.Kind == js_ast.ScopeFunctionBody)) {
						// Silently merge this symbol into the existing symbol
						symbol.Link = existingMember.Ref
						s.Members[symbol.OriginalName] = existingMember
						continue nextMember
					}

					// Otherwise if this isn't a catch identifier or "arguments", it's a collision
					if existingSymbol.Kind != ast.SymbolCatchIdentifier && existingSymbol.Kind != ast.SymbolArguments {
						// An identifier binding from a catch statement and a function
						// declaration can both silently shadow another hoisted symbol
						if symbol.Kind != ast.SymbolCatchIdentifier && symbol.Kind != ast.SymbolHoistedFunction {
							if !isSloppyModeBlockLevelFnStmt {
								p.addSymbolAlreadyDeclaredError(symbol.OriginalName, member.Loc, existingMember.Loc)
							} else if s == scope.Parent {
								// Never mind about this, turns out it's not needed after all
								delete(p.hoistedRefForSloppyModeBlockFn, originalMemberRef)
							}
						}
						continue nextMember
					}

					// If this is a catch identifier, silently merge the existing symbol
					// into this symbol but continue hoisting past this catch scope
					existingSymbol.Link = member.Ref
					s.Members[symbol.OriginalName] = member
				}

				if s.Kind.StopsHoisting() {
					// Declare the member in the scope that stopped the hoisting
					s.Members[symbol.OriginalName] = member
					break
				}
				s = s.Parent
			}
		}
	}

	for _, child := range scope.Children {
		p.hoistSymbols(child)
	}
}

func (p *parser) declareBinding(kind ast.SymbolKind, binding js_ast.Binding, opts parseStmtOpts) {
	js_ast.ForEachIdentifierBinding(binding, func(loc logger.Loc, b *js_ast.BIdentifier) {
		if !opts.isTypeScriptDeclare || (opts.isNamespaceScope && opts.isExport) {
			b.Ref = p.declareSymbol(kind, loc, p.loadNameFromRef(b.Ref))
		}
	})
}

func (p *parser) recordUsage(ref ast.Ref) {
	// The use count stored in the symbol is used for generating symbol names
	// during minification. These counts shouldn't include references inside dead
	// code regions since those will be culled.
	if !p.isControlFlowDead {
		p.symbols[ref.InnerIndex].UseCountEstimate++
		use := p.symbolUses[ref]
		use.CountEstimate++
		p.symbolUses[ref] = use
	}

	// The correctness of TypeScript-to-JavaScript conversion relies on accurate
	// symbol use counts for the whole file, including dead code regions. This is
	// tracked separately in a parser-only data structure.
	if p.options.ts.Parse {
		p.tsUseCounts[ref.InnerIndex]++
	}
}

func (p *parser) ignoreUsage(ref ast.Ref) {
	// Roll back the use count increment in recordUsage()
	if !p.isControlFlowDead {
		p.symbols[ref.InnerIndex].UseCountEstimate--
		use := p.symbolUses[ref]
		use.CountEstimate--
		if use.CountEstimate == 0 {
			delete(p.symbolUses, ref)
		} else {
			p.symbolUses[ref] = use
		}
	}

	// Don't roll back the "tsUseCounts" increment. This must be counted even if
	// the value is ignored because that's what the TypeScript compiler does.
}

func (p *parser) ignoreUsageOfIdentifierInDotChain(expr js_ast.Expr) {
	for {
		switch e := expr.Data.(type) {
		case *js_ast.EIdentifier:
			p.ignoreUsage(e.Ref)

		case *js_ast.EDot:
			expr = e.Target
			continue

		case *js_ast.EIndex:
			if _, ok := e.Index.Data.(*js_ast.EString); ok {
				expr = e.Target
				continue
			}
		}

		return
	}
}

func (p *parser) importFromRuntime(loc logger.Loc, name string) js_ast.Expr {
	it, ok := p.runtimeImports[name]
	if !ok {
		it.Loc = loc
		it.Ref = p.newSymbol(ast.SymbolOther, name)
		p.moduleScope.Generated = append(p.moduleScope.Generated, it.Ref)
		p.runtimeImports[name] = it
	}
	p.recordUsage(it.Ref)
	return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: it.Ref}}
}

func (p *parser) callRuntime(loc logger.Loc, name string, args []js_ast.Expr) js_ast.Expr {
	return js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
		Target: p.importFromRuntime(loc, name),
		Args:   args,
	}}
}

type JSXImport uint8

const (
	JSXImportJSX JSXImport = iota
	JSXImportJSXS
	JSXImportFragment
	JSXImportCreateElement
)

func (p *parser) importJSXSymbol(loc logger.Loc, jsx JSXImport) js_ast.Expr {
	var symbols map[string]ast.LocRef
	var name string

	switch jsx {
	case JSXImportJSX:
		symbols = p.jsxRuntimeImports
		if p.options.jsx.Development {
			name = "jsxDEV"
		} else {
			name = "jsx"
		}

	case JSXImportJSXS:
		symbols = p.jsxRuntimeImports
		if p.options.jsx.Development {
			name = "jsxDEV"
		} else {
			name = "jsxs"
		}

	case JSXImportFragment:
		symbols = p.jsxRuntimeImports
		name = "Fragment"

	case JSXImportCreateElement:
		symbols = p.jsxLegacyImports
		name = "createElement"
	}

	it, ok := symbols[name]
	if !ok {
		it.Loc = loc
		it.Ref = p.newSymbol(ast.SymbolOther, name)
		p.moduleScope.Generated = append(p.moduleScope.Generated, it.Ref)
		p.isImportItem[it.Ref] = true
		symbols[name] = it
	}

	p.recordUsage(it.Ref)
	return p.handleIdentifier(loc, &js_ast.EIdentifier{Ref: it.Ref}, identifierOpts{
		wasOriginallyIdentifier: true,
	})
}

func (p *parser) valueToSubstituteForRequire(loc logger.Loc) js_ast.Expr {
	if p.source.Index != runtime.SourceIndex &&
		config.ShouldCallRuntimeRequire(p.options.mode, p.options.outputFormat) {
		return p.importFromRuntime(loc, "__require")
	}

	p.recordUsage(p.requireRef)
	return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.requireRef}}
}

func (p *parser) makePromiseRef() ast.Ref {
	if p.promiseRef == ast.InvalidRef {
		p.promiseRef = p.newSymbol(ast.SymbolUnbound, "Promise")
	}
	return p.promiseRef
}

func (p *parser) makeRegExpRef() ast.Ref {
	if p.regExpRef == ast.InvalidRef {
		p.regExpRef = p.newSymbol(ast.SymbolUnbound, "RegExp")
		p.moduleScope.Generated = append(p.moduleScope.Generated, p.regExpRef)
	}
	return p.regExpRef
}

func (p *parser) makeBigIntRef() ast.Ref {
	if p.bigIntRef == ast.InvalidRef {
		p.bigIntRef = p.newSymbol(ast.SymbolUnbound, "BigInt")
		p.moduleScope.Generated = append(p.moduleScope.Generated, p.bigIntRef)
	}
	return p.bigIntRef
}

// The name is temporarily stored in the ref until the scope traversal pass
// happens, at which point a symbol will be generated and the ref will point
// to the symbol instead.
//
// The scope traversal pass will reconstruct the name using one of two methods.
// In the common case, the name is a slice of the file itself. In that case we
// can just store the slice and not need to allocate any extra memory. In the
// rare case, the name is an externally-allocated string. In that case we store
// an index to the string and use that index during the scope traversal pass.
func (p *parser) storeNameInRef(name js_lexer.MaybeSubstring) ast.Ref {
	// Is the data in "name" a subset of the data in "p.source.Contents"?
	if name.Start.IsValid() {
		// The name is a slice of the file contents, so we can just reference it by
		// length and don't have to allocate anything. This is the common case.
		//
		// It's stored as a negative value so we'll crash if we try to use it. That
		// way we'll catch cases where we've forgotten to call loadNameFromRef().
		// The length is the negative part because we know it's non-zero.
		return ast.Ref{SourceIndex: -uint32(len(name.String)), InnerIndex: uint32(name.Start.GetIndex())}
	} else {
		// The name is some memory allocated elsewhere. This is either an inline
		// string constant in the parser or an identifier with escape sequences
		// in the source code, which is very unusual. Stash it away for later.
		// This uses allocations but it should hopefully be very uncommon.
		ref := ast.Ref{SourceIndex: 0x80000000, InnerIndex: uint32(len(p.allocatedNames))}
		p.allocatedNames = append(p.allocatedNames, name.String)
		return ref
	}
}

// This is the inverse of storeNameInRef() above
func (p *parser) loadNameFromRef(ref ast.Ref) string {
	if ref.SourceIndex == 0x80000000 {
		return p.allocatedNames[ref.InnerIndex]
	} else {
		if (ref.SourceIndex & 0x80000000) == 0 {
			panic("Internal error: invalid symbol reference")
		}
		return p.source.Contents[ref.InnerIndex : int32(ref.InnerIndex)-int32(ref.SourceIndex)]
	}
}

// Due to ES6 destructuring patterns, there are many cases where it's
// impossible to distinguish between an array or object literal and a
// destructuring assignment until we hit the "=" operator later on.
// This object defers errors about being in one state or the other
// until we discover which state we're in.
type deferredErrors struct {
	// These are errors for expressions
	invalidExprDefaultValue  logger.Range
	invalidExprAfterQuestion logger.Range
	arraySpreadFeature       logger.Range

	// These errors are for arrow functions
	invalidParens []logger.Range
}

func (from *deferredErrors) mergeInto(to *deferredErrors) {
	if from.invalidExprDefaultValue.Len > 0 {
		to.invalidExprDefaultValue = from.invalidExprDefaultValue
	}
	if from.invalidExprAfterQuestion.Len > 0 {
		to.invalidExprAfterQuestion = from.invalidExprAfterQuestion
	}
	if from.arraySpreadFeature.Len > 0 {
		to.arraySpreadFeature = from.arraySpreadFeature
	}
	if len(from.invalidParens) > 0 {
		if len(to.invalidParens) > 0 {
			to.invalidParens = append(to.invalidParens, from.invalidParens...)
		} else {
			to.invalidParens = from.invalidParens
		}
	}
}

func (p *parser) logExprErrors(errors *deferredErrors) {
	if errors.invalidExprDefaultValue.Len > 0 {
		p.log.AddError(&p.tracker, errors.invalidExprDefaultValue, "Unexpected \"=\"")
	}

	if errors.invalidExprAfterQuestion.Len > 0 {
		r := errors.invalidExprAfterQuestion
		p.log.AddError(&p.tracker, r, fmt.Sprintf("Unexpected %q", p.source.Contents[r.Loc.Start:r.Loc.Start+r.Len]))
	}

	if errors.arraySpreadFeature.Len > 0 {
		p.markSyntaxFeature(compat.ArraySpread, errors.arraySpreadFeature)
	}
}

func (p *parser) logDeferredArrowArgErrors(errors *deferredErrors) {
	for _, paren := range errors.invalidParens {
		p.log.AddError(&p.tracker, paren, "Invalid binding pattern")
	}
}

func (p *parser) logNullishCoalescingErrorPrecedenceError(op string) {
	prevOp := "??"
	if p.lexer.Token == js_lexer.TQuestionQuestion {
		op, prevOp = prevOp, op
	}
	// p.log.AddError(&p.tracker, p.lexer.Range(), fmt.Sprintf("The %q operator requires parentheses"))
	p.log.AddErrorWithNotes(&p.tracker, p.lexer.Range(), fmt.Sprintf("Cannot use %q with %q without parentheses", op, prevOp),
		[]logger.MsgData{{Text: fmt.Sprintf("Expressions of the form \"x %s y %s z\" are not allowed in JavaScript. "+
			"You must disambiguate between \"(x %s y) %s z\" and \"x %s (y %s z)\" by adding parentheses.", prevOp, op, prevOp, op, prevOp, op)}})
}

func defineValueCanBeUsedInAssignTarget(data js_ast.E) bool {
	switch data.(type) {
	case *js_ast.EIdentifier, *js_ast.EDot:
		return true
	}

	// Substituting a constant into an assignment target (e.g. "x = 1" becomes
	// "0 = 1") will cause a syntax error, so we avoid doing this. The caller
	// will log a warning instead.
	return false
}

func (p *parser) logAssignToDefine(r logger.Range, name string, expr js_ast.Expr) {
	// If this is a compound expression, pretty-print it for the error message.
	// We don't use a literal slice of the source text in case it contains
	// problematic things (e.g. spans multiple lines, has embedded comments).
	if expr.Data != nil {
		var parts []string
		for {
			if id, ok := expr.Data.(*js_ast.EIdentifier); ok {
				parts = append(parts, p.loadNameFromRef(id.Ref))
				break
			} else if dot, ok := expr.Data.(*js_ast.EDot); ok {
				parts = append(parts, dot.Name)
				parts = append(parts, ".")
				expr = dot.Target
			} else if index, ok := expr.Data.(*js_ast.EIndex); ok {
				if str, ok := index.Index.Data.(*js_ast.EString); ok {
					parts = append(parts, "]")
					parts = append(parts, string(helpers.QuoteSingle(helpers.UTF16ToString(str.Value), false)))
					parts = append(parts, "[")
					expr = index.Target
				} else {
					return
				}
			} else {
				return
			}
		}
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
		name = strings.Join(parts, "")
	}

	kind := logger.Warning
	if p.suppressWarningsAboutWeirdCode {
		kind = logger.Debug
	}

	p.log.AddIDWithNotes(logger.MsgID_JS_AssignToDefine, kind, &p.tracker, r,
		fmt.Sprintf("Suspicious assignment to defined constant %q", name),
		[]logger.MsgData{{Text: fmt.Sprintf(
			"The expression %q has been configured to be replaced with a constant using the \"define\" feature. "+
				"If this expression is supposed to be a compile-time constant, then it doesn't make sense to assign to it here. "+
				"Or if this expression is supposed to change at run-time, this \"define\" substitution should be removed.", name)}})
}

// The "await" and "yield" expressions are never allowed in argument lists but
// may or may not be allowed otherwise depending on the details of the enclosing
// function or module. This needs to be handled when parsing an arrow function
// argument list because we don't know if these expressions are not allowed until
// we reach the "=>" token (or discover the absence of one).
//
// Specifically, for await:
//
//	// This is ok
//	async function foo() { (x = await y) }
//
//	// This is an error
//	async function foo() { (x = await y) => {} }
//
// And for yield:
//
//	// This is ok
//	function* foo() { (x = yield y) }
//
//	// This is an error
//	function* foo() { (x = yield y) => {} }
type deferredArrowArgErrors struct {
	invalidExprAwait logger.Range
	invalidExprYield logger.Range
}

func (p *parser) logArrowArgErrors(errors *deferredArrowArgErrors) {
	if errors.invalidExprAwait.Len > 0 {
		p.log.AddError(&p.tracker, errors.invalidExprAwait, "Cannot use an \"await\" expression here:")
	}

	if errors.invalidExprYield.Len > 0 {
		p.log.AddError(&p.tracker, errors.invalidExprYield, "Cannot use a \"yield\" expression here:")
	}
}

func (p *parser) keyNameForError(key js_ast.Expr) string {
	switch k := key.Data.(type) {
	case *js_ast.EString:
		return fmt.Sprintf("%q", helpers.UTF16ToString(k.Value))
	case *js_ast.EPrivateIdentifier:
		return fmt.Sprintf("%q", p.loadNameFromRef(k.Ref))
	}
	return "property"
}

func (p *parser) checkForLegacyOctalLiteral(e js_ast.E) {
	if p.lexer.IsLegacyOctalLiteral {
		if p.legacyOctalLiterals == nil {
			p.legacyOctalLiterals = make(map[js_ast.E]logger.Range)
		}
		p.legacyOctalLiterals[e] = p.lexer.Range()
	}
}

func (p *parser) notesForAssertTypeJSON(record *ast.ImportRecord, alias string) []logger.MsgData {
	return []logger.MsgData{p.tracker.MsgData(
		js_lexer.RangeOfImportAssertOrWith(p.source, *ast.FindAssertOrWithEntry(record.AssertOrWith.Entries, "type"), js_lexer.KeyAndValueRange),
		"The JSON import assertion is here:"),
		{Text: fmt.Sprintf("You can either keep the import assertion and only use the \"default\" import, "+
			"or you can remove the import assertion and use the %q import.", alias)}}
}

// This assumes the caller has already checked for TStringLiteral or TNoSubstitutionTemplateLiteral
func (p *parser) parseStringLiteral() js_ast.Expr {
	var legacyOctalLoc logger.Loc
	loc := p.lexer.Loc()
	text := p.lexer.StringLiteral()

	// Enable using a "/* @__KEY__ */" comment to turn a string into a key
	hasPropertyKeyComment := (p.lexer.HasCommentBefore & js_lexer.KeyCommentBefore) != 0
	if hasPropertyKeyComment {
		if name := helpers.UTF16ToString(text); p.isMangledProp(name) {
			value := js_ast.Expr{Loc: loc, Data: &js_ast.ENameOfSymbol{
				Ref:                   p.storeNameInRef(js_lexer.MaybeSubstring{String: name}),
				HasPropertyKeyComment: true,
			}}
			p.lexer.Next()
			return value
		}
	}

	if p.lexer.LegacyOctalLoc.Start > loc.Start {
		legacyOctalLoc = p.lexer.LegacyOctalLoc
	}
	value := js_ast.Expr{Loc: loc, Data: &js_ast.EString{
		Value:                 text,
		LegacyOctalLoc:        legacyOctalLoc,
		PreferTemplate:        p.lexer.Token == js_lexer.TNoSubstitutionTemplateLiteral,
		HasPropertyKeyComment: hasPropertyKeyComment,
	}}
	p.lexer.Next()
	return value
}

func (p *parser) parseBigIntOrStringIfUnsupported() js_ast.Expr {
	if p.options.unsupportedJSFeatures.Has(compat.Bigint) {
		var i big.Int
		fmt.Sscan(p.lexer.Identifier.String, &i)
		return js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EString{Value: helpers.StringToUTF16(i.String())}}
	}
	return js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EBigInt{Value: p.lexer.Identifier.String}}
}

type propertyOpts struct {
	decorators       []js_ast.Decorator
	decoratorScope   *js_ast.Scope
	decoratorContext decoratorContextFlags

	asyncRange     logger.Range
	generatorRange logger.Range
	tsDeclareRange logger.Range
	classKeyword   logger.Range
	isAsync        bool
	isGenerator    bool

	// Class-related options
	isStatic        bool
	isTSAbstract    bool
	isClass         bool
	classHasExtends bool
}

func (p *parser) parseProperty(startLoc logger.Loc, kind js_ast.PropertyKind, opts propertyOpts, errors *deferredErrors) (js_ast.Property, bool) {
	var flags js_ast.PropertyFlags
	var key js_ast.Expr
	var closeBracketLoc logger.Loc
	keyRange := p.lexer.Range()

	switch p.lexer.Token {
	case js_lexer.TNumericLiteral:
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.ENumber{Value: p.lexer.Number}}
		p.checkForLegacyOctalLiteral(key.Data)
		p.lexer.Next()

	case js_lexer.TStringLiteral:
		key = p.parseStringLiteral()
		if !p.options.minifySyntax {
			flags |= js_ast.PropertyPreferQuotedKey
		}

	case js_lexer.TBigIntegerLiteral:
		key = p.parseBigIntOrStringIfUnsupported()
		p.lexer.Next()

	case js_lexer.TPrivateIdentifier:
		if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True && len(opts.decorators) > 0 {
			p.log.AddError(&p.tracker, p.lexer.Range(), "TypeScript experimental decorators cannot be used on private identifiers")
		} else if !opts.isClass {
			p.lexer.Expected(js_lexer.TIdentifier)
		} else if opts.tsDeclareRange.Len != 0 {
			p.log.AddError(&p.tracker, opts.tsDeclareRange, "\"declare\" cannot be used with a private identifier")
		}
		name := p.lexer.Identifier
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EPrivateIdentifier{Ref: p.storeNameInRef(name)}}
		p.reportPrivateNameUsage(name.String)
		p.lexer.Next()

	case js_lexer.TOpenBracket:
		flags |= js_ast.PropertyIsComputed
		p.markSyntaxFeature(compat.ObjectExtensions, p.lexer.Range())
		p.lexer.Next()
		wasIdentifier := p.lexer.Token == js_lexer.TIdentifier
		expr := p.parseExpr(js_ast.LComma)

		// Handle index signatures
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon && wasIdentifier && opts.isClass {
			if _, ok := expr.Data.(*js_ast.EIdentifier); ok {
				if opts.tsDeclareRange.Len != 0 {
					p.log.AddError(&p.tracker, opts.tsDeclareRange, "\"declare\" cannot be used with an index signature")
				}

				// "[key: string]: any;"
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
				p.lexer.Expect(js_lexer.TCloseBracket)
				p.lexer.Expect(js_lexer.TColon)
				p.skipTypeScriptType(js_ast.LLowest)
				p.lexer.ExpectOrInsertSemicolon()

				// Skip this property entirely
				return js_ast.Property{}, false
			}
		}

		closeBracketLoc = p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBracket)
		key = expr

	case js_lexer.TAsterisk:
		if kind != js_ast.PropertyField && (kind != js_ast.PropertyMethod || opts.isGenerator) {
			p.lexer.Unexpected()
		}
		opts.isGenerator = true
		opts.generatorRange = p.lexer.Range()
		p.lexer.Next()
		return p.parseProperty(startLoc, js_ast.PropertyMethod, opts, errors)

	default:
		name := p.lexer.Identifier
		raw := p.lexer.Raw()
		nameRange := p.lexer.Range()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(js_lexer.TIdentifier)
		}
		p.lexer.Next()

		// Support contextual keywords
		if kind == js_ast.PropertyField {
			// Does the following token look like a key?
			couldBeModifierKeyword := p.lexer.IsIdentifierOrKeyword()
			if !couldBeModifierKeyword {
				switch p.lexer.Token {
				case js_lexer.TOpenBracket, js_lexer.TNumericLiteral, js_lexer.TStringLiteral, js_lexer.TPrivateIdentifier:
					couldBeModifierKeyword = true
				case js_lexer.TAsterisk:
					if opts.isAsync || (raw != "get" && raw != "set") {
						couldBeModifierKeyword = true
					}
				}
			}

			// If so, check for a modifier keyword
			if couldBeModifierKeyword {
				switch raw {
				case "get":
					if !opts.isAsync {
						p.markSyntaxFeature(compat.ObjectAccessors, nameRange)
						return p.parseProperty(startLoc, js_ast.PropertyGetter, opts, nil)
					}

				case "set":
					if !opts.isAsync {
						p.markSyntaxFeature(compat.ObjectAccessors, nameRange)
						return p.parseProperty(startLoc, js_ast.PropertySetter, opts, nil)
					}

				case "accessor":
					if !p.lexer.HasNewlineBefore && !opts.isAsync && opts.isClass {
						return p.parseProperty(startLoc, js_ast.PropertyAutoAccessor, opts, nil)
					}

				case "async":
					if !p.lexer.HasNewlineBefore && !opts.isAsync {
						opts.isAsync = true
						opts.asyncRange = nameRange
						return p.parseProperty(startLoc, js_ast.PropertyMethod, opts, nil)
					}

				case "static":
					if !opts.isStatic && !opts.isAsync && opts.isClass {
						opts.isStatic = true
						return p.parseProperty(startLoc, kind, opts, nil)
					}

				case "declare":
					if !p.lexer.HasNewlineBefore && opts.isClass && p.options.ts.Parse && opts.tsDeclareRange.Len == 0 {
						opts.tsDeclareRange = nameRange
						scopeIndex := len(p.scopesInOrder)

						if prop, ok := p.parseProperty(startLoc, kind, opts, nil); ok &&
							prop.Kind == js_ast.PropertyField && prop.ValueOrNil.Data == nil &&
							(p.options.ts.Config.ExperimentalDecorators == config.True && len(opts.decorators) > 0) {
							// If this is a well-formed class field with the "declare" keyword,
							// only keep the declaration to preserve its side-effects when
							// there are TypeScript experimental decorators present:
							//
							//   class Foo {
							//     // Remove this
							//     declare [(console.log('side effect 1'), 'foo')]
							//
							//     // Keep this
							//     @decorator(console.log('side effect 2')) declare bar
							//   }
							//
							// This behavior is surprisingly somehow valid with TypeScript
							// experimental decorators, which was possibly by accident.
							// TypeScript does not allow this with JavaScript decorators.
							//
							// References:
							//
							//   https://github.com/evanw/esbuild/issues/1675
							//   https://github.com/microsoft/TypeScript/issues/46345
							//
							prop.Kind = js_ast.PropertyDeclareOrAbstract
							return prop, true
						}

						p.discardScopesUpTo(scopeIndex)
						return js_ast.Property{}, false
					}

				case "abstract":
					if !p.lexer.HasNewlineBefore && opts.isClass && p.options.ts.Parse && !opts.isTSAbstract {
						opts.isTSAbstract = true
						scopeIndex := len(p.scopesInOrder)

						if prop, ok := p.parseProperty(startLoc, kind, opts, nil); ok &&
							prop.Kind == js_ast.PropertyField && prop.ValueOrNil.Data == nil &&
							(p.options.ts.Config.ExperimentalDecorators == config.True && len(opts.decorators) > 0) {
							// If this is a well-formed class field with the "abstract" keyword,
							// only keep the declaration to preserve its side-effects when
							// there are TypeScript experimental decorators present:
							//
							//   abstract class Foo {
							//     // Remove this
							//     abstract [(console.log('side effect 1'), 'foo')]
							//
							//     // Keep this
							//     @decorator(console.log('side effect 2')) abstract bar
							//   }
							//
							// This behavior is valid with TypeScript experimental decorators.
							// TypeScript does not allow this with JavaScript decorators.
							//
							// References:
							//
							//   https://github.com/evanw/esbuild/issues/3684
							//
							prop.Kind = js_ast.PropertyDeclareOrAbstract
							return prop, true
						}

						p.discardScopesUpTo(scopeIndex)
						return js_ast.Property{}, false
					}

				case "private", "protected", "public", "readonly", "override":
					// Skip over TypeScript keywords
					if opts.isClass && p.options.ts.Parse {
						return p.parseProperty(startLoc, kind, opts, nil)
					}
				}
			} else if p.lexer.Token == js_lexer.TOpenBrace && name.String == "static" && len(opts.decorators) == 0 {
				loc := p.lexer.Loc()
				p.lexer.Next()

				oldFnOrArrowDataParse := p.fnOrArrowDataParse
				p.fnOrArrowDataParse = fnOrArrowDataParse{
					isReturnDisallowed: true,
					allowSuperProperty: true,
					await:              forbidAll,
				}

				p.pushScopeForParsePass(js_ast.ScopeClassStaticInit, loc)
				stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
				p.popScope()

				p.fnOrArrowDataParse = oldFnOrArrowDataParse

				closeBraceLoc := p.lexer.Loc()
				p.lexer.Expect(js_lexer.TCloseBrace)
				return js_ast.Property{
					Kind: js_ast.PropertyClassStaticBlock,
					Loc:  startLoc,
					ClassStaticBlock: &js_ast.ClassStaticBlock{
						Loc:   loc,
						Block: js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc},
					},
				}, true
			}
		}

		if p.isMangledProp(name.String) {
			key = js_ast.Expr{Loc: nameRange.Loc, Data: &js_ast.ENameOfSymbol{
				Ref:                   p.storeNameInRef(name),
				HasPropertyKeyComment: true,
			}}
		} else {
			key = js_ast.Expr{Loc: nameRange.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(name.String)}}
		}

		// Parse a shorthand property
		if !opts.isClass && kind == js_ast.PropertyField && p.lexer.Token != js_lexer.TColon &&
			p.lexer.Token != js_lexer.TOpenParen && p.lexer.Token != js_lexer.TLessThan &&
			js_lexer.Keywords[name.String] == js_lexer.T(0) {

			// Forbid invalid identifiers
			if (p.fnOrArrowDataParse.await != allowIdent && name.String == "await") ||
				(p.fnOrArrowDataParse.yield != allowIdent && name.String == "yield") {
				p.log.AddError(&p.tracker, nameRange, fmt.Sprintf("Cannot use %q as an identifier here:", name.String))
			}

			ref := p.storeNameInRef(name)
			value := js_ast.Expr{Loc: key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}

			// Destructuring patterns have an optional default value
			var initializerOrNil js_ast.Expr
			if errors != nil && p.lexer.Token == js_lexer.TEquals {
				errors.invalidExprDefaultValue = p.lexer.Range()
				p.lexer.Next()
				initializerOrNil = p.parseExpr(js_ast.LComma)
			}

			return js_ast.Property{
				Kind:             kind,
				Loc:              startLoc,
				Key:              key,
				ValueOrNil:       value,
				InitializerOrNil: initializerOrNil,
				Flags:            js_ast.PropertyWasShorthand,
			}, true
		}
	}

	hasTypeParameters := false
	hasDefiniteAssignmentAssertionOperator := false

	if p.options.ts.Parse {
		if opts.isClass {
			if p.lexer.Token == js_lexer.TQuestion {
				// "class X { foo?: number }"
				// "class X { foo?(): number }"
				p.lexer.Next()
			} else if p.lexer.Token == js_lexer.TExclamation && !p.lexer.HasNewlineBefore &&
				(kind == js_ast.PropertyField || kind == js_ast.PropertyAutoAccessor) {
				// "class X { foo!: number }"
				p.lexer.Next()
				hasDefiniteAssignmentAssertionOperator = true
			}
		}

		// "class X { foo?<T>(): T }"
		// "const x = { foo<T>(): T {} }"
		if !hasDefiniteAssignmentAssertionOperator && kind != js_ast.PropertyAutoAccessor {
			hasTypeParameters = p.skipTypeScriptTypeParameters(allowConstModifier) != didNotSkipAnything
		}
	}

	// Parse a class field with an optional initial value
	if kind == js_ast.PropertyAutoAccessor || (opts.isClass && kind == js_ast.PropertyField &&
		!hasTypeParameters && (p.lexer.Token != js_lexer.TOpenParen || hasDefiniteAssignmentAssertionOperator)) {
		var initializerOrNil js_ast.Expr

		// Forbid the names "constructor" and "prototype" in some cases
		if !flags.Has(js_ast.PropertyIsComputed) {
			if str, ok := key.Data.(*js_ast.EString); ok && (helpers.UTF16EqualsString(str.Value, "constructor") ||
				(opts.isStatic && helpers.UTF16EqualsString(str.Value, "prototype"))) {
				p.log.AddError(&p.tracker, keyRange, fmt.Sprintf("Invalid field name %q", helpers.UTF16ToString(str.Value)))
			}
		}

		// Skip over types
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon {
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)
		}

		if p.lexer.Token == js_lexer.TEquals {
			p.lexer.Next()

			// "this" and "super" property access is allowed in field initializers
			oldIsThisDisallowed := p.fnOrArrowDataParse.isThisDisallowed
			oldAllowSuperProperty := p.fnOrArrowDataParse.allowSuperProperty
			p.fnOrArrowDataParse.isThisDisallowed = false
			p.fnOrArrowDataParse.allowSuperProperty = true

			initializerOrNil = p.parseExpr(js_ast.LComma)

			p.fnOrArrowDataParse.isThisDisallowed = oldIsThisDisallowed
			p.fnOrArrowDataParse.allowSuperProperty = oldAllowSuperProperty
		}

		// Special-case private identifiers
		if private, ok := key.Data.(*js_ast.EPrivateIdentifier); ok {
			name := p.loadNameFromRef(private.Ref)
			if name == "#constructor" {
				p.log.AddError(&p.tracker, keyRange, fmt.Sprintf("Invalid field name %q", name))
			}
			var declare ast.SymbolKind
			if kind == js_ast.PropertyAutoAccessor {
				if opts.isStatic {
					declare = ast.SymbolPrivateStaticGetSetPair
				} else {
					declare = ast.SymbolPrivateGetSetPair
				}
				private.Ref = p.declareSymbol(declare, key.Loc, name)
				p.privateGetters[private.Ref] = p.newSymbol(ast.SymbolOther, name[1:]+"_get")
				p.privateSetters[private.Ref] = p.newSymbol(ast.SymbolOther, name[1:]+"_set")
			} else {
				if opts.isStatic {
					declare = ast.SymbolPrivateStaticField
				} else {
					declare = ast.SymbolPrivateField
				}
				private.Ref = p.declareSymbol(declare, key.Loc, name)
			}
		}

		p.lexer.ExpectOrInsertSemicolon()
		if opts.isStatic {
			flags |= js_ast.PropertyIsStatic
		}
		return js_ast.Property{
			Decorators:       opts.decorators,
			Loc:              startLoc,
			Kind:             kind,
			Flags:            flags,
			Key:              key,
			InitializerOrNil: initializerOrNil,
			CloseBracketLoc:  closeBracketLoc,
		}, true
	}

	// Parse a method expression
	if p.lexer.Token == js_lexer.TOpenParen || kind.IsMethodDefinition() || opts.isClass {
		hasError := false

		if !hasError && opts.tsDeclareRange.Len != 0 {
			what := "method"
			if kind == js_ast.PropertyGetter {
				what = "getter"
			} else if kind == js_ast.PropertySetter {
				what = "setter"
			}
			p.log.AddError(&p.tracker, opts.tsDeclareRange, "\"declare\" cannot be used with a "+what)
			hasError = true
		}

		if opts.isAsync && p.markAsyncFn(opts.asyncRange, opts.isGenerator) {
			hasError = true
		}

		if !hasError && opts.isGenerator && p.markSyntaxFeature(compat.Generator, opts.generatorRange) {
			hasError = true
		}

		if !hasError && p.lexer.Token == js_lexer.TOpenParen && kind != js_ast.PropertyGetter && kind != js_ast.PropertySetter && p.markSyntaxFeature(compat.ObjectExtensions, p.lexer.Range()) {
			hasError = true
		}

		loc := p.lexer.Loc()
		scopeIndex := p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, loc)
		isConstructor := false

		// Forbid the names "constructor" and "prototype" in some cases
		if opts.isClass && !flags.Has(js_ast.PropertyIsComputed) {
			if str, ok := key.Data.(*js_ast.EString); ok {
				if !opts.isStatic && helpers.UTF16EqualsString(str.Value, "constructor") {
					switch {
					case kind == js_ast.PropertyGetter:
						p.log.AddError(&p.tracker, keyRange, "Class constructor cannot be a getter")
					case kind == js_ast.PropertySetter:
						p.log.AddError(&p.tracker, keyRange, "Class constructor cannot be a setter")
					case opts.isAsync:
						p.log.AddError(&p.tracker, keyRange, "Class constructor cannot be an async function")
					case opts.isGenerator:
						p.log.AddError(&p.tracker, keyRange, "Class constructor cannot be a generator")
					default:
						isConstructor = true
					}
				} else if opts.isStatic && helpers.UTF16EqualsString(str.Value, "prototype") {
					p.log.AddError(&p.tracker, keyRange, "Invalid static method name \"prototype\"")
				}
			}
		}

		await := allowIdent
		yield := allowIdent
		if opts.isAsync {
			await = allowExpr
		}
		if opts.isGenerator {
			yield = allowExpr
		}

		fn, hadBody := p.parseFn(nil, opts.classKeyword, opts.decoratorContext, fnOrArrowDataParse{
			needsAsyncLoc:      key.Loc,
			asyncRange:         opts.asyncRange,
			await:              await,
			yield:              yield,
			allowSuperCall:     opts.classHasExtends && isConstructor,
			allowSuperProperty: true,
			decoratorScope:     opts.decoratorScope,
			isConstructor:      isConstructor,

			// Only allow omitting the body if we're parsing TypeScript class
			allowMissingBodyForTypeScript: p.options.ts.Parse && opts.isClass,
		})

		// "class Foo { foo(): void; foo(): void {} }"
		if !hadBody {
			// Skip this property entirely
			p.popAndDiscardScope(scopeIndex)
			return js_ast.Property{}, false
		}

		p.popScope()
		fn.IsUniqueFormalParameters = true
		value := js_ast.Expr{Loc: loc, Data: &js_ast.EFunction{Fn: fn}}

		// Enforce argument rules for accessors
		switch kind {
		case js_ast.PropertyGetter:
			if len(fn.Args) > 0 {
				r := js_lexer.RangeOfIdentifier(p.source, fn.Args[0].Binding.Loc)
				p.log.AddError(&p.tracker, r, fmt.Sprintf("Getter %s must have zero arguments", p.keyNameForError(key)))
			}

		case js_ast.PropertySetter:
			if len(fn.Args) != 1 {
				r := js_lexer.RangeOfIdentifier(p.source, key.Loc)
				if len(fn.Args) > 1 {
					r = js_lexer.RangeOfIdentifier(p.source, fn.Args[1].Binding.Loc)
				}
				p.log.AddError(&p.tracker, r, fmt.Sprintf("Setter %s must have exactly one argument", p.keyNameForError(key)))
			}

		default:
			kind = js_ast.PropertyMethod
		}

		// Special-case private identifiers
		if private, ok := key.Data.(*js_ast.EPrivateIdentifier); ok {
			var declare ast.SymbolKind
			var suffix string
			switch kind {
			case js_ast.PropertyGetter:
				if opts.isStatic {
					declare = ast.SymbolPrivateStaticGet
				} else {
					declare = ast.SymbolPrivateGet
				}
				suffix = "_get"
			case js_ast.PropertySetter:
				if opts.isStatic {
					declare = ast.SymbolPrivateStaticSet
				} else {
					declare = ast.SymbolPrivateSet
				}
				suffix = "_set"
			default:
				if opts.isStatic {
					declare = ast.SymbolPrivateStaticMethod
				} else {
					declare = ast.SymbolPrivateMethod
				}
				suffix = "_fn"
			}
			name := p.loadNameFromRef(private.Ref)
			if name == "#constructor" {
				p.log.AddError(&p.tracker, keyRange, fmt.Sprintf("Invalid method name %q", name))
			}
			private.Ref = p.declareSymbol(declare, key.Loc, name)
			methodRef := p.newSymbol(ast.SymbolOther, name[1:]+suffix)
			if kind == js_ast.PropertySetter {
				p.privateSetters[private.Ref] = methodRef
			} else {
				p.privateGetters[private.Ref] = methodRef
			}
		}

		if opts.isStatic {
			flags |= js_ast.PropertyIsStatic
		}
		return js_ast.Property{
			Decorators:      opts.decorators,
			Loc:             startLoc,
			Kind:            kind,
			Flags:           flags,
			Key:             key,
			ValueOrNil:      value,
			CloseBracketLoc: closeBracketLoc,
		}, true
	}

	// Parse an object key/value pair
	p.lexer.Expect(js_lexer.TColon)
	value := p.parseExprOrBindings(js_ast.LComma, errors)
	return js_ast.Property{
		Loc:             startLoc,
		Kind:            kind,
		Flags:           flags,
		Key:             key,
		ValueOrNil:      value,
		CloseBracketLoc: closeBracketLoc,
	}, true
}

func (p *parser) parsePropertyBinding() js_ast.PropertyBinding {
	var key js_ast.Expr
	var closeBracketLoc logger.Loc
	isComputed := false
	preferQuotedKey := false
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case js_lexer.TDotDotDot:
		p.lexer.Next()
		value := js_ast.Binding{Loc: p.saveExprCommentsHere(), Data: &js_ast.BIdentifier{Ref: p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Expect(js_lexer.TIdentifier)
		return js_ast.PropertyBinding{
			Loc:      loc,
			IsSpread: true,
			Value:    value,
		}

	case js_lexer.TNumericLiteral:
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.ENumber{Value: p.lexer.Number}}
		p.checkForLegacyOctalLiteral(key.Data)
		p.lexer.Next()

	case js_lexer.TStringLiteral:
		key = p.parseStringLiteral()
		preferQuotedKey = !p.options.minifySyntax

	case js_lexer.TBigIntegerLiteral:
		key = p.parseBigIntOrStringIfUnsupported()
		p.lexer.Next()

	case js_lexer.TOpenBracket:
		isComputed = true
		p.lexer.Next()
		key = p.parseExpr(js_ast.LComma)
		closeBracketLoc = p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBracket)

	default:
		name := p.lexer.Identifier
		nameRange := p.lexer.Range()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(js_lexer.TIdentifier)
		}
		p.lexer.Next()
		if p.isMangledProp(name.String) {
			key = js_ast.Expr{Loc: nameRange.Loc, Data: &js_ast.ENameOfSymbol{Ref: p.storeNameInRef(name)}}
		} else {
			key = js_ast.Expr{Loc: nameRange.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(name.String)}}
		}

		if p.lexer.Token != js_lexer.TColon && p.lexer.Token != js_lexer.TOpenParen {
			// Forbid invalid identifiers
			if (p.fnOrArrowDataParse.await != allowIdent && name.String == "await") ||
				(p.fnOrArrowDataParse.yield != allowIdent && name.String == "yield") {
				p.log.AddError(&p.tracker, nameRange, fmt.Sprintf("Cannot use %q as an identifier here:", name.String))
			}

			ref := p.storeNameInRef(name)
			value := js_ast.Binding{Loc: nameRange.Loc, Data: &js_ast.BIdentifier{Ref: ref}}

			var defaultValueOrNil js_ast.Expr
			if p.lexer.Token == js_lexer.TEquals {
				p.lexer.Next()
				defaultValueOrNil = p.parseExpr(js_ast.LComma)
			}

			return js_ast.PropertyBinding{
				Loc:               loc,
				Key:               key,
				Value:             value,
				DefaultValueOrNil: defaultValueOrNil,
			}
		}
	}

	p.lexer.Expect(js_lexer.TColon)
	value := p.parseBinding(parseBindingOpts{})

	var defaultValueOrNil js_ast.Expr
	if p.lexer.Token == js_lexer.TEquals {
		p.lexer.Next()
		defaultValueOrNil = p.parseExpr(js_ast.LComma)
	}

	return js_ast.PropertyBinding{
		Loc:               loc,
		IsComputed:        isComputed,
		PreferQuotedKey:   preferQuotedKey,
		Key:               key,
		Value:             value,
		DefaultValueOrNil: defaultValueOrNil,
		CloseBracketLoc:   closeBracketLoc,
	}
}

// These properties have special semantics in JavaScript. They must not be
// mangled or we could potentially fail to parse valid JavaScript syntax or
// generate invalid JavaScript syntax as output.
//
// This list is only intended to contain properties specific to the JavaScript
// language itself to avoid syntax errors in the generated output. It's not
// intended to contain properties for JavaScript APIs. Those must be provided
// by the user.
var permanentReservedProps = map[string]bool{
	"__proto__":   true,
	"constructor": true,
	"prototype":   true,
}

func (p *parser) isMangledProp(name string) bool {
	if p.options.mangleProps == nil {
		return false
	}
	if p.options.mangleProps.MatchString(name) && !permanentReservedProps[name] && (p.options.reserveProps == nil || !p.options.reserveProps.MatchString(name)) {
		return true
	}
	reservedProps := p.reservedProps
	if reservedProps == nil {
		reservedProps = make(map[string]bool)
		p.reservedProps = reservedProps
	}
	reservedProps[name] = true
	return false
}

func (p *parser) symbolForMangledProp(name string) ast.Ref {
	mangledProps := p.mangledProps
	if mangledProps == nil {
		mangledProps = make(map[string]ast.Ref)
		p.mangledProps = mangledProps
	}
	ref, ok := mangledProps[name]
	if !ok {
		ref = p.newSymbol(ast.SymbolMangledProp, name)
		mangledProps[name] = ref
	}
	if !p.isControlFlowDead {
		p.symbols[ref.InnerIndex].UseCountEstimate++
	}
	return ref
}

type wasOriginallyDotOrIndex uint8

const (
	wasOriginallyDot wasOriginallyDotOrIndex = iota
	wasOriginallyIndex
)

func (p *parser) dotOrMangledPropParse(
	target js_ast.Expr,
	name js_lexer.MaybeSubstring,
	nameLoc logger.Loc,
	optionalChain js_ast.OptionalChain,
	original wasOriginallyDotOrIndex,
) js_ast.E {
	if (original != wasOriginallyIndex || p.options.mangleQuoted) && p.isMangledProp(name.String) {
		return &js_ast.EIndex{
			Target:        target,
			Index:         js_ast.Expr{Loc: nameLoc, Data: &js_ast.ENameOfSymbol{Ref: p.storeNameInRef(name)}},
			OptionalChain: optionalChain,
		}
	}

	return &js_ast.EDot{
		Target:        target,
		Name:          name.String,
		NameLoc:       nameLoc,
		OptionalChain: optionalChain,
	}
}

func (p *parser) dotOrMangledPropVisit(target js_ast.Expr, name string, nameLoc logger.Loc) js_ast.E {
	if p.isMangledProp(name) {
		return &js_ast.EIndex{
			Target: target,
			Index:  js_ast.Expr{Loc: nameLoc, Data: &js_ast.ENameOfSymbol{Ref: p.symbolForMangledProp(name)}},
		}
	}

	return &js_ast.EDot{
		Target:  target,
		Name:    name,
		NameLoc: nameLoc,
	}
}

func (p *parser) parseArrowBody(args []js_ast.Arg, data fnOrArrowDataParse) *js_ast.EArrow {
	arrowLoc := p.lexer.Loc()

	// Newlines are not allowed before "=>"
	if p.lexer.HasNewlineBefore {
		p.log.AddError(&p.tracker, p.lexer.Range(), "Unexpected newline before \"=>\"")
		panic(js_lexer.LexerPanic{})
	}

	p.lexer.Expect(js_lexer.TEqualsGreaterThan)

	for _, arg := range args {
		p.declareBinding(ast.SymbolHoisted, arg.Binding, parseStmtOpts{})
	}

	// The ability to use "this" and "super" is inherited by arrow functions
	data.isThisDisallowed = p.fnOrArrowDataParse.isThisDisallowed
	data.allowSuperCall = p.fnOrArrowDataParse.allowSuperCall
	data.allowSuperProperty = p.fnOrArrowDataParse.allowSuperProperty

	if p.lexer.Token == js_lexer.TOpenBrace {
		body := p.parseFnBody(data)
		p.afterArrowBodyLoc = p.lexer.Loc()
		return &js_ast.EArrow{Args: args, Body: body}
	}

	p.pushScopeForParsePass(js_ast.ScopeFunctionBody, arrowLoc)
	defer p.popScope()

	oldFnOrArrowData := p.fnOrArrowDataParse
	p.fnOrArrowDataParse = data
	expr := p.parseExpr(js_ast.LComma)
	p.fnOrArrowDataParse = oldFnOrArrowData
	return &js_ast.EArrow{
		Args:       args,
		PreferExpr: true,
		Body:       js_ast.FnBody{Loc: arrowLoc, Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: expr.Loc, Data: &js_ast.SReturn{ValueOrNil: expr}}}}},
	}
}

func (p *parser) checkForArrowAfterTheCurrentToken() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.lexer.Next()
	isArrowAfterThisToken := p.lexer.Token == js_lexer.TEqualsGreaterThan

	p.lexer = oldLexer
	return isArrowAfterThisToken
}

// This parses an expression. This assumes we've already parsed the "async"
// keyword and are currently looking at the following token.
func (p *parser) parseAsyncPrefixExpr(asyncRange logger.Range, level js_ast.L, flags exprFlag) js_ast.Expr {
	// "async function() {}"
	if !p.lexer.HasNewlineBefore && p.lexer.Token == js_lexer.TFunction {
		return p.parseFnExpr(asyncRange.Loc, true /* isAsync */, asyncRange)
	}

	// Check the precedence level to avoid parsing an arrow function in
	// "new async () => {}". This also avoids parsing "new async()" as
	// "new (async())()" instead.
	if !p.lexer.HasNewlineBefore && level < js_ast.LMember {
		switch p.lexer.Token {
		// "async => {}"
		case js_lexer.TEqualsGreaterThan:
			if level <= js_ast.LAssign {
				arg := js_ast.Arg{Binding: js_ast.Binding{Loc: asyncRange.Loc, Data: &js_ast.BIdentifier{
					Ref: p.storeNameInRef(js_lexer.MaybeSubstring{String: "async"})}}}

				p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, asyncRange.Loc)
				defer p.popScope()

				return js_ast.Expr{Loc: asyncRange.Loc, Data: p.parseArrowBody([]js_ast.Arg{arg}, fnOrArrowDataParse{
					needsAsyncLoc: asyncRange.Loc,
				})}
			}

		// "async x => {}"
		case js_lexer.TIdentifier:
			if level <= js_ast.LAssign {
				isArrowFn := true
				if (flags&exprFlagForLoopInit) != 0 && p.lexer.Identifier.String == "of" {
					// See https://github.com/tc39/ecma262/issues/2034 for details

					// "for (async of" is only an arrow function if the next token is "=>"
					isArrowFn = p.checkForArrowAfterTheCurrentToken()

					// Do not allow "for (async of []) ;" but do allow "for await (async of []) ;"
					if !isArrowFn && (flags&exprFlagForAwaitLoopInit) == 0 && p.lexer.Raw() == "of" {
						r := logger.Range{Loc: asyncRange.Loc, Len: p.lexer.Range().End() - asyncRange.Loc.Start}
						p.log.AddError(&p.tracker, r, "For loop initializers cannot start with \"async of\"")
						panic(js_lexer.LexerPanic{})
					}
				} else if p.options.ts.Parse && p.lexer.Token == js_lexer.TIdentifier {
					// Make sure we can parse the following TypeScript code:
					//
					//   export function open(async?: boolean): void {
					//     console.log(async as boolean)
					//   }
					//
					// TypeScript solves this by using a two-token lookahead to check for
					// "=>" after an identifier after the "async". This is done in
					// "isUnParenthesizedAsyncArrowFunctionWorker" which was introduced
					// here: https://github.com/microsoft/TypeScript/pull/8444
					isArrowFn = p.checkForArrowAfterTheCurrentToken()
				}

				if isArrowFn {
					p.markAsyncFn(asyncRange, false)
					ref := p.storeNameInRef(p.lexer.Identifier)
					arg := js_ast.Arg{Binding: js_ast.Binding{Loc: p.lexer.Loc(), Data: &js_ast.BIdentifier{Ref: ref}}}
					p.lexer.Next()

					p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, asyncRange.Loc)
					defer p.popScope()

					arrow := p.parseArrowBody([]js_ast.Arg{arg}, fnOrArrowDataParse{
						needsAsyncLoc: arg.Binding.Loc,
						await:         allowExpr,
					})
					arrow.IsAsync = true
					return js_ast.Expr{Loc: asyncRange.Loc, Data: arrow}
				}
			}

		// "async()"
		// "async () => {}"
		case js_lexer.TOpenParen:
			p.lexer.Next()
			return p.parseParenExpr(asyncRange.Loc, level, parenExprOpts{asyncRange: asyncRange})

		// "async<T>()"
		// "async <T>() => {}"
		case js_lexer.TLessThan:
			if p.options.ts.Parse && (!p.options.jsx.Parse || p.isTSArrowFnJSX()) {
				if result := p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking(); result != didNotSkipAnything {
					p.lexer.Next()
					return p.parseParenExpr(asyncRange.Loc, level, parenExprOpts{
						asyncRange:   asyncRange,
						forceArrowFn: result == definitelyTypeParameters,
					})
				}
			}
		}
	}

	// "async"
	// "async + 1"
	return js_ast.Expr{Loc: asyncRange.Loc, Data: &js_ast.EIdentifier{
		Ref: p.storeNameInRef(js_lexer.MaybeSubstring{String: "async"})}}
}

func (p *parser) parseFnExpr(loc logger.Loc, isAsync bool, asyncRange logger.Range) js_ast.Expr {
	p.lexer.Next()
	isGenerator := p.lexer.Token == js_lexer.TAsterisk
	hasError := false
	if isAsync {
		hasError = p.markAsyncFn(asyncRange, isGenerator)
	}
	if isGenerator {
		if !hasError {
			p.markSyntaxFeature(compat.Generator, p.lexer.Range())
		}
		p.lexer.Next()
	}
	var name *ast.LocRef

	p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, loc)
	defer p.popScope()

	// The name is optional
	if p.lexer.Token == js_lexer.TIdentifier {
		// Don't declare the name "arguments" since it's shadowed and inaccessible
		name = &ast.LocRef{Loc: p.lexer.Loc()}
		if text := p.lexer.Identifier.String; text != "arguments" {
			name.Ref = p.declareSymbol(ast.SymbolHoistedFunction, name.Loc, text)
		} else {
			name.Ref = p.newSymbol(ast.SymbolHoistedFunction, text)
		}
		p.lexer.Next()
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters(allowConstModifier)
	}

	await := allowIdent
	yield := allowIdent
	if isAsync {
		await = allowExpr
	}
	if isGenerator {
		yield = allowExpr
	}

	fn, _ := p.parseFn(name, logger.Range{}, 0, fnOrArrowDataParse{
		needsAsyncLoc: loc,
		asyncRange:    asyncRange,
		await:         await,
		yield:         yield,
	})
	p.validateFunctionName(fn, fnExpr)
	return js_ast.Expr{Loc: loc, Data: &js_ast.EFunction{Fn: fn}}
}

type parenExprOpts struct {
	asyncRange   logger.Range
	forceArrowFn bool
}

// This assumes that the open parenthesis has already been parsed by the caller
func (p *parser) parseParenExpr(loc logger.Loc, level js_ast.L, opts parenExprOpts) js_ast.Expr {
	items := []js_ast.Expr{}
	errors := deferredErrors{}
	arrowArgErrors := deferredArrowArgErrors{}
	spreadRange := logger.Range{}
	typeColonRange := logger.Range{}
	commaAfterSpread := logger.Loc{}
	isAsync := opts.asyncRange.Len > 0

	// Push a scope assuming this is an arrow function. It may not be, in which
	// case we'll need to roll this change back. This has to be done ahead of
	// parsing the arguments instead of later on when we hit the "=>" token and
	// we know it's an arrow function because the arguments may have default
	// values that introduce new scopes and declare new symbols. If this is an
	// arrow function, then those new scopes will need to be parented under the
	// scope of the arrow function itself.
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, loc)

	// Allow "in" inside parentheses
	oldAllowIn := p.allowIn
	p.allowIn = true

	// Forbid "await" and "yield", but only for arrow functions
	oldFnOrArrowData := p.fnOrArrowDataParse
	p.fnOrArrowDataParse.arrowArgErrors = &arrowArgErrors

	// Scan over the comma-separated arguments or expressions
	for p.lexer.Token != js_lexer.TCloseParen {
		itemLoc := p.lexer.Loc()
		isSpread := p.lexer.Token == js_lexer.TDotDotDot

		if isSpread {
			spreadRange = p.lexer.Range()
			p.markSyntaxFeature(compat.RestArgument, spreadRange)
			p.lexer.Next()
		}

		// We don't know yet whether these are arguments or expressions, so parse
		// a superset of the expression syntax. Errors about things that are valid
		// in one but not in the other are deferred.
		p.latestArrowArgLoc = p.lexer.Loc()
		item := p.parseExprOrBindings(js_ast.LComma, &errors)

		if isSpread {
			item = js_ast.Expr{Loc: itemLoc, Data: &js_ast.ESpread{Value: item}}
		}

		// Skip over types
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon {
			typeColonRange = p.lexer.Range()
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)
		}

		// There may be a "=" after the type (but not after an "as" cast)
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TEquals && p.lexer.Loc() != p.forbidSuffixAfterAsLoc {
			p.lexer.Next()
			item = js_ast.Assign(item, p.parseExpr(js_ast.LComma))
		}

		items = append(items, item)
		if p.lexer.Token != js_lexer.TComma {
			break
		}

		// Spread arguments must come last. If there's a spread argument followed
		// by a comma, throw an error if we use these expressions as bindings.
		if isSpread {
			commaAfterSpread = p.lexer.Loc()
		}

		// Eat the comma token
		p.lexer.Next()
	}

	// The parenthetical construct must end with a close parenthesis
	p.lexer.Expect(js_lexer.TCloseParen)

	// Restore "in" operator status before we parse the arrow function body
	p.allowIn = oldAllowIn

	// Also restore "await" and "yield" expression errors
	p.fnOrArrowDataParse = oldFnOrArrowData

	// Are these arguments to an arrow function?
	if p.lexer.Token == js_lexer.TEqualsGreaterThan || opts.forceArrowFn || (p.options.ts.Parse && p.lexer.Token == js_lexer.TColon) {
		// Arrow functions are not allowed inside certain expressions
		if level > js_ast.LAssign {
			p.lexer.Unexpected()
		}

		var invalidLog invalidLog
		args := []js_ast.Arg{}

		if isAsync {
			p.markAsyncFn(opts.asyncRange, false)
		}

		// First, try converting the expressions to bindings
		for _, item := range items {
			isSpread := false
			if spread, ok := item.Data.(*js_ast.ESpread); ok {
				item = spread.Value
				isSpread = true
			}
			binding, initializerOrNil, log := p.convertExprToBindingAndInitializer(item, invalidLog, isSpread)
			invalidLog = log
			args = append(args, js_ast.Arg{Binding: binding, DefaultOrNil: initializerOrNil})
		}

		// Avoid parsing TypeScript code like "a ? (1 + 2) : (3 + 4)" as an arrow
		// function. The ":" after the ")" may be a return type annotation, so we
		// attempt to convert the expressions to bindings first before deciding
		// whether this is an arrow function, and only pick an arrow function if
		// there were no conversion errors.
		if p.lexer.Token == js_lexer.TEqualsGreaterThan || (len(invalidLog.invalidTokens) == 0 &&
			p.trySkipTypeScriptArrowReturnTypeWithBacktracking()) || opts.forceArrowFn {
			if commaAfterSpread.Start != 0 {
				p.log.AddError(&p.tracker, logger.Range{Loc: commaAfterSpread, Len: 1}, "Unexpected \",\" after rest pattern")
			}
			p.logArrowArgErrors(&arrowArgErrors)
			p.logDeferredArrowArgErrors(&errors)

			// Now that we've decided we're an arrow function, report binding pattern
			// conversion errors
			if len(invalidLog.invalidTokens) > 0 {
				for _, token := range invalidLog.invalidTokens {
					p.log.AddError(&p.tracker, token, "Invalid binding pattern")
				}
				panic(js_lexer.LexerPanic{})
			}

			// Also report syntax features used in bindings
			for _, entry := range invalidLog.syntaxFeatures {
				p.markSyntaxFeature(entry.feature, entry.token)
			}

			await := allowIdent
			if isAsync {
				await = allowExpr
			}

			arrow := p.parseArrowBody(args, fnOrArrowDataParse{
				needsAsyncLoc: loc,
				await:         await,
			})
			arrow.IsAsync = isAsync
			arrow.HasRestArg = spreadRange.Len > 0
			p.popScope()
			return js_ast.Expr{Loc: loc, Data: arrow}
		}
	}

	// If we get here, it's not an arrow function so undo the pushing of the
	// scope we did earlier. This needs to flatten any child scopes into the
	// parent scope as if the scope was never pushed in the first place.
	p.popAndFlattenScope(scopeIndex)

	// If this isn't an arrow function, then types aren't allowed
	if typeColonRange.Len > 0 {
		p.log.AddError(&p.tracker, typeColonRange, "Unexpected \":\"")
		panic(js_lexer.LexerPanic{})
	}

	// Are these arguments for a call to a function named "async"?
	if isAsync {
		p.logExprErrors(&errors)
		async := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{
			Ref: p.storeNameInRef(js_lexer.MaybeSubstring{String: "async"})}}
		return js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
			Target: async,
			Args:   items,
		}}
	}

	// Is this a chain of expressions and comma operators?
	if len(items) > 0 {
		p.logExprErrors(&errors)
		if spreadRange.Len > 0 {
			p.log.AddError(&p.tracker, spreadRange, "Unexpected \"...\"")
			panic(js_lexer.LexerPanic{})
		}
		value := js_ast.JoinAllWithComma(items)
		p.markExprAsParenthesized(value, loc, isAsync)
		return value
	}

	// Indicate that we expected an arrow function
	p.lexer.Expected(js_lexer.TEqualsGreaterThan)
	return js_ast.Expr{}
}

type invalidLog struct {
	invalidTokens  []logger.Range
	syntaxFeatures []syntaxFeature
}

type syntaxFeature struct {
	feature compat.JSFeature
	token   logger.Range
}

func (p *parser) convertExprToBindingAndInitializer(
	expr js_ast.Expr, invalidLog invalidLog, isSpread bool,
) (js_ast.Binding, js_ast.Expr, invalidLog) {
	var initializerOrNil js_ast.Expr
	if assign, ok := expr.Data.(*js_ast.EBinary); ok && assign.Op == js_ast.BinOpAssign {
		initializerOrNil = assign.Right
		expr = assign.Left
	}
	binding, invalidLog := p.convertExprToBinding(expr, invalidLog)
	if initializerOrNil.Data != nil {
		equalsRange := p.source.RangeOfOperatorBefore(initializerOrNil.Loc, "=")
		if isSpread {
			p.log.AddError(&p.tracker, equalsRange, "A rest argument cannot have a default initializer")
		} else {
			invalidLog.syntaxFeatures = append(invalidLog.syntaxFeatures, syntaxFeature{
				feature: compat.DefaultArgument,
				token:   equalsRange,
			})
		}
	}
	return binding, initializerOrNil, invalidLog
}

// Note: do not write to "p.log" in this function. Any errors due to conversion
// from expression to binding should be written to "invalidLog" instead. That
// way we can potentially keep this as an expression if it turns out it's not
// needed as a binding after all.
func (p *parser) convertExprToBinding(expr js_ast.Expr, invalidLog invalidLog) (js_ast.Binding, invalidLog) {
	switch e := expr.Data.(type) {
	case *js_ast.EMissing:
		return js_ast.Binding{Loc: expr.Loc, Data: js_ast.BMissingShared}, invalidLog

	case *js_ast.EIdentifier:
		return js_ast.Binding{Loc: expr.Loc, Data: &js_ast.BIdentifier{Ref: e.Ref}}, invalidLog

	case *js_ast.EArray:
		if e.CommaAfterSpread.Start != 0 {
			invalidLog.invalidTokens = append(invalidLog.invalidTokens, logger.Range{Loc: e.CommaAfterSpread, Len: 1})
		}
		invalidLog.syntaxFeatures = append(invalidLog.syntaxFeatures,
			syntaxFeature{feature: compat.Destructuring, token: p.source.RangeOfOperatorAfter(expr.Loc, "[")})
		items := []js_ast.ArrayBinding{}
		isSpread := false
		for _, item := range e.Items {
			if i, ok := item.Data.(*js_ast.ESpread); ok {
				isSpread = true
				item = i.Value
				if _, ok := item.Data.(*js_ast.EIdentifier); !ok {
					p.markSyntaxFeature(compat.NestedRestBinding, p.source.RangeOfOperatorAfter(item.Loc, "["))
				}
			}
			binding, initializerOrNil, log := p.convertExprToBindingAndInitializer(item, invalidLog, isSpread)
			invalidLog = log
			items = append(items, js_ast.ArrayBinding{
				Binding:           binding,
				DefaultValueOrNil: initializerOrNil,
				Loc:               item.Loc,
			})
		}
		return js_ast.Binding{Loc: expr.Loc, Data: &js_ast.BArray{
			Items:           items,
			HasSpread:       isSpread,
			IsSingleLine:    e.IsSingleLine,
			CloseBracketLoc: e.CloseBracketLoc,
		}}, invalidLog

	case *js_ast.EObject:
		if e.CommaAfterSpread.Start != 0 {
			invalidLog.invalidTokens = append(invalidLog.invalidTokens, logger.Range{Loc: e.CommaAfterSpread, Len: 1})
		}
		invalidLog.syntaxFeatures = append(invalidLog.syntaxFeatures,
			syntaxFeature{feature: compat.Destructuring, token: p.source.RangeOfOperatorAfter(expr.Loc, "{")})
		properties := []js_ast.PropertyBinding{}
		for _, property := range e.Properties {
			if property.Kind.IsMethodDefinition() {
				invalidLog.invalidTokens = append(invalidLog.invalidTokens, js_lexer.RangeOfIdentifier(p.source, property.Key.Loc))
				continue
			}
			binding, initializerOrNil, log := p.convertExprToBindingAndInitializer(property.ValueOrNil, invalidLog, false)
			invalidLog = log
			if initializerOrNil.Data == nil {
				initializerOrNil = property.InitializerOrNil
			}
			properties = append(properties, js_ast.PropertyBinding{
				Loc:               property.Loc,
				IsSpread:          property.Kind == js_ast.PropertySpread,
				IsComputed:        property.Flags.Has(js_ast.PropertyIsComputed),
				Key:               property.Key,
				Value:             binding,
				DefaultValueOrNil: initializerOrNil,
			})
		}
		return js_ast.Binding{Loc: expr.Loc, Data: &js_ast.BObject{
			Properties:    properties,
			IsSingleLine:  e.IsSingleLine,
			CloseBraceLoc: e.CloseBraceLoc,
		}}, invalidLog

	default:
		invalidLog.invalidTokens = append(invalidLog.invalidTokens, logger.Range{Loc: expr.Loc})
		return js_ast.Binding{}, invalidLog
	}
}

func (p *parser) saveExprCommentsHere() logger.Loc {
	loc := p.lexer.Loc()
	if p.exprComments != nil && len(p.lexer.CommentsBeforeToken) > 0 {
		comments := make([]string, len(p.lexer.CommentsBeforeToken))
		for i, comment := range p.lexer.CommentsBeforeToken {
			comments[i] = p.source.CommentTextWithoutIndent(comment)
		}
		p.exprComments[loc] = comments
		p.lexer.CommentsBeforeToken = p.lexer.CommentsBeforeToken[0:]
	}
	return loc
}

type exprFlag uint8

const (
	exprFlagDecorator exprFlag = 1 << iota
	exprFlagForLoopInit
	exprFlagForAwaitLoopInit
)

func (p *parser) parsePrefix(level js_ast.L, errors *deferredErrors, flags exprFlag) js_ast.Expr {
	loc := p.saveExprCommentsHere()

	switch p.lexer.Token {
	case js_lexer.TSuper:
		superRange := p.lexer.Range()
		p.lexer.Next()

		switch p.lexer.Token {
		case js_lexer.TOpenParen:
			if level < js_ast.LCall && p.fnOrArrowDataParse.allowSuperCall {
				return js_ast.Expr{Loc: loc, Data: js_ast.ESuperShared}
			}

		case js_lexer.TDot, js_lexer.TOpenBracket:
			if p.fnOrArrowDataParse.allowSuperProperty {
				return js_ast.Expr{Loc: loc, Data: js_ast.ESuperShared}
			}
		}

		p.log.AddError(&p.tracker, superRange, "Unexpected \"super\"")
		return js_ast.Expr{Loc: loc, Data: js_ast.ESuperShared}

	case js_lexer.TOpenParen:
		if errors != nil {
			errors.invalidParens = append(errors.invalidParens, p.lexer.Range())
		}

		p.lexer.Next()

		// Arrow functions aren't allowed in the middle of expressions
		if level > js_ast.LAssign {
			// Allow "in" inside parentheses
			oldAllowIn := p.allowIn
			p.allowIn = true

			value := p.parseExpr(js_ast.LLowest)
			p.markExprAsParenthesized(value, loc, false)
			p.lexer.Expect(js_lexer.TCloseParen)

			p.allowIn = oldAllowIn
			return value
		}

		value := p.parseParenExpr(loc, level, parenExprOpts{})
		return value

	case js_lexer.TFalse:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: false}}

	case js_lexer.TTrue:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: true}}

	case js_lexer.TNull:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: js_ast.ENullShared}

	case js_lexer.TThis:
		if p.fnOrArrowDataParse.isThisDisallowed {
			p.log.AddError(&p.tracker, p.lexer.Range(), "Cannot use \"this\" here:")
		}
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}

	case js_lexer.TPrivateIdentifier:
		if !p.allowPrivateIdentifiers || !p.allowIn || level >= js_ast.LCompare {
			p.lexer.Unexpected()
		}

		name := p.lexer.Identifier
		p.lexer.Next()

		// Check for "#foo in bar"
		if p.lexer.Token != js_lexer.TIn {
			p.lexer.Expected(js_lexer.TIn)
		}

		// Make sure to lower all matching private names
		if p.options.unsupportedJSFeatures.Has(compat.ClassPrivateBrandCheck) {
			if p.lowerAllOfThesePrivateNames == nil {
				p.lowerAllOfThesePrivateNames = make(map[string]bool)
			}
			p.lowerAllOfThesePrivateNames[name.String] = true
		}

		return js_ast.Expr{Loc: loc, Data: &js_ast.EPrivateIdentifier{Ref: p.storeNameInRef(name)}}

	case js_lexer.TIdentifier:
		name := p.lexer.Identifier
		nameRange := p.lexer.Range()
		raw := p.lexer.Raw()
		p.lexer.Next()

		// Handle async and await expressions
		switch name.String {
		case "async":
			if raw == "async" {
				return p.parseAsyncPrefixExpr(nameRange, level, flags)
			}

		case "await":
			switch p.fnOrArrowDataParse.await {
			case forbidAll:
				p.log.AddError(&p.tracker, nameRange, "The keyword \"await\" cannot be used here:")

			case allowExpr:
				if raw != "await" {
					p.log.AddError(&p.tracker, nameRange, "The keyword \"await\" cannot be escaped")
				} else {
					if p.fnOrArrowDataParse.isTopLevel {
						p.topLevelAwaitKeyword = nameRange
					}
					if p.fnOrArrowDataParse.arrowArgErrors != nil {
						p.fnOrArrowDataParse.arrowArgErrors.invalidExprAwait = nameRange
					}
					value := p.parseExpr(js_ast.LPrefix)
					if p.lexer.Token == js_lexer.TAsteriskAsterisk {
						p.lexer.Unexpected()
					}
					return js_ast.Expr{Loc: loc, Data: &js_ast.EAwait{Value: value}}
				}

			case allowIdent:
				p.lexer.PrevTokenWasAwaitKeyword = true
				p.lexer.AwaitKeywordLoc = loc
				p.lexer.FnOrArrowStartLoc = p.fnOrArrowDataParse.needsAsyncLoc
			}

		case "yield":
			switch p.fnOrArrowDataParse.yield {
			case forbidAll:
				p.log.AddError(&p.tracker, nameRange, "The keyword \"yield\" cannot be used here:")

			case allowExpr:
				if raw != "yield" {
					p.log.AddError(&p.tracker, nameRange, "The keyword \"yield\" cannot be escaped")
				} else {
					if level > js_ast.LAssign {
						p.log.AddError(&p.tracker, nameRange, "Cannot use a \"yield\" expression here without parentheses:")
					}
					if p.fnOrArrowDataParse.arrowArgErrors != nil {
						p.fnOrArrowDataParse.arrowArgErrors.invalidExprYield = nameRange
					}
					return p.parseYieldExpr(loc)
				}

			case allowIdent:
				if !p.lexer.HasNewlineBefore {
					// Try to gracefully recover if "yield" is used in the wrong place
					switch p.lexer.Token {
					case js_lexer.TNull, js_lexer.TIdentifier, js_lexer.TFalse, js_lexer.TTrue,
						js_lexer.TNumericLiteral, js_lexer.TBigIntegerLiteral, js_lexer.TStringLiteral:
						p.log.AddError(&p.tracker, nameRange, "Cannot use \"yield\" outside a generator function")
						return p.parseYieldExpr(loc)
					}
				}
			}
		}

		// Handle the start of an arrow expression
		if p.lexer.Token == js_lexer.TEqualsGreaterThan && level <= js_ast.LAssign {
			ref := p.storeNameInRef(name)
			arg := js_ast.Arg{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ref}}}

			p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, loc)
			defer p.popScope()

			return js_ast.Expr{Loc: loc, Data: p.parseArrowBody([]js_ast.Arg{arg}, fnOrArrowDataParse{
				needsAsyncLoc: loc,
			})}
		}

		ref := p.storeNameInRef(name)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}

	case js_lexer.TStringLiteral, js_lexer.TNoSubstitutionTemplateLiteral:
		return p.parseStringLiteral()

	case js_lexer.TTemplateHead:
		var legacyOctalLoc logger.Loc
		headLoc := p.lexer.Loc()
		head := p.lexer.StringLiteral()
		if p.lexer.LegacyOctalLoc.Start > loc.Start {
			legacyOctalLoc = p.lexer.LegacyOctalLoc
		}
		parts, tailLegacyOctalLoc := p.parseTemplateParts(false /* includeRaw */)
		if tailLegacyOctalLoc.Start > 0 {
			legacyOctalLoc = tailLegacyOctalLoc
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.ETemplate{
			HeadLoc:        headLoc,
			HeadCooked:     head,
			Parts:          parts,
			LegacyOctalLoc: legacyOctalLoc,
		}}

	case js_lexer.TNumericLiteral:
		value := js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: p.lexer.Number}}
		p.checkForLegacyOctalLiteral(value.Data)
		p.lexer.Next()
		return value

	case js_lexer.TBigIntegerLiteral:
		value := p.lexer.Identifier
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBigInt{Value: value.String}}

	case js_lexer.TSlash, js_lexer.TSlashEquals:
		p.lexer.ScanRegExp()
		value := p.lexer.Raw()
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.ERegExp{Value: value}}

	case js_lexer.TVoid:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpVoid, Value: value}}

	case js_lexer.TTypeof:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		_, valueIsIdentifier := value.Data.(*js_ast.EIdentifier)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{
			Op:                            js_ast.UnOpTypeof,
			Value:                         value,
			WasOriginallyTypeofIdentifier: valueIsIdentifier,
		}}

	case js_lexer.TDelete:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		if index, ok := value.Data.(*js_ast.EIndex); ok {
			if private, ok := index.Index.Data.(*js_ast.EPrivateIdentifier); ok {
				name := p.loadNameFromRef(private.Ref)
				r := logger.Range{Loc: index.Index.Loc, Len: int32(len(name))}
				p.log.AddError(&p.tracker, r, fmt.Sprintf("Deleting the private name %q is forbidden", name))
			}
		}
		_, valueIsIdentifier := value.Data.(*js_ast.EIdentifier)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{
			Op:    js_ast.UnOpDelete,
			Value: value,
			WasOriginallyDeleteOfIdentifierOrPropertyAccess: valueIsIdentifier || js_ast.IsPropertyAccess(value),
		}}

	case js_lexer.TPlus:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpPos, Value: value}}

	case js_lexer.TMinus:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpNeg, Value: value}}

	case js_lexer.TTilde:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpCpl, Value: value}}

	case js_lexer.TExclamation:
		p.lexer.Next()
		value := p.parseExpr(js_ast.LPrefix)
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpNot, Value: value}}

	case js_lexer.TMinusMinus:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpPreDec, Value: p.parseExpr(js_ast.LPrefix)}}

	case js_lexer.TPlusPlus:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpPreInc, Value: p.parseExpr(js_ast.LPrefix)}}

	case js_lexer.TFunction:
		return p.parseFnExpr(loc, false /* isAsync */, logger.Range{})

	case js_lexer.TClass:
		return p.parseClassExpr(nil)

	case js_lexer.TAt:
		// Parse decorators before class expressions
		decorators := p.parseDecorators(p.currentScope, logger.Range{}, decoratorBeforeClassExpr)
		return p.parseClassExpr(decorators)

	case js_lexer.TNew:
		p.lexer.Next()

		// Special-case the weird "new.target" expression here
		if p.lexer.Token == js_lexer.TDot {
			p.lexer.Next()
			if p.lexer.Token != js_lexer.TIdentifier || p.lexer.Raw() != "target" {
				p.lexer.Unexpected()
			}
			r := logger.Range{Loc: loc, Len: p.lexer.Range().End() - loc.Start}
			p.markSyntaxFeature(compat.NewTarget, r)
			p.lexer.Next()
			return js_ast.Expr{Loc: loc, Data: &js_ast.ENewTarget{Range: r}}
		}

		target := p.parseExprWithFlags(js_ast.LMember, flags)
		args := []js_ast.Expr{}
		var closeParenLoc logger.Loc
		var isMultiLine bool

		if p.lexer.Token == js_lexer.TOpenParen {
			args, closeParenLoc, isMultiLine = p.parseCallArgs()
		}

		return js_ast.Expr{Loc: loc, Data: &js_ast.ENew{
			Target:        target,
			Args:          args,
			CloseParenLoc: closeParenLoc,
			IsMultiLine:   isMultiLine,
		}}

	case js_lexer.TOpenBracket:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []js_ast.Expr{}
		selfErrors := deferredErrors{}
		commaAfterSpread := logger.Loc{}

		// Allow "in" inside arrays
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != js_lexer.TCloseBracket {
			switch p.lexer.Token {
			case js_lexer.TComma:
				items = append(items, js_ast.Expr{Loc: p.lexer.Loc(), Data: js_ast.EMissingShared})

			case js_lexer.TDotDotDot:
				if errors != nil {
					errors.arraySpreadFeature = p.lexer.Range()
				} else {
					p.markSyntaxFeature(compat.ArraySpread, p.lexer.Range())
				}
				dotsLoc := p.saveExprCommentsHere()
				p.lexer.Next()
				item := p.parseExprOrBindings(js_ast.LComma, &selfErrors)
				items = append(items, js_ast.Expr{Loc: dotsLoc, Data: &js_ast.ESpread{Value: item}})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == js_lexer.TComma {
					commaAfterSpread = p.lexer.Loc()
				}

			default:
				item := p.parseExprOrBindings(js_ast.LComma, &selfErrors)
				items = append(items, item)
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
			p.lexer.Next()
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
		}

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		closeBracketLoc := p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBracket)
		p.allowIn = oldAllowIn

		if p.willNeedBindingPattern() {
			// Is this a binding pattern?
		} else if errors == nil {
			// Is this an expression?
			p.logExprErrors(&selfErrors)
		} else {
			// In this case, we can't distinguish between the two yet
			selfErrors.mergeInto(errors)
		}

		return js_ast.Expr{Loc: loc, Data: &js_ast.EArray{
			Items:            items,
			CommaAfterSpread: commaAfterSpread,
			IsSingleLine:     isSingleLine,
			CloseBracketLoc:  closeBracketLoc,
		}}

	case js_lexer.TOpenBrace:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []js_ast.Property{}
		selfErrors := deferredErrors{}
		commaAfterSpread := logger.Loc{}

		// Allow "in" inside object literals
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != js_lexer.TCloseBrace {
			if p.lexer.Token == js_lexer.TDotDotDot {
				dotLoc := p.saveExprCommentsHere()
				p.lexer.Next()
				value := p.parseExprOrBindings(js_ast.LComma, &selfErrors)
				properties = append(properties, js_ast.Property{
					Kind:       js_ast.PropertySpread,
					Loc:        dotLoc,
					ValueOrNil: value,
				})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == js_lexer.TComma {
					commaAfterSpread = p.lexer.Loc()
				}
			} else {
				// This property may turn out to be a type in TypeScript, which should be ignored
				if property, ok := p.parseProperty(p.saveExprCommentsHere(), js_ast.PropertyField, propertyOpts{}, &selfErrors); ok {
					properties = append(properties, property)
				}
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
			p.lexer.Next()
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
		}

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		closeBraceLoc := p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBrace)
		p.allowIn = oldAllowIn

		if p.willNeedBindingPattern() {
			// Is this a binding pattern?
		} else if errors == nil {
			// Is this an expression?
			p.logExprErrors(&selfErrors)
		} else {
			// In this case, we can't distinguish between the two yet
			selfErrors.mergeInto(errors)
		}

		return js_ast.Expr{Loc: loc, Data: &js_ast.EObject{
			Properties:       properties,
			CommaAfterSpread: commaAfterSpread,
			IsSingleLine:     isSingleLine,
			CloseBraceLoc:    closeBraceLoc,
		}}

	case js_lexer.TLessThan:
		// This is a very complicated and highly ambiguous area of TypeScript
		// syntax. Many similar-looking things are overloaded.
		//
		// TS:
		//
		//   A type cast:
		//     <A>(x)
		//     <[]>(x)
		//     <A[]>(x)
		//     <const>(x)
		//
		//   An arrow function with type parameters:
		//     <A>(x) => {}
		//     <A, B>(x) => {}
		//     <A = B>(x) => {}
		//     <A extends B>(x) => {}
		//     <const A>(x) => {}
		//     <const A extends B>(x) => {}
		//
		//   A syntax error:
		//     <>() => {}
		//
		// TSX:
		//
		//   A JSX element:
		//     <>() => {}</>
		//     <A>(x) => {}</A>
		//     <A extends/>
		//     <A extends>(x) => {}</A>
		//     <A extends={false}>(x) => {}</A>
		//     <const A extends/>
		//     <const A extends>(x) => {}</const>
		//
		//   An arrow function with type parameters:
		//     <A,>(x) => {}
		//     <A, B>(x) => {}
		//     <A = B>(x) => {}
		//     <A extends B>(x) => {}
		//     <const>(x)</const>
		//     <const A extends B>(x) => {}
		//
		//   A syntax error:
		//     <[]>(x)
		//     <A[]>(x)
		//     <>() => {}
		//     <A>(x) => {}

		if p.options.ts.Parse && p.options.jsx.Parse && p.isTSArrowFnJSX() {
			p.skipTypeScriptTypeParameters(allowConstModifier)
			p.lexer.Expect(js_lexer.TOpenParen)
			return p.parseParenExpr(loc, level, parenExprOpts{forceArrowFn: true})
		}

		// Print a friendly error message when parsing JSX as JavaScript
		if !p.options.jsx.Parse && !p.options.ts.Parse {
			var how string
			switch logger.API {
			case logger.CLIAPI:
				how = " You can use \"--loader:.js=jsx\" to do that."
			case logger.JSAPI:
				how = " You can use \"loader: { '.js': 'jsx' }\" to do that."
			case logger.GoAPI:
				how = " You can use 'Loader: map[string]api.Loader{\".js\": api.LoaderJSX}' to do that."
			}
			p.log.AddErrorWithNotes(&p.tracker, p.lexer.Range(), "The JSX syntax extension is not currently enabled", []logger.MsgData{{
				Text: "The esbuild loader for this file is currently set to \"js\" but it must be set to \"jsx\" to be able to parse JSX syntax." + how}})
			p.options.jsx.Parse = true
		}

		if p.options.jsx.Parse {
			// Use NextInsideJSXElement() instead of Next() so we parse "<<" as "<"
			p.lexer.NextInsideJSXElement()
			element := p.parseJSXElement(loc)

			// The call to parseJSXElement() above doesn't consume the last
			// TGreaterThan because the caller knows what Next() function to call.
			// Use Next() instead of NextInsideJSXElement() here since the next
			// token is an expression.
			p.lexer.Next()
			return element
		}

		if p.options.ts.Parse {
			// This is either an old-style type cast or a generic lambda function

			// TypeScript 4.5 introduced the ".mts" and ".cts" extensions that forbid
			// the use of an expression starting with "<" that would be ambiguous
			// when the file is in JSX mode.
			if p.options.ts.NoAmbiguousLessThan && !p.isTSArrowFnJSX() {
				p.log.AddError(&p.tracker, p.lexer.Range(),
					"This syntax is not allowed in files with the \".mts\" or \".cts\" extension")
			}

			// "<T>(x)"
			// "<T>(x) => {}"
			if result := p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking(); result != didNotSkipAnything {
				p.lexer.Expect(js_lexer.TOpenParen)
				return p.parseParenExpr(loc, level, parenExprOpts{
					forceArrowFn: result == definitelyTypeParameters,
				})
			}

			// "<T>x"
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)
			p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)
			value := p.parsePrefix(level, errors, flags)
			return value
		}

		p.lexer.Unexpected()
		return js_ast.Expr{}

	case js_lexer.TImport:
		p.lexer.Next()
		return p.parseImportExpr(loc, level)

	default:
		p.lexer.Unexpected()
		return js_ast.Expr{}
	}
}

func (p *parser) parseYieldExpr(loc logger.Loc) js_ast.Expr {
	// Parse a yield-from expression, which yields from an iterator
	isStar := p.lexer.Token == js_lexer.TAsterisk
	if isStar && !p.lexer.HasNewlineBefore {
		p.lexer.Next()
	}

	var valueOrNil js_ast.Expr

	// The yield expression only has a value in certain cases
	if isStar {
		valueOrNil = p.parseExpr(js_ast.LYield)
	} else {
		switch p.lexer.Token {
		case js_lexer.TCloseBrace, js_lexer.TCloseBracket, js_lexer.TCloseParen,
			js_lexer.TColon, js_lexer.TComma, js_lexer.TSemicolon:

		default:
			if !p.lexer.HasNewlineBefore {
				valueOrNil = p.parseExpr(js_ast.LYield)
			}
		}
	}

	return js_ast.Expr{Loc: loc, Data: &js_ast.EYield{ValueOrNil: valueOrNil, IsStar: isStar}}
}

func (p *parser) willNeedBindingPattern() bool {
	switch p.lexer.Token {
	case js_lexer.TEquals:
		// "[a] = b;"
		return true

	case js_lexer.TIn:
		// "for ([a] in b) {}"
		return !p.allowIn

	case js_lexer.TIdentifier:
		// "for ([a] of b) {}"
		return !p.allowIn && p.lexer.IsContextualKeyword("of")

	default:
		return false
	}
}

// Note: The caller has already parsed the "import" keyword
func (p *parser) parseImportExpr(loc logger.Loc, level js_ast.L) js_ast.Expr {
	// Parse an "import.meta" expression
	if p.lexer.Token == js_lexer.TDot {
		p.lexer.Next()
		if !p.lexer.IsContextualKeyword("meta") {
			p.lexer.ExpectedString("\"meta\"")
		}
		p.esmImportMeta = logger.Range{Loc: loc, Len: p.lexer.Range().End() - loc.Start}
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EImportMeta{RangeLen: p.esmImportMeta.Len}}
	}

	if level > js_ast.LCall {
		r := js_lexer.RangeOfIdentifier(p.source, loc)
		p.log.AddError(&p.tracker, r, "Cannot use an \"import\" expression here without parentheses:")
	}

	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	p.lexer.Expect(js_lexer.TOpenParen)

	value := p.parseExpr(js_ast.LComma)
	var optionsOrNil js_ast.Expr

	if p.lexer.Token == js_lexer.TComma {
		// "import('./foo.json', )"
		p.lexer.Next()

		if p.lexer.Token != js_lexer.TCloseParen {
			// "import('./foo.json', { assert: { type: 'json' } })"
			optionsOrNil = p.parseExpr(js_ast.LComma)

			if p.lexer.Token == js_lexer.TComma {
				// "import('./foo.json', { assert: { type: 'json' } }, )"
				p.lexer.Next()
			}
		}
	}

	closeParenLoc := p.saveExprCommentsHere()
	p.lexer.Expect(js_lexer.TCloseParen)

	p.allowIn = oldAllowIn
	return js_ast.Expr{Loc: loc, Data: &js_ast.EImportCall{
		Expr:          value,
		OptionsOrNil:  optionsOrNil,
		CloseParenLoc: closeParenLoc,
	}}
}

func (p *parser) parseExprOrBindings(level js_ast.L, errors *deferredErrors) js_ast.Expr {
	return p.parseExprCommon(level, errors, 0)
}

func (p *parser) parseExpr(level js_ast.L) js_ast.Expr {
	return p.parseExprCommon(level, nil, 0)
}

func (p *parser) parseExprWithFlags(level js_ast.L, flags exprFlag) js_ast.Expr {
	return p.parseExprCommon(level, nil, flags)
}

func (p *parser) parseExprCommon(level js_ast.L, errors *deferredErrors, flags exprFlag) js_ast.Expr {
	lexerCommentFlags := p.lexer.HasCommentBefore
	expr := p.parsePrefix(level, errors, flags)

	if (lexerCommentFlags&(js_lexer.PureCommentBefore|js_lexer.NoSideEffectsCommentBefore)) != 0 && !p.options.ignoreDCEAnnotations {
		if (lexerCommentFlags & js_lexer.NoSideEffectsCommentBefore) != 0 {
			switch e := expr.Data.(type) {
			case *js_ast.EArrow:
				e.HasNoSideEffectsComment = true
			case *js_ast.EFunction:
				e.Fn.HasNoSideEffectsComment = true
			}
		}

		// There is no formal spec for "__PURE__" comments but from reverse-
		// engineering, it looks like they apply to the next CallExpression or
		// NewExpression. So in "/* @__PURE__ */ a().b() + c()" the comment applies
		// to the expression "a().b()".
		if (lexerCommentFlags&js_lexer.PureCommentBefore) != 0 && level < js_ast.LCall {
			expr = p.parseSuffix(expr, js_ast.LCall-1, errors, flags)
			switch e := expr.Data.(type) {
			case *js_ast.ECall:
				e.CanBeUnwrappedIfUnused = true
			case *js_ast.ENew:
				e.CanBeUnwrappedIfUnused = true
			}
		}
	}

	return p.parseSuffix(expr, level, errors, flags)
}

func (p *parser) parseSuffix(left js_ast.Expr, level js_ast.L, errors *deferredErrors, flags exprFlag) js_ast.Expr {
	optionalChain := js_ast.OptionalChainNone

	for {
		if p.lexer.Loc() == p.afterArrowBodyLoc {
			for {
				switch p.lexer.Token {
				case js_lexer.TComma:
					if level >= js_ast.LComma {
						return left
					}
					p.lexer.Next()
					left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpComma, Left: left, Right: p.parseExpr(js_ast.LComma)}}

				default:
					return left
				}
			}
		}

		// Stop now if this token is forbidden to follow a TypeScript "as" cast
		if p.lexer.Loc() == p.forbidSuffixAfterAsLoc {
			return left
		}

		// Reset the optional chain flag by default. That way we won't accidentally
		// treat "c.d" as OptionalChainContinue in "a?.b + c.d".
		oldOptionalChain := optionalChain
		optionalChain = js_ast.OptionalChainNone

		switch p.lexer.Token {
		case js_lexer.TDot:
			p.lexer.Next()

			if p.lexer.Token == js_lexer.TPrivateIdentifier && p.allowPrivateIdentifiers {
				// "a.#b"
				// "a?.b.#c"
				if _, ok := left.Data.(*js_ast.ESuper); ok {
					p.lexer.Expected(js_lexer.TIdentifier)
				}
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				p.reportPrivateNameUsage(name.String)
				p.lexer.Next()
				ref := p.storeNameInRef(name)
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIndex{
					Target:        left,
					Index:         js_ast.Expr{Loc: nameLoc, Data: &js_ast.EPrivateIdentifier{Ref: ref}},
					OptionalChain: oldOptionalChain,
				}}
			} else {
				// "a.b"
				// "a?.b.c"
				if !p.lexer.IsIdentifierOrKeyword() {
					p.lexer.Expect(js_lexer.TIdentifier)
				}
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				p.lexer.Next()
				left = js_ast.Expr{Loc: left.Loc, Data: p.dotOrMangledPropParse(left, name, nameLoc, oldOptionalChain, wasOriginallyDot)}
			}

			optionalChain = oldOptionalChain

		case js_lexer.TQuestionDot:
			p.lexer.Next()
			optionalStart := js_ast.OptionalChainStart

			// Remove unnecessary optional chains
			if p.options.minifySyntax {
				if isNullOrUndefined, _, ok := js_ast.ToNullOrUndefinedWithSideEffects(left.Data); ok && !isNullOrUndefined {
					optionalStart = js_ast.OptionalChainNone
				}
			}

			switch p.lexer.Token {
			case js_lexer.TOpenBracket:
				// "a?.[b]"
				p.lexer.Next()

				// Allow "in" inside the brackets
				oldAllowIn := p.allowIn
				p.allowIn = true

				index := p.parseExpr(js_ast.LLowest)

				p.allowIn = oldAllowIn

				closeBracketLoc := p.saveExprCommentsHere()
				p.lexer.Expect(js_lexer.TCloseBracket)
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIndex{
					Target:          left,
					Index:           index,
					OptionalChain:   optionalStart,
					CloseBracketLoc: closeBracketLoc,
				}}

			case js_lexer.TOpenParen:
				// "a?.()"
				if level >= js_ast.LCall {
					return left
				}
				kind := js_ast.NormalCall
				if js_ast.IsPropertyAccess(left) {
					kind = js_ast.TargetWasOriginallyPropertyAccess
				}
				args, closeParenLoc, isMultiLine := p.parseCallArgs()
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ECall{
					Target:        left,
					Args:          args,
					CloseParenLoc: closeParenLoc,
					OptionalChain: optionalStart,
					IsMultiLine:   isMultiLine,
					Kind:          kind,
				}}

			case js_lexer.TLessThan, js_lexer.TLessThanLessThan:
				// "a?.<T>()"
				// "a?.<<T>() => T>()"
				if !p.options.ts.Parse {
					p.lexer.Expected(js_lexer.TIdentifier)
				}
				p.skipTypeScriptTypeArguments(skipTypeScriptTypeArgumentsOpts{})
				if p.lexer.Token != js_lexer.TOpenParen {
					p.lexer.Expected(js_lexer.TOpenParen)
				}
				if level >= js_ast.LCall {
					return left
				}
				kind := js_ast.NormalCall
				if js_ast.IsPropertyAccess(left) {
					kind = js_ast.TargetWasOriginallyPropertyAccess
				}
				args, closeParenLoc, isMultiLine := p.parseCallArgs()
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ECall{
					Target:        left,
					Args:          args,
					CloseParenLoc: closeParenLoc,
					OptionalChain: optionalStart,
					IsMultiLine:   isMultiLine,
					Kind:          kind,
				}}

			default:
				if p.lexer.Token == js_lexer.TPrivateIdentifier && p.allowPrivateIdentifiers {
					// "a?.#b"
					name := p.lexer.Identifier
					nameLoc := p.lexer.Loc()
					p.reportPrivateNameUsage(name.String)
					p.lexer.Next()
					ref := p.storeNameInRef(name)
					left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIndex{
						Target:        left,
						Index:         js_ast.Expr{Loc: nameLoc, Data: &js_ast.EPrivateIdentifier{Ref: ref}},
						OptionalChain: optionalStart,
					}}
				} else {
					// "a?.b"
					if !p.lexer.IsIdentifierOrKeyword() {
						p.lexer.Expect(js_lexer.TIdentifier)
					}
					name := p.lexer.Identifier
					nameLoc := p.lexer.Loc()
					p.lexer.Next()
					left = js_ast.Expr{Loc: left.Loc, Data: p.dotOrMangledPropParse(left, name, nameLoc, optionalStart, wasOriginallyDot)}
				}
			}

			// Only continue if we have started
			if optionalStart == js_ast.OptionalChainStart {
				optionalChain = js_ast.OptionalChainContinue
			}

		case js_lexer.TNoSubstitutionTemplateLiteral:
			if oldOptionalChain != js_ast.OptionalChainNone {
				p.log.AddError(&p.tracker, p.lexer.Range(), "Template literals cannot have an optional chain as a tag")
			}
			headLoc := p.lexer.Loc()
			headCooked, headRaw := p.lexer.CookedAndRawTemplateContents()
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
				TagOrNil:                       left,
				HeadLoc:                        headLoc,
				HeadCooked:                     headCooked,
				HeadRaw:                        headRaw,
				TagWasOriginallyPropertyAccess: js_ast.IsPropertyAccess(left),
			}}

		case js_lexer.TTemplateHead:
			if oldOptionalChain != js_ast.OptionalChainNone {
				p.log.AddError(&p.tracker, p.lexer.Range(), "Template literals cannot have an optional chain as a tag")
			}
			headLoc := p.lexer.Loc()
			headCooked, headRaw := p.lexer.CookedAndRawTemplateContents()
			parts, _ := p.parseTemplateParts(true /* includeRaw */)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
				TagOrNil:                       left,
				HeadLoc:                        headLoc,
				HeadCooked:                     headCooked,
				HeadRaw:                        headRaw,
				Parts:                          parts,
				TagWasOriginallyPropertyAccess: js_ast.IsPropertyAccess(left),
			}}

		case js_lexer.TOpenBracket:
			// When parsing a decorator, ignore EIndex expressions since they may be
			// part of a computed property:
			//
			//   class Foo {
			//     @foo ['computed']() {}
			//   }
			//
			// This matches the behavior of the TypeScript compiler.
			if (flags & exprFlagDecorator) != 0 {
				return left
			}

			p.lexer.Next()

			// Allow "in" inside the brackets
			oldAllowIn := p.allowIn
			p.allowIn = true

			index := p.parseExpr(js_ast.LLowest)

			p.allowIn = oldAllowIn

			closeBracketLoc := p.saveExprCommentsHere()
			p.lexer.Expect(js_lexer.TCloseBracket)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIndex{
				Target:          left,
				Index:           index,
				OptionalChain:   oldOptionalChain,
				CloseBracketLoc: closeBracketLoc,
			}}
			optionalChain = oldOptionalChain

		case js_lexer.TOpenParen:
			if level >= js_ast.LCall {
				return left
			}
			kind := js_ast.NormalCall
			if js_ast.IsPropertyAccess(left) {
				kind = js_ast.TargetWasOriginallyPropertyAccess
			}
			args, closeParenLoc, isMultiLine := p.parseCallArgs()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ECall{
				Target:        left,
				Args:          args,
				CloseParenLoc: closeParenLoc,
				OptionalChain: oldOptionalChain,
				IsMultiLine:   isMultiLine,
				Kind:          kind,
			}}
			optionalChain = oldOptionalChain

		case js_lexer.TQuestion:
			if level >= js_ast.LConditional {
				return left
			}
			p.lexer.Next()

			// Stop now if we're parsing one of these:
			// "(a?) => {}"
			// "(a?: b) => {}"
			// "(a?, b?) => {}"
			if p.options.ts.Parse && left.Loc == p.latestArrowArgLoc && (p.lexer.Token == js_lexer.TColon ||
				p.lexer.Token == js_lexer.TCloseParen || p.lexer.Token == js_lexer.TComma) {
				if errors == nil {
					p.lexer.Unexpected()
				}
				errors.invalidExprAfterQuestion = p.lexer.Range()
				return left
			}

			// Allow "in" in between "?" and ":"
			oldAllowIn := p.allowIn
			p.allowIn = true

			yes := p.parseExpr(js_ast.LComma)

			p.allowIn = oldAllowIn

			p.lexer.Expect(js_lexer.TColon)
			no := p.parseExpr(js_ast.LComma)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIf{Test: left, Yes: yes, No: no}}

		case js_lexer.TExclamation:
			// Skip over TypeScript non-null assertions
			if p.lexer.HasNewlineBefore {
				return left
			}
			if !p.options.ts.Parse {
				p.lexer.Unexpected()
			}
			p.lexer.Next()
			optionalChain = oldOptionalChain

		case js_lexer.TMinusMinus:
			if p.lexer.HasNewlineBefore || level >= js_ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EUnary{Op: js_ast.UnOpPostDec, Value: left}}

		case js_lexer.TPlusPlus:
			if p.lexer.HasNewlineBefore || level >= js_ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EUnary{Op: js_ast.UnOpPostInc, Value: left}}

		case js_lexer.TComma:
			if level >= js_ast.LComma {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpComma, Left: left, Right: p.parseExpr(js_ast.LComma)}}

		case js_lexer.TPlus:
			if level >= js_ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpAdd, Left: left, Right: p.parseExpr(js_ast.LAdd)}}

		case js_lexer.TPlusEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpAddAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TMinus:
			if level >= js_ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpSub, Left: left, Right: p.parseExpr(js_ast.LAdd)}}

		case js_lexer.TMinusEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpSubAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TAsterisk:
			if level >= js_ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpMul, Left: left, Right: p.parseExpr(js_ast.LMultiply)}}

		case js_lexer.TAsteriskAsterisk:
			if level >= js_ast.LExponentiation {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpPow, Left: left, Right: p.parseExpr(js_ast.LExponentiation - 1)}}

		case js_lexer.TAsteriskAsteriskEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpPowAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TAsteriskEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpMulAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TPercent:
			if level >= js_ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpRem, Left: left, Right: p.parseExpr(js_ast.LMultiply)}}

		case js_lexer.TPercentEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpRemAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TSlash:
			if level >= js_ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpDiv, Left: left, Right: p.parseExpr(js_ast.LMultiply)}}

		case js_lexer.TSlashEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpDivAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TEqualsEquals:
			if level >= js_ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLooseEq, Left: left, Right: p.parseExpr(js_ast.LEquals)}}

		case js_lexer.TExclamationEquals:
			if level >= js_ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLooseNe, Left: left, Right: p.parseExpr(js_ast.LEquals)}}

		case js_lexer.TEqualsEqualsEquals:
			if level >= js_ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpStrictEq, Left: left, Right: p.parseExpr(js_ast.LEquals)}}

		case js_lexer.TExclamationEqualsEquals:
			if level >= js_ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpStrictNe, Left: left, Right: p.parseExpr(js_ast.LEquals)}}

		case js_lexer.TLessThan:
			// TypeScript allows type arguments to be specified with angle brackets
			// inside an expression. Unlike in other languages, this unfortunately
			// appears to require backtracking to parse.
			if p.options.ts.Parse && p.trySkipTypeArgumentsInExpressionWithBacktracking() {
				optionalChain = oldOptionalChain
				continue
			}

			if level >= js_ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLt, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		case js_lexer.TLessThanEquals:
			if level >= js_ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLe, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		case js_lexer.TGreaterThan:
			if level >= js_ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpGt, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		case js_lexer.TGreaterThanEquals:
			if level >= js_ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpGe, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		case js_lexer.TLessThanLessThan:
			// TypeScript allows type arguments to be specified with angle brackets
			// inside an expression. Unlike in other languages, this unfortunately
			// appears to require backtracking to parse.
			if p.options.ts.Parse && p.trySkipTypeArgumentsInExpressionWithBacktracking() {
				optionalChain = oldOptionalChain
				continue
			}

			if level >= js_ast.LShift {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpShl, Left: left, Right: p.parseExpr(js_ast.LShift)}}

		case js_lexer.TLessThanLessThanEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpShlAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TGreaterThanGreaterThan:
			if level >= js_ast.LShift {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpShr, Left: left, Right: p.parseExpr(js_ast.LShift)}}

		case js_lexer.TGreaterThanGreaterThanEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpShrAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TGreaterThanGreaterThanGreaterThan:
			if level >= js_ast.LShift {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpUShr, Left: left, Right: p.parseExpr(js_ast.LShift)}}

		case js_lexer.TGreaterThanGreaterThanGreaterThanEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpUShrAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TQuestionQuestion:
			if level >= js_ast.LNullishCoalescing {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpNullishCoalescing, Left: left, Right: p.parseExpr(js_ast.LNullishCoalescing)}}

		case js_lexer.TQuestionQuestionEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpNullishCoalescingAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TBarBar:
			if level >= js_ast.LLogicalOr {
				return left
			}

			// Prevent "||" inside "??" from the right
			if level == js_ast.LNullishCoalescing {
				p.logNullishCoalescingErrorPrecedenceError("||")
			}

			p.lexer.Next()
			right := p.parseExpr(js_ast.LLogicalOr)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLogicalOr, Left: left, Right: right}}

			// Prevent "||" inside "??" from the left
			if level < js_ast.LNullishCoalescing {
				left = p.parseSuffix(left, js_ast.LNullishCoalescing+1, nil, flags)
				if p.lexer.Token == js_lexer.TQuestionQuestion {
					p.logNullishCoalescingErrorPrecedenceError("||")
				}
			}

		case js_lexer.TBarBarEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLogicalOrAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TAmpersandAmpersand:
			if level >= js_ast.LLogicalAnd {
				return left
			}

			// Prevent "&&" inside "??" from the right
			if level == js_ast.LNullishCoalescing {
				p.logNullishCoalescingErrorPrecedenceError("&&")
			}

			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLogicalAnd, Left: left, Right: p.parseExpr(js_ast.LLogicalAnd)}}

			// Prevent "&&" inside "??" from the left
			if level < js_ast.LNullishCoalescing {
				left = p.parseSuffix(left, js_ast.LNullishCoalescing+1, nil, flags)
				if p.lexer.Token == js_lexer.TQuestionQuestion {
					p.logNullishCoalescingErrorPrecedenceError("&&")
				}
			}

		case js_lexer.TAmpersandAmpersandEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLogicalAndAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TBar:
			if level >= js_ast.LBitwiseOr {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpBitwiseOr, Left: left, Right: p.parseExpr(js_ast.LBitwiseOr)}}

		case js_lexer.TBarEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpBitwiseOrAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TAmpersand:
			if level >= js_ast.LBitwiseAnd {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpBitwiseAnd, Left: left, Right: p.parseExpr(js_ast.LBitwiseAnd)}}

		case js_lexer.TAmpersandEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpBitwiseAndAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TCaret:
			if level >= js_ast.LBitwiseXor {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpBitwiseXor, Left: left, Right: p.parseExpr(js_ast.LBitwiseXor)}}

		case js_lexer.TCaretEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpBitwiseXorAssign, Left: left, Right: p.parseExpr(js_ast.LAssign - 1)}}

		case js_lexer.TEquals:
			if level >= js_ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = js_ast.Assign(left, p.parseExpr(js_ast.LAssign-1))

		case js_lexer.TIn:
			if level >= js_ast.LCompare || !p.allowIn {
				return left
			}

			// Warn about "!a in b" instead of "!(a in b)"
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode {
				kind = logger.Debug
			}
			if e, ok := left.Data.(*js_ast.EUnary); ok && e.Op == js_ast.UnOpNot {
				r := logger.Range{Loc: left.Loc, Len: p.source.LocBeforeWhitespace(p.lexer.Loc()).Start - left.Loc.Start}
				data := p.tracker.MsgData(r, "Suspicious use of the \"!\" operator inside the \"in\" operator")
				data.Location.Suggestion = fmt.Sprintf("(%s)", p.source.TextForRange(r))
				p.log.AddMsgID(logger.MsgID_JS_SuspiciousBooleanNot, logger.Msg{
					Kind: kind,
					Data: data,
					Notes: []logger.MsgData{{Text: "The code \"!x in y\" is parsed as \"(!x) in y\". " +
						"You need to insert parentheses to get \"!(x in y)\" instead."}},
				})
			}

			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpIn, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		case js_lexer.TInstanceof:
			if level >= js_ast.LCompare {
				return left
			}

			// Warn about "!a instanceof b" instead of "!(a instanceof b)". Here's an
			// example of code with this problem: https://github.com/mrdoob/three.js/pull/11182.
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode {
				kind = logger.Debug
			}
			if e, ok := left.Data.(*js_ast.EUnary); ok && e.Op == js_ast.UnOpNot {
				r := logger.Range{Loc: left.Loc, Len: p.source.LocBeforeWhitespace(p.lexer.Loc()).Start - left.Loc.Start}
				data := p.tracker.MsgData(r, "Suspicious use of the \"!\" operator inside the \"instanceof\" operator")
				data.Location.Suggestion = fmt.Sprintf("(%s)", p.source.TextForRange(r))
				p.log.AddMsgID(logger.MsgID_JS_SuspiciousBooleanNot, logger.Msg{
					Kind: kind,
					Data: data,
					Notes: []logger.MsgData{{Text: "The code \"!x instanceof y\" is parsed as \"(!x) instanceof y\". " +
						"You need to insert parentheses to get \"!(x instanceof y)\" instead."}},
				})
			}

			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpInstanceof, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		default:
			// Handle the TypeScript "as"/"satisfies" operator
			if p.options.ts.Parse && level < js_ast.LCompare && !p.lexer.HasNewlineBefore && (p.lexer.IsContextualKeyword("as") || p.lexer.IsContextualKeyword("satisfies")) {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)

				// These tokens are not allowed to follow a cast expression. This isn't
				// an outright error because it may be on a new line, in which case it's
				// the start of a new expression when it's after a cast:
				//
				//   x = y as z
				//   (something);
				//
				switch p.lexer.Token {
				case js_lexer.TPlusPlus, js_lexer.TMinusMinus, js_lexer.TNoSubstitutionTemplateLiteral,
					js_lexer.TTemplateHead, js_lexer.TOpenParen, js_lexer.TOpenBracket, js_lexer.TQuestionDot:
					p.forbidSuffixAfterAsLoc = p.lexer.Loc()
					return left
				}
				if p.lexer.Token.IsAssign() {
					p.forbidSuffixAfterAsLoc = p.lexer.Loc()
					return left
				}
				continue
			}

			return left
		}
	}
}

func (p *parser) parseExprOrLetOrUsingStmt(opts parseStmtOpts) (js_ast.Expr, js_ast.Stmt, []js_ast.Decl) {
	couldBeLet := false
	couldBeUsing := false
	couldBeAwaitUsing := false
	tokenRange := p.lexer.Range()

	if p.lexer.Token == js_lexer.TIdentifier {
		raw := p.lexer.Raw()
		couldBeLet = raw == "let"
		couldBeUsing = raw == "using"
		couldBeAwaitUsing = raw == "await" && p.fnOrArrowDataParse.await == allowExpr
	}

	if !couldBeLet && !couldBeUsing && !couldBeAwaitUsing {
		var flags exprFlag
		if opts.isForLoopInit {
			flags |= exprFlagForLoopInit
		}
		if opts.isForAwaitLoopInit {
			flags |= exprFlagForAwaitLoopInit
		}
		return p.parseExprCommon(js_ast.LLowest, nil, flags), js_ast.Stmt{}, nil
	}

	name := p.lexer.Identifier
	p.lexer.Next()

	if couldBeLet {
		isLet := opts.isExport
		switch p.lexer.Token {
		case js_lexer.TIdentifier, js_lexer.TOpenBracket, js_lexer.TOpenBrace:
			if opts.lexicalDecl == lexicalDeclAllowAll || !p.lexer.HasNewlineBefore || p.lexer.Token == js_lexer.TOpenBracket {
				isLet = true
			}
		}
		if isLet {
			// Handle a "let" declaration
			if opts.lexicalDecl != lexicalDeclAllowAll {
				p.forbidLexicalDecl(tokenRange.Loc)
			}
			p.markSyntaxFeature(compat.ConstAndLet, tokenRange)
			decls := p.parseAndDeclareDecls(ast.SymbolOther, opts)
			return js_ast.Expr{}, js_ast.Stmt{Loc: tokenRange.Loc, Data: &js_ast.SLocal{
				Kind:     js_ast.LocalLet,
				Decls:    decls,
				IsExport: opts.isExport,
			}}, decls
		}
	} else if couldBeUsing && p.lexer.Token == js_lexer.TIdentifier && !p.lexer.HasNewlineBefore && (!opts.isForLoopInit || p.lexer.Raw() != "of") {
		// Handle a "using" declaration
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(tokenRange.Loc)
		}
		opts.isUsingStmt = true
		decls := p.parseAndDeclareDecls(ast.SymbolConst, opts)
		if !opts.isForLoopInit {
			p.requireInitializers(js_ast.LocalUsing, decls)
		}
		return js_ast.Expr{}, js_ast.Stmt{Loc: tokenRange.Loc, Data: &js_ast.SLocal{
			Kind:     js_ast.LocalUsing,
			Decls:    decls,
			IsExport: opts.isExport,
		}}, decls
	} else if couldBeAwaitUsing {
		// Handle an "await using" declaration
		if p.fnOrArrowDataParse.isTopLevel {
			p.topLevelAwaitKeyword = tokenRange
		}
		var value js_ast.Expr
		if p.lexer.Token == js_lexer.TIdentifier && p.lexer.Raw() == "using" {
			usingLoc := p.saveExprCommentsHere()
			usingRange := p.lexer.Range()
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TIdentifier && !p.lexer.HasNewlineBefore {
				// It's an "await using" declaration if we get here
				if opts.lexicalDecl != lexicalDeclAllowAll {
					p.forbidLexicalDecl(usingRange.Loc)
				}
				opts.isUsingStmt = true
				decls := p.parseAndDeclareDecls(ast.SymbolConst, opts)
				if !opts.isForLoopInit {
					p.requireInitializers(js_ast.LocalAwaitUsing, decls)
				}
				return js_ast.Expr{}, js_ast.Stmt{Loc: tokenRange.Loc, Data: &js_ast.SLocal{
					Kind:     js_ast.LocalAwaitUsing,
					Decls:    decls,
					IsExport: opts.isExport,
				}}, decls
			}
			value = js_ast.Expr{Loc: usingLoc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef(js_lexer.MaybeSubstring{String: "using"})}}
		} else {
			value = p.parseExpr(js_ast.LPrefix)
		}
		if p.lexer.Token == js_lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		value = p.parseSuffix(value, js_ast.LPrefix, nil, 0)
		expr := js_ast.Expr{Loc: tokenRange.Loc, Data: &js_ast.EAwait{Value: value}}
		return p.parseSuffix(expr, js_ast.LLowest, nil, 0), js_ast.Stmt{}, nil
	}

	// Parse the remainder of this expression that starts with an identifier
	expr := js_ast.Expr{Loc: tokenRange.Loc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef(name)}}
	return p.parseSuffix(expr, js_ast.LLowest, nil, 0), js_ast.Stmt{}, nil
}

func (p *parser) parseCallArgs() (args []js_ast.Expr, closeParenLoc logger.Loc, isMultiLine bool) {
	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	p.lexer.Expect(js_lexer.TOpenParen)

	for p.lexer.Token != js_lexer.TCloseParen {
		if p.lexer.HasNewlineBefore {
			isMultiLine = true
		}
		loc := p.lexer.Loc()
		isSpread := p.lexer.Token == js_lexer.TDotDotDot
		if isSpread {
			p.markSyntaxFeature(compat.RestArgument, p.lexer.Range())
			p.lexer.Next()
		}
		arg := p.parseExpr(js_ast.LComma)
		if isSpread {
			arg = js_ast.Expr{Loc: loc, Data: &js_ast.ESpread{Value: arg}}
		}
		args = append(args, arg)
		if p.lexer.Token != js_lexer.TComma {
			break
		}
		if p.lexer.HasNewlineBefore {
			isMultiLine = true
		}
		p.lexer.Next()
	}

	if p.lexer.HasNewlineBefore {
		isMultiLine = true
	}
	closeParenLoc = p.saveExprCommentsHere()
	p.lexer.Expect(js_lexer.TCloseParen)
	p.allowIn = oldAllowIn
	return
}

func (p *parser) parseJSXNamespacedName() (logger.Range, js_lexer.MaybeSubstring) {
	nameRange := p.lexer.Range()
	name := p.lexer.Identifier
	p.lexer.ExpectInsideJSXElement(js_lexer.TIdentifier)

	// Parse JSX namespaces. These are not supported by React or TypeScript
	// but someone using JSX syntax in more obscure ways may find a use for
	// them. A namespaced name is just always turned into a string so you
	// can't use this feature to reference JavaScript identifiers.
	if p.lexer.Token == js_lexer.TColon {
		// Parse the colon
		nameRange.Len = p.lexer.Range().End() - nameRange.Loc.Start
		ns := name.String + ":"
		p.lexer.NextInsideJSXElement()

		// Parse the second identifier
		if p.lexer.Token == js_lexer.TIdentifier {
			nameRange.Len = p.lexer.Range().End() - nameRange.Loc.Start
			ns += p.lexer.Identifier.String
			p.lexer.NextInsideJSXElement()
		} else {
			p.log.AddError(&p.tracker, logger.Range{Loc: logger.Loc{Start: nameRange.End()}},
				fmt.Sprintf("Expected identifier after %q in namespaced JSX name", ns))
			panic(js_lexer.LexerPanic{})
		}
		return nameRange, js_lexer.MaybeSubstring{String: ns}
	}

	return nameRange, name
}

func tagOrFragmentHelpText(tag string) string {
	if tag == "" {
		return "fragment tag"
	}
	return fmt.Sprintf("%q tag", tag)
}

func (p *parser) parseJSXTag() (logger.Range, string, js_ast.Expr) {
	loc := p.lexer.Loc()

	// A missing tag is a fragment
	if p.lexer.Token == js_lexer.TGreaterThan {
		return logger.Range{Loc: loc, Len: 0}, "", js_ast.Expr{}
	}

	// The tag is an identifier
	tagRange, tagName := p.parseJSXNamespacedName()

	// Certain identifiers are strings
	if strings.ContainsAny(tagName.String, "-:") || (p.lexer.Token != js_lexer.TDot && tagName.String[0] >= 'a' && tagName.String[0] <= 'z') {
		return tagRange, tagName.String, js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(tagName.String)}}
	}

	// Otherwise, this is an identifier
	tag := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef(tagName)}}

	// Parse a member expression chain
	chain := tagName.String
	for p.lexer.Token == js_lexer.TDot {
		p.lexer.NextInsideJSXElement()
		memberRange := p.lexer.Range()
		member := p.lexer.Identifier
		p.lexer.ExpectInsideJSXElement(js_lexer.TIdentifier)

		// Dashes are not allowed in member expression chains
		index := strings.IndexByte(member.String, '-')
		if index >= 0 {
			p.log.AddError(&p.tracker, logger.Range{Loc: logger.Loc{Start: memberRange.Loc.Start + int32(index)}},
				"Unexpected \"-\"")
			panic(js_lexer.LexerPanic{})
		}

		chain += "." + member.String
		tag = js_ast.Expr{Loc: loc, Data: p.dotOrMangledPropParse(tag, member, memberRange.Loc, js_ast.OptionalChainNone, wasOriginallyDot)}
		tagRange.Len = memberRange.Loc.Start + memberRange.Len - tagRange.Loc.Start
	}

	return tagRange, chain, tag
}

func (p *parser) parseJSXElement(loc logger.Loc) js_ast.Expr {
	// Keep track of the location of the first JSX element for error messages
	if p.firstJSXElementLoc.Start == -1 {
		p.firstJSXElementLoc = loc
	}

	// Parse the tag
	startRange, startText, startTagOrNil := p.parseJSXTag()

	// The tag may have TypeScript type arguments: "<Foo<T>/>"
	if p.options.ts.Parse {
		// Pass a flag to the type argument skipper because we need to call
		// js_lexer.NextInsideJSXElement() after we hit the closing ">". The next
		// token after the ">" might be an attribute name with a dash in it
		// like this: "<Foo<T> data-disabled/>"
		p.skipTypeScriptTypeArguments(skipTypeScriptTypeArgumentsOpts{isInsideJSXElement: true})
	}

	// Parse attributes
	var previousStringWithBackslashLoc logger.Loc
	properties := []js_ast.Property{}
	isSingleLine := true
	if startTagOrNil.Data != nil {
	parseAttributes:
		for {
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}

			switch p.lexer.Token {
			case js_lexer.TIdentifier:
				// Parse the key
				keyRange, keyName := p.parseJSXNamespacedName()
				var key js_ast.Expr
				if p.isMangledProp(keyName.String) && !strings.ContainsRune(keyName.String, ':') {
					key = js_ast.Expr{Loc: keyRange.Loc, Data: &js_ast.ENameOfSymbol{Ref: p.storeNameInRef(keyName)}}
				} else {
					key = js_ast.Expr{Loc: keyRange.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(keyName.String)}}
				}

				// Parse the value
				var value js_ast.Expr
				var flags js_ast.PropertyFlags
				if p.lexer.Token != js_lexer.TEquals {
					// Implicitly true value
					flags |= js_ast.PropertyWasShorthand
					value = js_ast.Expr{Loc: logger.Loc{Start: keyRange.Loc.Start + keyRange.Len}, Data: &js_ast.EBoolean{Value: true}}
				} else {
					// Use NextInsideJSXElement() not Next() so we can parse a JSX-style string literal
					p.lexer.NextInsideJSXElement()
					if p.lexer.Token == js_lexer.TStringLiteral {
						stringLoc := p.lexer.Loc()
						if p.lexer.PreviousBackslashQuoteInJSX.Loc.Start > stringLoc.Start {
							previousStringWithBackslashLoc = stringLoc
						}
						if p.options.jsx.Preserve {
							value = js_ast.Expr{Loc: stringLoc, Data: &js_ast.EJSXText{Raw: p.lexer.Raw()}}
						} else {
							value = js_ast.Expr{Loc: stringLoc, Data: &js_ast.EString{Value: p.lexer.StringLiteral()}}
						}
						p.lexer.NextInsideJSXElement()
					} else if p.lexer.Token == js_lexer.TLessThan {
						// This may be removed in the future: https://github.com/facebook/jsx/issues/53
						loc := p.lexer.Loc()
						p.lexer.NextInsideJSXElement()
						flags |= js_ast.PropertyWasShorthand
						value = p.parseJSXElement(loc)

						// The call to parseJSXElement() above doesn't consume the last
						// TGreaterThan because the caller knows what Next() function to call.
						// Use NextJSXElementChild() here since the next token is inside a JSX
						// element.
						p.lexer.NextInsideJSXElement()
					} else {
						// Use Expect() not ExpectInsideJSXElement() so we can parse expression tokens
						p.lexer.Expect(js_lexer.TOpenBrace)
						value = p.parseExpr(js_ast.LLowest)
						p.lexer.ExpectInsideJSXElement(js_lexer.TCloseBrace)
					}
				}

				// Add a property
				properties = append(properties, js_ast.Property{
					Loc:        keyRange.Loc,
					Key:        key,
					ValueOrNil: value,
					Flags:      flags,
				})

			case js_lexer.TOpenBrace:
				// Use Next() not ExpectInsideJSXElement() so we can parse "..."
				p.lexer.Next()
				dotLoc := p.saveExprCommentsHere()
				p.lexer.Expect(js_lexer.TDotDotDot)
				value := p.parseExpr(js_ast.LComma)
				properties = append(properties, js_ast.Property{
					Kind:       js_ast.PropertySpread,
					Loc:        dotLoc,
					ValueOrNil: value,
				})

				// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
				p.lexer.NextInsideJSXElement()

			default:
				break parseAttributes
			}
		}

		// Check for and warn about duplicate attributes
		if len(properties) > 1 && !p.suppressWarningsAboutWeirdCode {
			keys := make(map[string]logger.Loc)
			for _, property := range properties {
				if property.Kind != js_ast.PropertySpread {
					if str, ok := property.Key.Data.(*js_ast.EString); ok {
						key := helpers.UTF16ToString(str.Value)
						if prevLoc, ok := keys[key]; ok {
							r := js_lexer.RangeOfIdentifier(p.source, property.Key.Loc)
							p.log.AddIDWithNotes(logger.MsgID_JS_DuplicateObjectKey, logger.Warning, &p.tracker, r,
								fmt.Sprintf("Duplicate %q attribute in JSX element", key),
								[]logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, prevLoc),
									fmt.Sprintf("The original %q attribute is here:", key))})
						}
						keys[key] = property.Key.Loc
					}
				}
			}
		}
	}

	// People sometimes try to use the output of "JSON.stringify()" as a JSX
	// attribute when automatically-generating JSX code. Doing so is incorrect
	// because JSX strings work like XML instead of like JS (since JSX is XML-in-
	// JS). Specifically, using a backslash before a quote does not cause it to
	// be escaped:
	//
	//   JSX ends the "content" attribute here and sets "content" to 'some so-called \\'
	//                                          v
	//         <Button content="some so-called \"button text\"" />
	//                                                      ^
	//       There is no "=" after the JSX attribute "text", so we expect a ">"
	//
	// This code special-cases this error to provide a less obscure error message.
	if p.lexer.Token == js_lexer.TSyntaxError && p.lexer.Raw() == "\\" && previousStringWithBackslashLoc.Start > 0 {
		msg := logger.Msg{Kind: logger.Error, Data: p.tracker.MsgData(p.lexer.Range(),
			"Unexpected backslash in JSX element")}

		// Option 1: Suggest using an XML escape
		jsEscape := p.source.TextForRange(p.lexer.PreviousBackslashQuoteInJSX)
		xmlEscape := ""
		if jsEscape == "\\\"" {
			xmlEscape = "&quot;"
		} else if jsEscape == "\\'" {
			xmlEscape = "&apos;"
		}
		if xmlEscape != "" {
			data := p.tracker.MsgData(p.lexer.PreviousBackslashQuoteInJSX,
				"Quoted JSX attributes use XML-style escapes instead of JavaScript-style escapes:")
			data.Location.Suggestion = xmlEscape
			msg.Notes = append(msg.Notes, data)
		}

		// Option 2: Suggest using a JavaScript string
		if stringRange := p.source.RangeOfString(previousStringWithBackslashLoc); stringRange.Len > 0 {
			data := p.tracker.MsgData(stringRange,
				"Consider using a JavaScript string inside {...} instead of a quoted JSX attribute:")
			data.Location.Suggestion = fmt.Sprintf("{%s}", p.source.TextForRange(stringRange))
			msg.Notes = append(msg.Notes, data)
		}

		p.log.AddMsg(msg)
		panic(js_lexer.LexerPanic{})
	}

	// A slash here is a self-closing element
	if p.lexer.Token == js_lexer.TSlash {
		// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
		closeLoc := p.lexer.Loc()
		p.lexer.NextInsideJSXElement()
		if p.lexer.Token != js_lexer.TGreaterThan {
			p.lexer.Expected(js_lexer.TGreaterThan)
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EJSXElement{
			TagOrNil:        startTagOrNil,
			Properties:      properties,
			CloseLoc:        closeLoc,
			IsTagSingleLine: isSingleLine,
		}}
	}

	// Attempt to provide a better error message for people incorrectly trying to
	// use arrow functions in TSX (which doesn't work because they are JSX elements)
	if p.options.ts.Parse && len(properties) == 0 && startText != "" && p.lexer.Token == js_lexer.TGreaterThan &&
		strings.HasPrefix(p.source.Contents[p.lexer.Loc().Start:], ">(") {
		badArrowInTSXRange := p.lexer.BadArrowInTSXRange
		badArrowInTSXSuggestion := p.lexer.BadArrowInTSXSuggestion

		p.lexer.CouldBeBadArrowInTSX++
		p.lexer.BadArrowInTSXRange = logger.Range{Loc: loc, Len: p.lexer.Range().End() - loc.Start}
		p.lexer.BadArrowInTSXSuggestion = fmt.Sprintf("<%s,>", startText)

		defer func() {
			p.lexer.CouldBeBadArrowInTSX--
			p.lexer.BadArrowInTSXRange = badArrowInTSXRange
			p.lexer.BadArrowInTSXSuggestion = badArrowInTSXSuggestion
		}()
	}

	// Use ExpectJSXElementChild() so we parse child strings
	p.lexer.ExpectJSXElementChild(js_lexer.TGreaterThan)

	// Parse the children of this element
	nullableChildren := []js_ast.Expr{}
	for {
		switch p.lexer.Token {
		case js_lexer.TStringLiteral:
			if p.options.jsx.Preserve {
				nullableChildren = append(nullableChildren, js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EJSXText{Raw: p.lexer.Raw()}})
			} else if str := p.lexer.StringLiteral(); len(str) > 0 {
				nullableChildren = append(nullableChildren, js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EString{Value: str}})
			} else {
				// Skip this token if it turned out to be empty after trimming
			}
			p.lexer.NextJSXElementChild()

		case js_lexer.TOpenBrace:
			// Use Next() instead of NextJSXElementChild() here since the next token is an expression
			p.lexer.Next()

			// The expression is optional, and may be absent
			if p.lexer.Token == js_lexer.TCloseBrace {
				// Save comments even for absent expressions
				nullableChildren = append(nullableChildren, js_ast.Expr{Loc: p.saveExprCommentsHere(), Data: nil})
			} else {
				if p.lexer.Token == js_lexer.TDotDotDot {
					// TypeScript preserves "..." before JSX child expressions here.
					// Babel gives the error "Spread children are not supported in React"
					// instead, so it should be safe to support this TypeScript-specific
					// behavior. Note that TypeScript's behavior changed in TypeScript 4.5.
					// Before that, the "..." was omitted instead of being preserved.
					itemLoc := p.lexer.Loc()
					p.markSyntaxFeature(compat.RestArgument, p.lexer.Range())
					p.lexer.Next()
					nullableChildren = append(nullableChildren, js_ast.Expr{Loc: itemLoc, Data: &js_ast.ESpread{Value: p.parseExpr(js_ast.LLowest)}})
				} else {
					nullableChildren = append(nullableChildren, p.parseExpr(js_ast.LLowest))
				}
			}

			// Use ExpectJSXElementChild() so we parse child strings
			p.lexer.ExpectJSXElementChild(js_lexer.TCloseBrace)

		case js_lexer.TLessThan:
			lessThanLoc := p.lexer.Loc()
			p.lexer.NextInsideJSXElement()

			if p.lexer.Token != js_lexer.TSlash {
				// This is a child element
				nullableChildren = append(nullableChildren, p.parseJSXElement(lessThanLoc))

				// The call to parseJSXElement() above doesn't consume the last
				// TGreaterThan because the caller knows what Next() function to call.
				// Use NextJSXElementChild() here since the next token is an element
				// child.
				p.lexer.NextJSXElementChild()
				continue
			}

			// This is the closing element
			p.lexer.NextInsideJSXElement()
			endRange, endText, _ := p.parseJSXTag()
			if startText != endText {
				startTag := tagOrFragmentHelpText(startText)
				endTag := tagOrFragmentHelpText(endText)
				msg := logger.Msg{
					Kind:  logger.Error,
					Data:  p.tracker.MsgData(endRange, fmt.Sprintf("Unexpected closing %s does not match opening %s", endTag, startTag)),
					Notes: []logger.MsgData{p.tracker.MsgData(startRange, fmt.Sprintf("The opening %s is here:", startTag))},
				}
				msg.Data.Location.Suggestion = startText
				p.log.AddMsg(msg)
			}
			if p.lexer.Token != js_lexer.TGreaterThan {
				p.lexer.Expected(js_lexer.TGreaterThan)
			}

			return js_ast.Expr{Loc: loc, Data: &js_ast.EJSXElement{
				TagOrNil:         startTagOrNil,
				Properties:       properties,
				NullableChildren: nullableChildren,
				CloseLoc:         lessThanLoc,
				IsTagSingleLine:  isSingleLine,
			}}

		case js_lexer.TEndOfFile:
			startTag := tagOrFragmentHelpText(startText)
			msg := logger.Msg{
				Kind:  logger.Error,
				Data:  p.tracker.MsgData(p.lexer.Range(), fmt.Sprintf("Unexpected end of file before a closing %s", startTag)),
				Notes: []logger.MsgData{p.tracker.MsgData(startRange, fmt.Sprintf("The opening %s is here:", startTag))},
			}
			msg.Data.Location.Suggestion = fmt.Sprintf("</%s>", startText)
			p.log.AddMsg(msg)
			panic(js_lexer.LexerPanic{})

		default:
			p.lexer.Unexpected()
		}
	}
}

func (p *parser) parseTemplateParts(includeRaw bool) (parts []js_ast.TemplatePart, legacyOctalLoc logger.Loc) {
	// Allow "in" inside template literals
	oldAllowIn := p.allowIn
	p.allowIn = true

	for {
		p.lexer.Next()
		value := p.parseExpr(js_ast.LLowest)
		tailLoc := p.lexer.Loc()
		p.lexer.RescanCloseBraceAsTemplateToken()
		if includeRaw {
			tailCooked, tailRaw := p.lexer.CookedAndRawTemplateContents()
			parts = append(parts, js_ast.TemplatePart{
				Value:      value,
				TailLoc:    tailLoc,
				TailCooked: tailCooked,
				TailRaw:    tailRaw,
			})
		} else {
			parts = append(parts, js_ast.TemplatePart{
				Value:      value,
				TailLoc:    tailLoc,
				TailCooked: p.lexer.StringLiteral(),
			})
			if p.lexer.LegacyOctalLoc.Start > tailLoc.Start {
				legacyOctalLoc = p.lexer.LegacyOctalLoc
			}
		}
		if p.lexer.Token == js_lexer.TTemplateTail {
			p.lexer.Next()
			break
		}
	}

	p.allowIn = oldAllowIn

	return parts, legacyOctalLoc
}

func (p *parser) parseAndDeclareDecls(kind ast.SymbolKind, opts parseStmtOpts) []js_ast.Decl {
	decls := []js_ast.Decl{}

	for {
		// Forbid "let let" and "const let" but not "var let"
		if (kind == ast.SymbolOther || kind == ast.SymbolConst) && p.lexer.IsContextualKeyword("let") {
			p.log.AddError(&p.tracker, p.lexer.Range(), "Cannot use \"let\" as an identifier here:")
		}

		var valueOrNil js_ast.Expr
		local := p.parseBinding(parseBindingOpts{isUsingStmt: opts.isUsingStmt})
		p.declareBinding(kind, local, opts)

		// Skip over types
		if p.options.ts.Parse {
			// "let foo!"
			isDefiniteAssignmentAssertion := p.lexer.Token == js_lexer.TExclamation && !p.lexer.HasNewlineBefore
			if isDefiniteAssignmentAssertion {
				p.lexer.Next()
			}

			// "let foo: number"
			if isDefiniteAssignmentAssertion || p.lexer.Token == js_lexer.TColon {
				p.lexer.Expect(js_lexer.TColon)
				p.skipTypeScriptType(js_ast.LLowest)
			}
		}

		if p.lexer.Token == js_lexer.TEquals {
			p.lexer.Next()
			valueOrNil = p.parseExpr(js_ast.LComma)

			// Rollup (the tool that invented the "@__NO_SIDE_EFFECTS__" comment) only
			// applies this to the first declaration, and only when it's a "const".
			// For more info see: https://github.com/rollup/rollup/pull/5024/files
			if !p.options.ignoreDCEAnnotations && kind == ast.SymbolConst {
				switch e := valueOrNil.Data.(type) {
				case *js_ast.EArrow:
					if opts.hasNoSideEffectsComment {
						e.HasNoSideEffectsComment = true
					}
					if e.HasNoSideEffectsComment && !opts.isTypeScriptDeclare {
						if b, ok := local.Data.(*js_ast.BIdentifier); ok {
							p.symbols[b.Ref.InnerIndex].Flags |= ast.CallCanBeUnwrappedIfUnused
						}
					}

				case *js_ast.EFunction:
					if opts.hasNoSideEffectsComment {
						e.Fn.HasNoSideEffectsComment = true
					}
					if e.Fn.HasNoSideEffectsComment && !opts.isTypeScriptDeclare {
						if b, ok := local.Data.(*js_ast.BIdentifier); ok {
							p.symbols[b.Ref.InnerIndex].Flags |= ast.CallCanBeUnwrappedIfUnused
						}
					}
				}

				// Only apply this to the first declaration
				opts.hasNoSideEffectsComment = false
			}
		}

		decls = append(decls, js_ast.Decl{Binding: local, ValueOrNil: valueOrNil})

		if p.lexer.Token != js_lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	return decls
}

func (p *parser) requireInitializers(kind js_ast.LocalKind, decls []js_ast.Decl) {
	for _, d := range decls {
		if d.ValueOrNil.Data == nil {
			what := "constant"
			if kind == js_ast.LocalUsing {
				what = "declaration"
			}
			if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
				r := js_lexer.RangeOfIdentifier(p.source, d.Binding.Loc)
				p.log.AddError(&p.tracker, r,
					fmt.Sprintf("The %s %q must be initialized", what, p.symbols[id.Ref.InnerIndex].OriginalName))
			} else {
				p.log.AddError(&p.tracker, logger.Range{Loc: d.Binding.Loc},
					fmt.Sprintf("This %s must be initialized", what))
			}
		}
	}
}

func (p *parser) forbidInitializers(decls []js_ast.Decl, loopType string, isVar bool) {
	if len(decls) > 1 {
		p.log.AddError(&p.tracker, logger.Range{Loc: decls[0].Binding.Loc},
			fmt.Sprintf("for-%s loops must have a single declaration", loopType))
	} else if len(decls) == 1 && decls[0].ValueOrNil.Data != nil {
		if isVar {
			if _, ok := decls[0].Binding.Data.(*js_ast.BIdentifier); ok {
				// This is a weird special case. Initializers are allowed in "var"
				// statements with identifier bindings.
				return
			}
		}
		p.log.AddError(&p.tracker, logger.Range{Loc: decls[0].ValueOrNil.Loc},
			fmt.Sprintf("for-%s loop variables cannot have an initializer", loopType))
	}
}

func (p *parser) parseClauseAlias(kind string) js_lexer.MaybeSubstring {
	loc := p.lexer.Loc()

	// The alias may now be a string (see https://github.com/tc39/ecma262/pull/2154)
	if p.lexer.Token == js_lexer.TStringLiteral {
		r := p.source.RangeOfString(loc)
		alias, problem, ok := helpers.UTF16ToStringWithValidation(p.lexer.StringLiteral())
		if !ok {
			p.log.AddError(&p.tracker, r,
				fmt.Sprintf("This %s alias is invalid because it contains the unpaired Unicode surrogate U+%X", kind, problem))
		}
		return js_lexer.MaybeSubstring{String: alias}
	}

	// The alias may be a keyword
	if !p.lexer.IsIdentifierOrKeyword() {
		p.lexer.Expect(js_lexer.TIdentifier)
	}

	alias := p.lexer.Identifier
	p.checkForUnrepresentableIdentifier(loc, alias.String)
	return alias
}

func (p *parser) parseImportClause() ([]js_ast.ClauseItem, bool) {
	items := []js_ast.ClauseItem{}
	p.lexer.Expect(js_lexer.TOpenBrace)
	isSingleLine := !p.lexer.HasNewlineBefore

	for p.lexer.Token != js_lexer.TCloseBrace {
		isIdentifier := p.lexer.Token == js_lexer.TIdentifier
		aliasLoc := p.lexer.Loc()
		alias := p.parseClauseAlias("import")
		name := ast.LocRef{Loc: aliasLoc, Ref: p.storeNameInRef(alias)}
		originalName := alias
		p.lexer.Next()

		// "import { type xx } from 'mod'"
		// "import { type xx as yy } from 'mod'"
		// "import { type 'xx' as yy } from 'mod'"
		// "import { type as } from 'mod'"
		// "import { type as as } from 'mod'"
		// "import { type as as as } from 'mod'"
		if p.options.ts.Parse && alias.String == "type" && p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				if p.lexer.IsContextualKeyword("as") {
					originalName = p.lexer.Identifier
					name = ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
					p.lexer.Next()

					if p.lexer.Token == js_lexer.TIdentifier {
						// "import { type as as as } from 'mod'"
						// "import { type as as foo } from 'mod'"
						p.lexer.Next()
					} else {
						// "import { type as as } from 'mod'"
						items = append(items, js_ast.ClauseItem{
							Alias:        alias.String,
							AliasLoc:     aliasLoc,
							Name:         name,
							OriginalName: originalName.String,
						})
					}
				} else if p.lexer.Token == js_lexer.TIdentifier {
					// "import { type as xxx } from 'mod'"
					originalName = p.lexer.Identifier
					name = ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
					p.lexer.Expect(js_lexer.TIdentifier)

					// Reject forbidden names
					if isEvalOrArguments(originalName.String) {
						r := js_lexer.RangeOfIdentifier(p.source, name.Loc)
						p.log.AddError(&p.tracker, r, fmt.Sprintf("Cannot use %q as an identifier here:", originalName.String))
					}

					items = append(items, js_ast.ClauseItem{
						Alias:        alias.String,
						AliasLoc:     aliasLoc,
						Name:         name,
						OriginalName: originalName.String,
					})
				}
			} else {
				isIdentifier := p.lexer.Token == js_lexer.TIdentifier

				// "import { type xx } from 'mod'"
				// "import { type xx as yy } from 'mod'"
				// "import { type if as yy } from 'mod'"
				// "import { type 'xx' as yy } from 'mod'"
				p.parseClauseAlias("import")
				p.lexer.Next()

				if p.lexer.IsContextualKeyword("as") {
					p.lexer.Next()
					p.lexer.Expect(js_lexer.TIdentifier)
				} else if !isIdentifier {
					// An import where the name is a keyword must have an alias
					p.lexer.ExpectedString("\"as\"")
				}
			}
		} else {
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				originalName = p.lexer.Identifier
				name = ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
				p.lexer.Expect(js_lexer.TIdentifier)
			} else if !isIdentifier {
				// An import where the name is a keyword must have an alias
				p.lexer.ExpectedString("\"as\"")
			}

			// Reject forbidden names
			if isEvalOrArguments(originalName.String) {
				r := js_lexer.RangeOfIdentifier(p.source, name.Loc)
				p.log.AddError(&p.tracker, r, fmt.Sprintf("Cannot use %q as an identifier here:", originalName.String))
			}

			items = append(items, js_ast.ClauseItem{
				Alias:        alias.String,
				AliasLoc:     aliasLoc,
				Name:         name,
				OriginalName: originalName.String,
			})
		}

		if p.lexer.Token != js_lexer.TComma {
			break
		}
		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		p.lexer.Next()
		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
	}

	if p.lexer.HasNewlineBefore {
		isSingleLine = false
	}
	p.lexer.Expect(js_lexer.TCloseBrace)
	return items, isSingleLine
}

func (p *parser) parseExportClause() ([]js_ast.ClauseItem, bool) {
	items := []js_ast.ClauseItem{}
	firstNonIdentifierLoc := logger.Loc{}
	p.lexer.Expect(js_lexer.TOpenBrace)
	isSingleLine := !p.lexer.HasNewlineBefore

	for p.lexer.Token != js_lexer.TCloseBrace {
		alias := p.parseClauseAlias("export")
		aliasLoc := p.lexer.Loc()
		name := ast.LocRef{Loc: aliasLoc, Ref: p.storeNameInRef(alias)}
		originalName := alias

		// The name can actually be a keyword if we're really an "export from"
		// statement. However, we won't know until later. Allow keywords as
		// identifiers for now and throw an error later if there's no "from".
		//
		//   // This is fine
		//   export { default } from 'path'
		//
		//   // This is a syntax error
		//   export { default }
		//
		if p.lexer.Token != js_lexer.TIdentifier && firstNonIdentifierLoc.Start == 0 {
			firstNonIdentifierLoc = p.lexer.Loc()
		}
		p.lexer.Next()

		if p.options.ts.Parse && alias.String == "type" && p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				if p.lexer.IsContextualKeyword("as") {
					alias = p.parseClauseAlias("export")
					aliasLoc = p.lexer.Loc()
					p.lexer.Next()

					if p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
						// "export { type as as as }"
						// "export { type as as foo }"
						// "export { type as as 'foo' }"
						p.parseClauseAlias("export")
						p.lexer.Next()
					} else {
						// "export { type as as }"
						items = append(items, js_ast.ClauseItem{
							Alias:        alias.String,
							AliasLoc:     aliasLoc,
							Name:         name,
							OriginalName: originalName.String,
						})
					}
				} else if p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
					// "export { type as xxx }"
					// "export { type as 'xxx' }"
					alias = p.parseClauseAlias("export")
					aliasLoc = p.lexer.Loc()
					p.lexer.Next()

					items = append(items, js_ast.ClauseItem{
						Alias:        alias.String,
						AliasLoc:     aliasLoc,
						Name:         name,
						OriginalName: originalName.String,
					})
				}
			} else {
				// The name can actually be a keyword if we're really an "export from"
				// statement. However, we won't know until later. Allow keywords as
				// identifiers for now and throw an error later if there's no "from".
				//
				//   // This is fine
				//   export { type default } from 'path'
				//
				//   // This is a syntax error
				//   export { type default }
				//
				if p.lexer.Token != js_lexer.TIdentifier && firstNonIdentifierLoc.Start == 0 {
					firstNonIdentifierLoc = p.lexer.Loc()
				}

				// "export { type xx }"
				// "export { type xx as yy }"
				// "export { type xx as if }"
				// "export { type default } from 'path'"
				// "export { type default as if } from 'path'"
				// "export { type xx as 'yy' }"
				// "export { type 'xx' } from 'mod'"
				p.parseClauseAlias("export")
				p.lexer.Next()

				if p.lexer.IsContextualKeyword("as") {
					p.lexer.Next()
					p.parseClauseAlias("export")
					p.lexer.Next()
				}
			}
		} else {
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				alias = p.parseClauseAlias("export")
				aliasLoc = p.lexer.Loc()
				p.lexer.Next()
			}

			items = append(items, js_ast.ClauseItem{
				Alias:        alias.String,
				AliasLoc:     aliasLoc,
				Name:         name,
				OriginalName: originalName.String,
			})
		}

		if p.lexer.Token != js_lexer.TComma {
			break
		}
		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		p.lexer.Next()
		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
	}

	if p.lexer.HasNewlineBefore {
		isSingleLine = false
	}
	p.lexer.Expect(js_lexer.TCloseBrace)

	// Throw an error here if we found a keyword earlier and this isn't an
	// "export from" statement after all
	if firstNonIdentifierLoc.Start != 0 && !p.lexer.IsContextualKeyword("from") {
		r := js_lexer.RangeOfIdentifier(p.source, firstNonIdentifierLoc)
		p.log.AddError(&p.tracker, r, fmt.Sprintf("Expected identifier but found %q", p.source.TextForRange(r)))
		panic(js_lexer.LexerPanic{})
	}

	return items, isSingleLine
}

type parseBindingOpts struct {
	isUsingStmt bool
}

func (p *parser) parseBinding(opts parseBindingOpts) js_ast.Binding {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case js_lexer.TIdentifier:
		name := p.lexer.Identifier

		// Forbid invalid identifiers
		if (p.fnOrArrowDataParse.await != allowIdent && name.String == "await") ||
			(p.fnOrArrowDataParse.yield != allowIdent && name.String == "yield") {
			p.log.AddError(&p.tracker, p.lexer.Range(), fmt.Sprintf("Cannot use %q as an identifier here:", name.String))
		}

		ref := p.storeNameInRef(name)
		p.lexer.Next()
		return js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ref}}

	case js_lexer.TOpenBracket:
		if opts.isUsingStmt {
			break
		}
		p.markSyntaxFeature(compat.Destructuring, p.lexer.Range())
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []js_ast.ArrayBinding{}
		hasSpread := false

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != js_lexer.TCloseBracket {
			itemLoc := p.saveExprCommentsHere()

			if p.lexer.Token == js_lexer.TComma {
				binding := js_ast.Binding{Loc: itemLoc, Data: js_ast.BMissingShared}
				items = append(items, js_ast.ArrayBinding{
					Binding: binding,
					Loc:     itemLoc,
				})
			} else {
				if p.lexer.Token == js_lexer.TDotDotDot {
					p.lexer.Next()
					hasSpread = true

					// This was a bug in the ES2015 spec that was fixed in ES2016
					if p.lexer.Token != js_lexer.TIdentifier {
						p.markSyntaxFeature(compat.NestedRestBinding, p.lexer.Range())
					}
				}

				p.saveExprCommentsHere()
				binding := p.parseBinding(parseBindingOpts{})

				var defaultValueOrNil js_ast.Expr
				if !hasSpread && p.lexer.Token == js_lexer.TEquals {
					p.lexer.Next()
					defaultValueOrNil = p.parseExpr(js_ast.LComma)
				}

				items = append(items, js_ast.ArrayBinding{
					Binding:           binding,
					DefaultValueOrNil: defaultValueOrNil,
					Loc:               itemLoc,
				})

				// Commas after spread elements are not allowed
				if hasSpread && p.lexer.Token == js_lexer.TComma {
					p.log.AddError(&p.tracker, p.lexer.Range(), "Unexpected \",\" after rest pattern")
					panic(js_lexer.LexerPanic{})
				}
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
			p.lexer.Next()
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
		}

		p.allowIn = oldAllowIn

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		closeBracketLoc := p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBracket)
		return js_ast.Binding{Loc: loc, Data: &js_ast.BArray{
			Items:           items,
			HasSpread:       hasSpread,
			IsSingleLine:    isSingleLine,
			CloseBracketLoc: closeBracketLoc,
		}}

	case js_lexer.TOpenBrace:
		if opts.isUsingStmt {
			break
		}
		p.markSyntaxFeature(compat.Destructuring, p.lexer.Range())
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []js_ast.PropertyBinding{}

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != js_lexer.TCloseBrace {
			p.saveExprCommentsHere()
			property := p.parsePropertyBinding()
			properties = append(properties, property)

			// Commas after spread elements are not allowed
			if property.IsSpread && p.lexer.Token == js_lexer.TComma {
				p.log.AddError(&p.tracker, p.lexer.Range(), "Unexpected \",\" after rest pattern")
				panic(js_lexer.LexerPanic{})
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
			p.lexer.Next()
			if p.lexer.HasNewlineBefore {
				isSingleLine = false
			}
		}

		p.allowIn = oldAllowIn

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		closeBraceLoc := p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBrace)
		return js_ast.Binding{Loc: loc, Data: &js_ast.BObject{
			Properties:    properties,
			IsSingleLine:  isSingleLine,
			CloseBraceLoc: closeBraceLoc,
		}}
	}

	p.lexer.Expect(js_lexer.TIdentifier)
	return js_ast.Binding{}
}

func (p *parser) parseFn(
	name *ast.LocRef,
	classKeyword logger.Range,
	decoratorContext decoratorContextFlags,
	data fnOrArrowDataParse,
) (fn js_ast.Fn, hadBody bool) {
	fn.Name = name
	fn.HasRestArg = false
	fn.IsAsync = data.await == allowExpr
	fn.IsGenerator = data.yield == allowExpr
	fn.ArgumentsRef = ast.InvalidRef
	fn.OpenParenLoc = p.lexer.Loc()
	p.lexer.Expect(js_lexer.TOpenParen)

	// Await and yield are not allowed in function arguments
	oldFnOrArrowData := p.fnOrArrowDataParse
	if data.await == allowExpr {
		p.fnOrArrowDataParse.await = forbidAll
	} else {
		p.fnOrArrowDataParse.await = allowIdent
	}
	if data.yield == allowExpr {
		p.fnOrArrowDataParse.yield = forbidAll
	} else {
		p.fnOrArrowDataParse.yield = allowIdent
	}

	// Don't suggest inserting "async" before anything if "await" is found
	p.fnOrArrowDataParse.needsAsyncLoc.Start = -1

	// If "super" is allowed in the body, it's allowed in the arguments
	p.fnOrArrowDataParse.allowSuperCall = data.allowSuperCall
	p.fnOrArrowDataParse.allowSuperProperty = data.allowSuperProperty

	for p.lexer.Token != js_lexer.TCloseParen {
		// Skip over "this" type annotations
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TThis {
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
			}
			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
			continue
		}

		var decorators []js_ast.Decorator
		if data.decoratorScope != nil {
			oldAwait := p.fnOrArrowDataParse.await
			oldNeedsAsyncLoc := p.fnOrArrowDataParse.needsAsyncLoc

			// While TypeScript parameter decorators are expressions, they are not
			// evaluated where they exist in the code. They are moved to after the
			// class declaration and evaluated there instead. Specifically this
			// TypeScript code:
			//
			//   class Foo {
			//     foo(@bar() baz) {}
			//   }
			//
			// becomes this JavaScript code:
			//
			//   class Foo {
			//     foo(baz) {}
			//   }
			//   __decorate([
			//     __param(0, bar())
			//   ], Foo.prototype, "foo", null);
			//
			// One consequence of this is that whether "await" is allowed or not
			// depends on whether the class declaration itself is inside an "async"
			// function or not. The TypeScript compiler allows code that does this:
			//
			//   async function fn(foo) {
			//     class Foo {
			//       foo(@bar(await foo) baz) {}
			//     }
			//     return Foo
			//   }
			//
			// because that becomes the following valid JavaScript:
			//
			//   async function fn(foo) {
			//     class Foo {
			//       foo(baz) {}
			//     }
			//     __decorate([
			//       __param(0, bar(await foo))
			//     ], Foo.prototype, "foo", null);
			//     return Foo;
			//   }
			//
			if oldFnOrArrowData.await == allowExpr {
				p.fnOrArrowDataParse.await = allowExpr
			} else {
				p.fnOrArrowDataParse.needsAsyncLoc = oldFnOrArrowData.needsAsyncLoc
			}

			decorators = p.parseDecorators(data.decoratorScope, classKeyword, decoratorContext|decoratorInFnArgs)

			p.fnOrArrowDataParse.await = oldAwait
			p.fnOrArrowDataParse.needsAsyncLoc = oldNeedsAsyncLoc
		}

		if !fn.HasRestArg && p.lexer.Token == js_lexer.TDotDotDot {
			p.markSyntaxFeature(compat.RestArgument, p.lexer.Range())
			p.lexer.Next()
			fn.HasRestArg = true
		}

		isTypeScriptCtorField := false
		isIdentifier := p.lexer.Token == js_lexer.TIdentifier
		text := p.lexer.Identifier.String
		arg := p.parseBinding(parseBindingOpts{})

		if p.options.ts.Parse {
			// Skip over TypeScript accessibility modifiers, which turn this argument
			// into a class field when used inside a class constructor. This is known
			// as a "parameter property" in TypeScript.
			if isIdentifier && data.isConstructor {
				for p.lexer.Token == js_lexer.TIdentifier || p.lexer.Token == js_lexer.TOpenBrace || p.lexer.Token == js_lexer.TOpenBracket {
					if text != "public" && text != "private" && text != "protected" && text != "readonly" && text != "override" {
						break
					}
					isTypeScriptCtorField = true

					// TypeScript requires an identifier binding
					if p.lexer.Token != js_lexer.TIdentifier {
						p.lexer.Expect(js_lexer.TIdentifier)
					}
					text = p.lexer.Identifier.String

					// Re-parse the binding (the current binding is the TypeScript keyword)
					arg = p.parseBinding(parseBindingOpts{})
				}
			}

			// "function foo(a?) {}"
			if p.lexer.Token == js_lexer.TQuestion {
				p.lexer.Next()
			}

			// "function foo(a: any) {}"
			if p.lexer.Token == js_lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
			}
		}

		p.declareBinding(ast.SymbolHoisted, arg, parseStmtOpts{})

		var defaultValueOrNil js_ast.Expr
		if !fn.HasRestArg && p.lexer.Token == js_lexer.TEquals {
			p.markSyntaxFeature(compat.DefaultArgument, p.lexer.Range())
			p.lexer.Next()
			defaultValueOrNil = p.parseExpr(js_ast.LComma)
		}

		fn.Args = append(fn.Args, js_ast.Arg{
			Decorators:   decorators,
			Binding:      arg,
			DefaultOrNil: defaultValueOrNil,

			// We need to track this because it affects code generation
			IsTypeScriptCtorField: isTypeScriptCtorField,
		})

		if p.lexer.Token != js_lexer.TComma {
			break
		}
		if fn.HasRestArg {
			// JavaScript does not allow a comma after a rest argument
			if data.isTypeScriptDeclare {
				// TypeScript does allow a comma after a rest argument in a "declare" context
				p.lexer.Next()
			} else {
				p.lexer.Expect(js_lexer.TCloseParen)
			}
			break
		}
		p.lexer.Next()
	}

	// Reserve the special name "arguments" in this scope. This ensures that it
	// shadows any variable called "arguments" in any parent scopes. But only do
	// this if it wasn't already declared above because arguments are allowed to
	// be called "arguments", in which case the real "arguments" is inaccessible.
	if _, ok := p.currentScope.Members["arguments"]; !ok {
		fn.ArgumentsRef = p.declareSymbol(ast.SymbolArguments, fn.OpenParenLoc, "arguments")
		p.symbols[fn.ArgumentsRef.InnerIndex].Flags |= ast.MustNotBeRenamed
	}

	p.lexer.Expect(js_lexer.TCloseParen)
	p.fnOrArrowDataParse = oldFnOrArrowData

	// "function foo(): any {}"
	if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon {
		p.lexer.Next()
		p.skipTypeScriptReturnType()
	}

	// "function foo(): any;"
	if data.allowMissingBodyForTypeScript && p.lexer.Token != js_lexer.TOpenBrace {
		p.lexer.ExpectOrInsertSemicolon()
		return
	}

	fn.Body = p.parseFnBody(data)
	hadBody = true
	return
}

type fnKind uint8

const (
	fnStmt fnKind = iota
	fnExpr
)

func (p *parser) validateFunctionName(fn js_ast.Fn, kind fnKind) {
	// Prevent the function name from being the same as a function-specific keyword
	if fn.Name != nil {
		if fn.IsAsync && p.symbols[fn.Name.Ref.InnerIndex].OriginalName == "await" {
			p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, fn.Name.Loc),
				"An async function cannot be named \"await\"")
		} else if fn.IsGenerator && p.symbols[fn.Name.Ref.InnerIndex].OriginalName == "yield" && kind == fnExpr {
			p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, fn.Name.Loc),
				"A generator function expression cannot be named \"yield\"")
		}
	}
}

func (p *parser) validateDeclaredSymbolName(loc logger.Loc, name string) {
	if js_lexer.StrictModeReservedWords[name] {
		p.markStrictModeFeature(reservedWord, js_lexer.RangeOfIdentifier(p.source, loc), name)
	} else if isEvalOrArguments(name) {
		p.markStrictModeFeature(evalOrArguments, js_lexer.RangeOfIdentifier(p.source, loc), name)
	}
}

func (p *parser) parseClassStmt(loc logger.Loc, opts parseStmtOpts) js_ast.Stmt {
	var name *ast.LocRef
	classKeyword := p.lexer.Range()
	if p.lexer.Token == js_lexer.TClass {
		p.markSyntaxFeature(compat.Class, classKeyword)
		p.lexer.Next()
	} else {
		p.lexer.Expected(js_lexer.TClass)
	}

	if !opts.isNameOptional || (p.lexer.Token == js_lexer.TIdentifier && (!p.options.ts.Parse || p.lexer.Identifier.String != "implements")) {
		nameLoc := p.lexer.Loc()
		nameText := p.lexer.Identifier.String
		p.lexer.Expect(js_lexer.TIdentifier)
		if p.fnOrArrowDataParse.await != allowIdent && nameText == "await" {
			p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, nameLoc), "Cannot use \"await\" as an identifier here:")
		}
		name = &ast.LocRef{Loc: nameLoc, Ref: ast.InvalidRef}
		if !opts.isTypeScriptDeclare {
			name.Ref = p.declareSymbol(ast.SymbolClass, nameLoc, nameText)
		}
	}

	// Even anonymous classes can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters(allowInOutVarianceAnnotations | allowConstModifier)
	}

	classOpts := parseClassOpts{
		isTypeScriptDeclare: opts.isTypeScriptDeclare,
	}
	if opts.deferredDecorators != nil {
		classOpts.decorators = opts.deferredDecorators.decorators
	}
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeClassName, loc)
	class := p.parseClass(classKeyword, name, classOpts)

	if opts.isTypeScriptDeclare {
		p.popAndDiscardScope(scopeIndex)

		if opts.isNamespaceScope && opts.isExport {
			p.hasNonLocalExportDeclareInsideNamespace = true
		}

		// Remember that this was a "declare class" so we can allow decorators on it
		return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptSharedWasDeclareClass}
	}

	p.popScope()
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SClass{Class: class, IsExport: opts.isExport}}
}

func (p *parser) parseClassExpr(decorators []js_ast.Decorator) js_ast.Expr {
	classKeyword := p.lexer.Range()
	p.markSyntaxFeature(compat.Class, classKeyword)
	p.lexer.Expect(js_lexer.TClass)
	var name *ast.LocRef

	opts := parseClassOpts{
		decorators:       decorators,
		decoratorContext: decoratorInClassExpr,
	}
	p.pushScopeForParsePass(js_ast.ScopeClassName, classKeyword.Loc)

	// Parse an optional class name
	if p.lexer.Token == js_lexer.TIdentifier {
		if nameText := p.lexer.Identifier.String; !p.options.ts.Parse || nameText != "implements" {
			if p.fnOrArrowDataParse.await != allowIdent && nameText == "await" {
				p.log.AddError(&p.tracker, p.lexer.Range(), "Cannot use \"await\" as an identifier here:")
			}
			name = &ast.LocRef{Loc: p.lexer.Loc(), Ref: p.newSymbol(ast.SymbolOther, nameText)}
			p.lexer.Next()
		}
	}

	// Even anonymous classes can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters(allowInOutVarianceAnnotations | allowConstModifier)
	}

	class := p.parseClass(classKeyword, name, opts)

	p.popScope()
	return js_ast.Expr{Loc: classKeyword.Loc, Data: &js_ast.EClass{Class: class}}
}

type parseClassOpts struct {
	decorators          []js_ast.Decorator
	decoratorContext    decoratorContextFlags
	isTypeScriptDeclare bool
}

// By the time we call this, the identifier and type parameters have already
// been parsed. We need to start parsing from the "extends" clause.
func (p *parser) parseClass(classKeyword logger.Range, name *ast.LocRef, classOpts parseClassOpts) js_ast.Class {
	var extendsOrNil js_ast.Expr

	if p.lexer.Token == js_lexer.TExtends {
		p.lexer.Next()
		extendsOrNil = p.parseExpr(js_ast.LNew)

		// TypeScript's type argument parser inside expressions backtracks if the
		// first token after the end of the type parameter list is "{", so the
		// parsed expression above will have backtracked if there are any type
		// arguments. This means we have to re-parse for any type arguments here.
		// This seems kind of wasteful to me but it's what the official compiler
		// does and it probably doesn't have that high of a performance overhead
		// because "extends" clauses aren't that frequent, so it should be ok.
		if p.options.ts.Parse {
			p.skipTypeScriptTypeArguments(skipTypeScriptTypeArgumentsOpts{})
		}
	}

	if p.options.ts.Parse && p.lexer.IsContextualKeyword("implements") {
		p.lexer.Next()
		for {
			p.skipTypeScriptType(js_ast.LLowest)
			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}
	}

	bodyLoc := p.lexer.Loc()
	p.lexer.Expect(js_lexer.TOpenBrace)
	properties := []js_ast.Property{}
	hasPropertyDecorator := false

	// Allow "in" and private fields inside class bodies
	oldAllowIn := p.allowIn
	oldAllowPrivateIdentifiers := p.allowPrivateIdentifiers
	p.allowIn = true
	p.allowPrivateIdentifiers = true

	// A scope is needed for private identifiers
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeClassBody, bodyLoc)

	opts := propertyOpts{
		isClass:          true,
		decoratorScope:   p.currentScope,
		decoratorContext: classOpts.decoratorContext,
		classHasExtends:  extendsOrNil.Data != nil,
		classKeyword:     classKeyword,
	}
	hasConstructor := false

	for p.lexer.Token != js_lexer.TCloseBrace {
		if p.lexer.Token == js_lexer.TSemicolon {
			p.lexer.Next()
			continue
		}

		// Parse decorators for this property
		firstDecoratorLoc := p.lexer.Loc()
		scopeIndex := len(p.scopesInOrder)
		opts.decorators = p.parseDecorators(p.currentScope, classKeyword, opts.decoratorContext)
		if len(opts.decorators) > 0 {
			hasPropertyDecorator = true
		}

		// This property may turn out to be a type in TypeScript, which should be ignored
		if property, ok := p.parseProperty(p.saveExprCommentsHere(), js_ast.PropertyField, opts, nil); ok {
			properties = append(properties, property)

			// Forbid decorators on class constructors
			if key, ok := property.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(key.Value, "constructor") {
				if len(opts.decorators) > 0 {
					p.log.AddError(&p.tracker, logger.Range{Loc: firstDecoratorLoc},
						"Decorators are not allowed on class constructors")
				}
				if property.Kind.IsMethodDefinition() && !property.Flags.Has(js_ast.PropertyIsStatic) && !property.Flags.Has(js_ast.PropertyIsComputed) {
					if hasConstructor {
						p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, property.Key.Loc),
							"Classes cannot contain more than one constructor")
					}
					hasConstructor = true
				}
			}
		} else if !classOpts.isTypeScriptDeclare && len(opts.decorators) > 0 {
			p.log.AddError(&p.tracker, logger.Range{Loc: firstDecoratorLoc, Len: 1}, "Decorators are not valid here")
			p.discardScopesUpTo(scopeIndex)
		}
	}

	// Discard the private identifier scope inside a TypeScript "declare class"
	if classOpts.isTypeScriptDeclare {
		p.popAndDiscardScope(scopeIndex)
	} else {
		p.popScope()
	}

	p.allowIn = oldAllowIn
	p.allowPrivateIdentifiers = oldAllowPrivateIdentifiers

	closeBraceLoc := p.saveExprCommentsHere()
	p.lexer.Expect(js_lexer.TCloseBrace)

	// TypeScript has legacy behavior that uses assignment semantics instead of
	// define semantics for class fields when "useDefineForClassFields" is enabled
	// (in which case TypeScript behaves differently than JavaScript, which is
	// arguably "wrong").
	//
	// This legacy behavior exists because TypeScript added class fields to
	// TypeScript before they were added to JavaScript. They decided to go with
	// assignment semantics for whatever reason. Later on TC39 decided to go with
	// define semantics for class fields instead. This behaves differently if the
	// base class has a setter with the same name.
	//
	// The value of "useDefineForClassFields" defaults to false when it's not
	// specified and the target is earlier than "ES2022" since the class field
	// language feature was added in ES2022. However, TypeScript's "target"
	// setting currently defaults to "ES3" which unfortunately means that the
	// "useDefineForClassFields" setting defaults to false (i.e. to "wrong").
	//
	// We default "useDefineForClassFields" to true (i.e. to "correct") instead.
	// This is partially because our target defaults to "esnext", and partially
	// because this is a legacy behavior that no one should be using anymore.
	// Users that want the wrong behavior can either set "useDefineForClassFields"
	// to false in "tsconfig.json" explicitly, or set TypeScript's "target" to
	// "ES2021" or earlier in their in "tsconfig.json" file.
	useDefineForClassFields := !p.options.ts.Parse || p.options.ts.Config.UseDefineForClassFields == config.True ||
		(p.options.ts.Config.UseDefineForClassFields == config.Unspecified && p.options.ts.Config.Target != config.TSTargetBelowES2022)

	return js_ast.Class{
		ClassKeyword:  classKeyword,
		Decorators:    classOpts.decorators,
		Name:          name,
		ExtendsOrNil:  extendsOrNil,
		BodyLoc:       bodyLoc,
		Properties:    properties,
		CloseBraceLoc: closeBraceLoc,

		// Always lower standard decorators if they are present and TypeScript's
		// "useDefineForClassFields" setting is false even if the configured target
		// environment supports decorators. This setting changes the behavior of
		// class fields, and so we must lower decorators so they behave correctly.
		ShouldLowerStandardDecorators: (len(classOpts.decorators) > 0 || hasPropertyDecorator) &&
			((!p.options.ts.Parse && p.options.unsupportedJSFeatures.Has(compat.Decorators)) ||
				(p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators != config.True &&
					(p.options.unsupportedJSFeatures.Has(compat.Decorators) || !useDefineForClassFields))),

		UseDefineForClassFields: useDefineForClassFields,
	}
}

func (p *parser) parseLabelName() *ast.LocRef {
	if p.lexer.Token != js_lexer.TIdentifier || p.lexer.HasNewlineBefore {
		return nil
	}

	name := ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(p.lexer.Identifier)}
	p.lexer.Next()
	return &name
}

func (p *parser) parsePath() (logger.Range, string, *ast.ImportAssertOrWith, ast.ImportRecordFlags) {
	var flags ast.ImportRecordFlags
	pathRange := p.lexer.Range()
	pathText := helpers.UTF16ToString(p.lexer.StringLiteral())
	if p.lexer.Token == js_lexer.TNoSubstitutionTemplateLiteral {
		p.lexer.Next()
	} else {
		p.lexer.Expect(js_lexer.TStringLiteral)
	}

	// See https://github.com/tc39/proposal-import-attributes for more info
	var assertOrWith *ast.ImportAssertOrWith
	if p.lexer.Token == js_lexer.TWith || (!p.lexer.HasNewlineBefore && p.lexer.IsContextualKeyword("assert")) {
		// "import './foo.json' assert { type: 'json' }"
		// "import './foo.json' with { type: 'json' }"
		var entries []ast.AssertOrWithEntry
		duplicates := make(map[string]logger.Range)
		keyword := ast.WithKeyword
		if p.lexer.Token != js_lexer.TWith {
			keyword = ast.AssertKeyword
		}
		keywordLoc := p.saveExprCommentsHere()
		p.lexer.Next()
		openBraceLoc := p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TOpenBrace)

		for p.lexer.Token != js_lexer.TCloseBrace {
			// Parse the key
			keyLoc := p.saveExprCommentsHere()
			preferQuotedKey := false
			var key []uint16
			var keyText string
			if p.lexer.IsIdentifierOrKeyword() {
				keyText = p.lexer.Identifier.String
				key = helpers.StringToUTF16(keyText)
			} else if p.lexer.Token == js_lexer.TStringLiteral {
				key = p.lexer.StringLiteral()
				keyText = helpers.UTF16ToString(key)
				preferQuotedKey = !p.options.minifySyntax
			} else {
				p.lexer.Expect(js_lexer.TIdentifier)
			}
			if prevRange, ok := duplicates[keyText]; ok {
				what := "attribute"
				if keyword == ast.AssertKeyword {
					what = "assertion"
				}
				p.log.AddErrorWithNotes(&p.tracker, p.lexer.Range(), fmt.Sprintf("Duplicate import %s %q", what, keyText),
					[]logger.MsgData{p.tracker.MsgData(prevRange, fmt.Sprintf("The first %q was here:", keyText))})
			}
			duplicates[keyText] = p.lexer.Range()
			p.lexer.Next()
			p.lexer.Expect(js_lexer.TColon)

			// Parse the value
			valueLoc := p.saveExprCommentsHere()
			value := p.lexer.StringLiteral()
			p.lexer.Expect(js_lexer.TStringLiteral)

			entries = append(entries, ast.AssertOrWithEntry{
				Key:             key,
				KeyLoc:          keyLoc,
				Value:           value,
				ValueLoc:        valueLoc,
				PreferQuotedKey: preferQuotedKey,
			})

			// Using "assert: { type: 'json' }" triggers special behavior
			if keyword == ast.AssertKeyword && helpers.UTF16EqualsString(key, "type") && helpers.UTF16EqualsString(value, "json") {
				flags |= ast.AssertTypeJSON
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		closeBraceLoc := p.saveExprCommentsHere()
		p.lexer.Expect(js_lexer.TCloseBrace)
		if keyword == ast.AssertKeyword {
			p.maybeWarnAboutAssertKeyword(keywordLoc)
		}
		assertOrWith = &ast.ImportAssertOrWith{
			Entries:            entries,
			Keyword:            keyword,
			KeywordLoc:         keywordLoc,
			InnerOpenBraceLoc:  openBraceLoc,
			InnerCloseBraceLoc: closeBraceLoc,
		}
	}

	return pathRange, pathText, assertOrWith, flags
}

// Let people know if they probably should be using "with" instead of "assert"
func (p *parser) maybeWarnAboutAssertKeyword(loc logger.Loc) {
	if p.options.unsupportedJSFeatures.Has(compat.ImportAssertions) && !p.options.unsupportedJSFeatures.Has(compat.ImportAttributes) {
		where := config.PrettyPrintTargetEnvironment(p.options.originalTargetEnv, p.options.unsupportedJSFeatureOverridesMask)
		msg := logger.Msg{
			Kind:  logger.Warning,
			Data:  p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, loc), "The \"assert\" keyword is not supported in "+where),
			Notes: []logger.MsgData{{Text: "Did you mean to use \"with\" instead of \"assert\"?"}},
		}
		msg.Data.Location.Suggestion = "with"
		p.log.AddMsgID(logger.MsgID_JS_AssertToWith, msg)
	}
}

// This assumes the "function" token has already been parsed
func (p *parser) parseFnStmt(loc logger.Loc, opts parseStmtOpts, isAsync bool, asyncRange logger.Range) js_ast.Stmt {
	isGenerator := p.lexer.Token == js_lexer.TAsterisk
	hasError := false
	if isAsync {
		hasError = p.markAsyncFn(asyncRange, isGenerator)
	}
	if isGenerator {
		if !hasError {
			p.markSyntaxFeature(compat.Generator, p.lexer.Range())
		}
		p.lexer.Next()
	}

	switch opts.lexicalDecl {
	case lexicalDeclForbid:
		p.forbidLexicalDecl(loc)

	// Allow certain function statements in certain single-statement contexts
	case lexicalDeclAllowFnInsideIf, lexicalDeclAllowFnInsideLabel:
		if opts.isTypeScriptDeclare || isGenerator || isAsync {
			p.forbidLexicalDecl(loc)
		}
	}

	var name *ast.LocRef
	var nameText string

	// The name is optional for "export default function() {}" pseudo-statements
	if !opts.isNameOptional || p.lexer.Token == js_lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		nameText = p.lexer.Identifier.String
		if !isAsync && p.fnOrArrowDataParse.await != allowIdent && nameText == "await" {
			p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, nameLoc), "Cannot use \"await\" as an identifier here:")
		}
		p.lexer.Expect(js_lexer.TIdentifier)
		name = &ast.LocRef{Loc: nameLoc, Ref: ast.InvalidRef}
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters(allowConstModifier)
	}

	// Introduce a fake block scope for function declarations inside if statements
	var ifStmtScopeIndex int
	hasIfScope := opts.lexicalDecl == lexicalDeclAllowFnInsideIf
	if hasIfScope {
		ifStmtScopeIndex = p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
	}

	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, p.lexer.Loc())

	await := allowIdent
	yield := allowIdent
	if isAsync {
		await = allowExpr
	}
	if isGenerator {
		yield = allowExpr
	}

	fn, hadBody := p.parseFn(name, logger.Range{}, 0, fnOrArrowDataParse{
		needsAsyncLoc:       loc,
		asyncRange:          asyncRange,
		await:               await,
		yield:               yield,
		isTypeScriptDeclare: opts.isTypeScriptDeclare,

		// Only allow omitting the body if we're parsing TypeScript
		allowMissingBodyForTypeScript: p.options.ts.Parse,
	})

	// Don't output anything if it's just a forward declaration of a function
	if opts.isTypeScriptDeclare || !hadBody {
		p.popAndDiscardScope(scopeIndex)

		// Balance the fake block scope introduced above
		if hasIfScope {
			p.popAndDiscardScope(ifStmtScopeIndex)
		}

		if opts.isTypeScriptDeclare && opts.isNamespaceScope && opts.isExport {
			p.hasNonLocalExportDeclareInsideNamespace = true
		}

		return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
	}

	p.popScope()

	// Only declare the function after we know if it had a body or not. Otherwise
	// TypeScript code such as this will double-declare the symbol:
	//
	//     function foo(): void;
	//     function foo(): void {}
	//
	if name != nil {
		kind := ast.SymbolHoistedFunction
		if isGenerator || isAsync {
			kind = ast.SymbolGeneratorOrAsyncFunction
		}
		name.Ref = p.declareSymbol(kind, name.Loc, nameText)
	}

	// Balance the fake block scope introduced above
	if hasIfScope {
		p.popScope()
	}

	fn.HasIfScope = hasIfScope
	p.validateFunctionName(fn, fnStmt)
	if opts.hasNoSideEffectsComment && !p.options.ignoreDCEAnnotations {
		fn.HasNoSideEffectsComment = true
		if name != nil && !opts.isTypeScriptDeclare {
			p.symbols[name.Ref.InnerIndex].Flags |= ast.CallCanBeUnwrappedIfUnused
		}
	}
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SFunction{Fn: fn, IsExport: opts.isExport}}
}

type deferredDecorators struct {
	decorators []js_ast.Decorator
}

type decoratorContextFlags uint8

const (
	decoratorBeforeClassExpr = 1 << iota
	decoratorInClassExpr
	decoratorInFnArgs
)

func (p *parser) parseDecorators(decoratorScope *js_ast.Scope, classKeyword logger.Range, context decoratorContextFlags) (decorators []js_ast.Decorator) {
	if p.lexer.Token == js_lexer.TAt {
		if p.options.ts.Parse {
			if p.options.ts.Config.ExperimentalDecorators == config.True {
				if (context & decoratorInClassExpr) != 0 {
					p.lexer.AddRangeErrorWithNotes(p.lexer.Range(), "TypeScript experimental decorators can only be used with class declarations",
						[]logger.MsgData{p.tracker.MsgData(classKeyword, "This is a class expression, not a class declaration:")})
				} else if (context & decoratorBeforeClassExpr) != 0 {
					p.log.AddError(&p.tracker, p.lexer.Range(), "TypeScript experimental decorators cannot be used in expression position")
				}
			} else {
				if (context&decoratorInFnArgs) != 0 && p.options.ts.Config.ExperimentalDecorators != config.True {
					p.log.AddErrorWithNotes(&p.tracker, p.lexer.Range(), "Parameter decorators only work when experimental decorators are enabled", []logger.MsgData{{
						Text: "You can enable experimental decorators by adding \"experimentalDecorators\": true to your \"tsconfig.json\" file.",
					}})
				}
			}
		} else {
			if (context & decoratorInFnArgs) != 0 {
				p.log.AddError(&p.tracker, p.lexer.Range(), "Parameter decorators are not allowed in JavaScript")
			}
		}
	}

	// TypeScript decorators cause us to temporarily revert to the scope that
	// encloses the class declaration, since that's where the generated code
	// for TypeScript decorators will be inserted.
	oldScope := p.currentScope
	p.currentScope = decoratorScope

	for p.lexer.Token == js_lexer.TAt {
		atLoc := p.lexer.Loc()
		p.lexer.Next()

		var value js_ast.Expr
		if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True {
			// TypeScript's experimental decorator syntax is more permissive than
			// JavaScript. Parse a new/call expression with "exprFlagDecorator" so
			// we ignore EIndex expressions, since they may be part of a computed
			// property:
			//
			//   class Foo {
			//     @foo ['computed']() {}
			//   }
			//
			// This matches the behavior of the TypeScript compiler.
			p.parseExperimentalDecoratorNesting++
			value = p.parseExprWithFlags(js_ast.LNew, exprFlagDecorator)
			p.parseExperimentalDecoratorNesting--
		} else {
			// JavaScript's decorator syntax is more restrictive. Parse it using a
			// special parser that doesn't allow normal expressions (e.g. "?.").
			value = p.parseDecorator()
		}
		decorators = append(decorators, js_ast.Decorator{
			Value:            value,
			AtLoc:            atLoc,
			OmitNewlineAfter: !p.lexer.HasNewlineBefore,
		})
	}

	// Avoid "popScope" because this decorator scope is not hierarchical
	p.currentScope = oldScope
	return decorators
}

func (p *parser) parseDecorator() js_ast.Expr {
	if p.lexer.Token == js_lexer.TOpenParen {
		p.lexer.Next()
		value := p.parseExpr(js_ast.LLowest)
		p.lexer.Expect(js_lexer.TCloseParen)
		return value
	}

	name := p.lexer.Identifier
	nameRange := p.lexer.Range()
	p.lexer.Expect(js_lexer.TIdentifier)

	// Forbid invalid identifiers
	if (p.fnOrArrowDataParse.await != allowIdent && name.String == "await") ||
		(p.fnOrArrowDataParse.yield != allowIdent && name.String == "yield") {
		p.log.AddError(&p.tracker, nameRange, fmt.Sprintf("Cannot use %q as an identifier here:", name.String))
	}

	memberExpr := js_ast.Expr{Loc: nameRange.Loc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef(name)}}

	// Custom error reporting for error recovery
	var syntaxError logger.MsgData
	wrapRange := nameRange

loop:
	for {
		switch p.lexer.Token {
		case js_lexer.TExclamation:
			// Skip over TypeScript non-null assertions
			if p.lexer.HasNewlineBefore {
				break loop
			}
			if !p.options.ts.Parse {
				p.lexer.Unexpected()
			}
			wrapRange.Len = p.lexer.Range().End() - wrapRange.Loc.Start
			p.lexer.Next()

		case js_lexer.TDot, js_lexer.TQuestionDot:
			// The grammar for "DecoratorMemberExpression" currently forbids "?."
			if p.lexer.Token == js_lexer.TQuestionDot && syntaxError.Location == nil {
				syntaxError = p.tracker.MsgData(p.lexer.Range(), "JavaScript decorator syntax does not allow \"?.\" here")
			}

			p.lexer.Next()
			wrapRange.Len = p.lexer.Range().End() - wrapRange.Loc.Start

			if p.lexer.Token == js_lexer.TPrivateIdentifier {
				name := p.lexer.Identifier
				memberExpr.Data = &js_ast.EIndex{
					Target: memberExpr,
					Index:  js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EPrivateIdentifier{Ref: p.storeNameInRef(name)}},
				}
				p.reportPrivateNameUsage(name.String)
				p.lexer.Next()
			} else {
				memberExpr.Data = &js_ast.EDot{
					Target:  memberExpr,
					Name:    p.lexer.Identifier.String,
					NameLoc: p.lexer.Loc(),
				}
				p.lexer.Expect(js_lexer.TIdentifier)
			}

		case js_lexer.TOpenParen:
			args, closeParenLoc, isMultiLine := p.parseCallArgs()
			memberExpr.Data = &js_ast.ECall{
				Target:        memberExpr,
				Args:          args,
				CloseParenLoc: closeParenLoc,
				IsMultiLine:   isMultiLine,
				Kind:          js_ast.TargetWasOriginallyPropertyAccess,
			}
			wrapRange.Len = closeParenLoc.Start + 1 - wrapRange.Loc.Start

			// The grammar for "DecoratorCallExpression" currently forbids anything after it
			if p.lexer.Token == js_lexer.TDot {
				if syntaxError.Location == nil {
					syntaxError = p.tracker.MsgData(p.lexer.Range(), "JavaScript decorator syntax does not allow \".\" after a call expression")
				}
				continue
			}
			break loop

		default:
			// "@x<y>"
			// "@x.y<z>"
			if !p.skipTypeScriptTypeArguments(skipTypeScriptTypeArgumentsOpts{}) {
				break loop
			}
		}
	}

	// Suggest that non-decorator expressions be wrapped in parentheses
	if syntaxError.Location != nil {
		var notes []logger.MsgData
		if text := p.source.TextForRange(wrapRange); !strings.ContainsRune(text, '\n') {
			note := p.tracker.MsgData(wrapRange, "Wrap this decorator in parentheses to allow arbitrary expressions:")
			note.Location.Suggestion = fmt.Sprintf("(%s)", text)
			notes = []logger.MsgData{note}
		}
		p.log.AddMsg(logger.Msg{
			Kind:  logger.Error,
			Data:  syntaxError,
			Notes: notes,
		})
	}

	return memberExpr
}

type lexicalDecl uint8

const (
	lexicalDeclForbid lexicalDecl = iota
	lexicalDeclAllowAll
	lexicalDeclAllowFnInsideIf
	lexicalDeclAllowFnInsideLabel
)

type parseStmtOpts struct {
	deferredDecorators      *deferredDecorators
	lexicalDecl             lexicalDecl
	isModuleScope           bool
	isNamespaceScope        bool
	isExport                bool
	isExportDefault         bool
	isNameOptional          bool // For "export default" pseudo-statements
	isTypeScriptDeclare     bool
	isForLoopInit           bool
	isForAwaitLoopInit      bool
	allowDirectivePrologue  bool
	hasNoSideEffectsComment bool
	isUsingStmt             bool
}

func (p *parser) parseStmt(opts parseStmtOpts) js_ast.Stmt {
	loc := p.lexer.Loc()

	if (p.lexer.HasCommentBefore & js_lexer.NoSideEffectsCommentBefore) != 0 {
		opts.hasNoSideEffectsComment = true
	}

	// Do not attach any leading comments to the next expression
	p.lexer.CommentsBeforeToken = p.lexer.CommentsBeforeToken[:0]

	switch p.lexer.Token {
	case js_lexer.TSemicolon:
		p.lexer.Next()
		return js_ast.Stmt{Loc: loc, Data: js_ast.SEmptyShared}

	case js_lexer.TExport:
		previousExportKeyword := p.esmExportKeyword
		if opts.isModuleScope {
			p.esmExportKeyword = p.lexer.Range()
		} else if !opts.isNamespaceScope {
			p.lexer.Unexpected()
		}
		p.lexer.Next()

		switch p.lexer.Token {
		case js_lexer.TClass, js_lexer.TConst, js_lexer.TFunction, js_lexer.TVar, js_lexer.TAt:
			opts.isExport = true
			return p.parseStmt(opts)

		case js_lexer.TImport:
			// "export import foo = bar"
			if p.options.ts.Parse && (opts.isModuleScope || opts.isNamespaceScope) {
				opts.isExport = true
				return p.parseStmt(opts)
			}

			p.lexer.Unexpected()
			return js_ast.Stmt{}

		case js_lexer.TEnum:
			if !p.options.ts.Parse {
				p.lexer.Unexpected()
			}
			opts.isExport = true
			return p.parseStmt(opts)

		case js_lexer.TIdentifier:
			if p.lexer.IsContextualKeyword("let") {
				opts.isExport = true
				return p.parseStmt(opts)
			}

			if p.lexer.IsContextualKeyword("as") {
				// "export as namespace ns;"
				p.lexer.Next()
				p.lexer.ExpectContextualKeyword("namespace")
				p.lexer.Expect(js_lexer.TIdentifier)
				p.lexer.ExpectOrInsertSemicolon()
				return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
			}

			if p.lexer.IsContextualKeyword("async") {
				// "export async function foo() {}"
				asyncRange := p.lexer.Range()
				p.lexer.Next()
				if p.lexer.HasNewlineBefore {
					p.log.AddError(&p.tracker, logger.Range{Loc: logger.Loc{Start: asyncRange.End()}},
						"Unexpected newline after \"async\"")
					panic(js_lexer.LexerPanic{})
				}
				p.lexer.Expect(js_lexer.TFunction)
				opts.isExport = true
				return p.parseFnStmt(loc, opts, true /* isAsync */, asyncRange)
			}

			if p.options.ts.Parse {
				switch p.lexer.Identifier.String {
				case "type":
					// "export type foo = ..."
					typeRange := p.lexer.Range()
					p.lexer.Next()
					if p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace && p.lexer.Token != js_lexer.TAsterisk {
						p.log.AddError(&p.tracker, logger.Range{Loc: logger.Loc{Start: typeRange.End()}},
							"Unexpected newline after \"type\"")
						panic(js_lexer.LexerPanic{})
					}
					p.skipTypeScriptTypeStmt(parseStmtOpts{isModuleScope: opts.isModuleScope, isExport: true})
					return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}

				case "namespace", "abstract", "module", "interface":
					// "export namespace Foo {}"
					// "export abstract class Foo {}"
					// "export module Foo {}"
					// "export interface Foo {}"
					opts.isExport = true
					return p.parseStmt(opts)

				case "declare":
					// "export declare class Foo {}"
					opts.isExport = true
					opts.lexicalDecl = lexicalDeclAllowAll
					opts.isTypeScriptDeclare = true
					return p.parseStmt(opts)
				}
			}

			p.lexer.Unexpected()
			return js_ast.Stmt{}

		case js_lexer.TDefault:
			if !opts.isModuleScope && (!opts.isNamespaceScope || !opts.isTypeScriptDeclare) {
				p.lexer.Unexpected()
			}

			defaultLoc := p.lexer.Loc()
			p.lexer.Next()

			// Also pick up comments after the "default" keyword
			if (p.lexer.HasCommentBefore & js_lexer.NoSideEffectsCommentBefore) != 0 {
				opts.hasNoSideEffectsComment = true
			}

			// The default name is lazily generated only if no other name is present
			createDefaultName := func() ast.LocRef {
				// This must be named "default" for when "--keep-names" is active
				defaultName := ast.LocRef{Loc: defaultLoc, Ref: p.newSymbol(ast.SymbolOther, "default")}
				p.currentScope.Generated = append(p.currentScope.Generated, defaultName.Ref)
				return defaultName
			}

			// "export default async function() {}"
			// "export default async function foo() {}"
			if p.lexer.IsContextualKeyword("async") {
				asyncRange := p.lexer.Range()
				p.lexer.Next()

				if p.lexer.Token == js_lexer.TFunction && !p.lexer.HasNewlineBefore {
					p.lexer.Next()
					stmt := p.parseFnStmt(loc, parseStmtOpts{
						isNameOptional:          true,
						lexicalDecl:             lexicalDeclAllowAll,
						hasNoSideEffectsComment: opts.hasNoSideEffectsComment,
					}, true /* isAsync */, asyncRange)
					if _, ok := stmt.Data.(*js_ast.STypeScript); ok {
						return stmt // This was just a type annotation
					}

					// Use the statement name if present, since it's a better name
					var defaultName ast.LocRef
					if s, ok := stmt.Data.(*js_ast.SFunction); ok && s.Fn.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Fn.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}

					return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportDefault{DefaultName: defaultName, Value: stmt}}
				}

				defaultName := createDefaultName()
				expr := p.parseSuffix(p.parseAsyncPrefixExpr(asyncRange, js_ast.LComma, 0), js_ast.LComma, nil, 0)
				p.lexer.ExpectOrInsertSemicolon()
				return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportDefault{
					DefaultName: defaultName, Value: js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}}}}
			}

			// "export default class {}"
			// "export default class Foo {}"
			// "export default @x class {}"
			// "export default @x class Foo {}"
			// "export default function() {}"
			// "export default function foo() {}"
			// "export default interface Foo {}"
			// "export default interface + 1"
			if p.lexer.Token == js_lexer.TFunction || p.lexer.Token == js_lexer.TClass || p.lexer.Token == js_lexer.TAt ||
				(p.options.ts.Parse && p.lexer.IsContextualKeyword("interface")) {
				stmt := p.parseStmt(parseStmtOpts{
					deferredDecorators:      opts.deferredDecorators,
					isNameOptional:          true,
					isExportDefault:         true,
					lexicalDecl:             lexicalDeclAllowAll,
					hasNoSideEffectsComment: opts.hasNoSideEffectsComment,
				})

				// Use the statement name if present, since it's a better name
				var defaultName ast.LocRef
				switch s := stmt.Data.(type) {
				case *js_ast.STypeScript, *js_ast.SExpr:
					return stmt // Handle the "interface" case above
				case *js_ast.SFunction:
					if s.Fn.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Fn.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}
				case *js_ast.SClass:
					if s.Class.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Class.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}
				default:
					panic("Internal error")
				}
				return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportDefault{DefaultName: defaultName, Value: stmt}}
			}

			isIdentifier := p.lexer.Token == js_lexer.TIdentifier
			name := p.lexer.Identifier.String
			expr := p.parseExpr(js_ast.LComma)

			// "export default abstract class {}"
			// "export default abstract class Foo {}"
			if p.options.ts.Parse && isIdentifier && name == "abstract" && !p.lexer.HasNewlineBefore {
				if _, ok := expr.Data.(*js_ast.EIdentifier); ok && p.lexer.Token == js_lexer.TClass {
					stmt := p.parseClassStmt(loc, parseStmtOpts{
						deferredDecorators: opts.deferredDecorators,
						isNameOptional:     true,
					})

					// Use the statement name if present, since it's a better name
					var defaultName ast.LocRef
					if s, ok := stmt.Data.(*js_ast.SClass); ok && s.Class.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Class.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}

					return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportDefault{DefaultName: defaultName, Value: stmt}}
				}
			}

			p.lexer.ExpectOrInsertSemicolon()
			defaultName := createDefaultName()
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportDefault{
				DefaultName: defaultName, Value: js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}}}}

		case js_lexer.TAsterisk:
			if !opts.isModuleScope && (!opts.isNamespaceScope || !opts.isTypeScriptDeclare) {
				p.lexer.Unexpected()
			}

			p.lexer.Next()
			var namespaceRef ast.Ref
			var alias *js_ast.ExportStarAlias
			var pathRange logger.Range
			var pathText string
			var assertOrWith *ast.ImportAssertOrWith
			var flags ast.ImportRecordFlags

			if p.lexer.IsContextualKeyword("as") {
				// "export * as ns from 'path'"
				p.lexer.Next()
				name := p.parseClauseAlias("export")
				namespaceRef = p.storeNameInRef(name)
				alias = &js_ast.ExportStarAlias{Loc: p.lexer.Loc(), OriginalName: name.String}
				p.lexer.Next()
				p.lexer.ExpectContextualKeyword("from")
				pathRange, pathText, assertOrWith, flags = p.parsePath()
			} else {
				// "export * from 'path'"
				p.lexer.ExpectContextualKeyword("from")
				pathRange, pathText, assertOrWith, flags = p.parsePath()
				name := js_ast.GenerateNonUniqueNameFromPath(pathText) + "_star"
				namespaceRef = p.storeNameInRef(js_lexer.MaybeSubstring{String: name})
			}
			importRecordIndex := p.addImportRecord(ast.ImportStmt, pathRange, pathText, assertOrWith, flags)

			// Export-star statements anywhere in the file disable top-level const
			// local prefix because import cycles can be used to trigger TDZ
			p.currentScope.IsAfterConstLocalPrefix = true

			p.lexer.ExpectOrInsertSemicolon()
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportStar{
				NamespaceRef:      namespaceRef,
				Alias:             alias,
				ImportRecordIndex: importRecordIndex,
			}}

		case js_lexer.TOpenBrace:
			if !opts.isModuleScope && (!opts.isNamespaceScope || !opts.isTypeScriptDeclare) {
				p.lexer.Unexpected()
			}

			items, isSingleLine := p.parseExportClause()
			if p.lexer.IsContextualKeyword("from") {
				// "export {} from 'path'"
				p.lexer.Next()
				pathLoc, pathText, assertOrWith, flags := p.parsePath()
				importRecordIndex := p.addImportRecord(ast.ImportStmt, pathLoc, pathText, assertOrWith, flags)
				name := "import_" + js_ast.GenerateNonUniqueNameFromPath(pathText)
				namespaceRef := p.storeNameInRef(js_lexer.MaybeSubstring{String: name})

				// Export clause statements anywhere in the file disable top-level const
				// local prefix because import cycles can be used to trigger TDZ
				p.currentScope.IsAfterConstLocalPrefix = true

				p.lexer.ExpectOrInsertSemicolon()
				return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportFrom{
					Items:             items,
					NamespaceRef:      namespaceRef,
					ImportRecordIndex: importRecordIndex,
					IsSingleLine:      isSingleLine,
				}}
			}

			p.lexer.ExpectOrInsertSemicolon()
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportClause{Items: items, IsSingleLine: isSingleLine}}

		case js_lexer.TEquals:
			// "export = value;"
			p.esmExportKeyword = previousExportKeyword // This wasn't an ESM export statement after all
			if p.options.ts.Parse {
				p.lexer.Next()
				value := p.parseExpr(js_ast.LLowest)
				p.lexer.ExpectOrInsertSemicolon()
				return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportEquals{Value: value}}
			}
			p.lexer.Unexpected()
			return js_ast.Stmt{}

		default:
			p.lexer.Unexpected()
			return js_ast.Stmt{}
		}

	case js_lexer.TFunction:
		p.lexer.Next()
		return p.parseFnStmt(loc, opts, false /* isAsync */, logger.Range{})

	case js_lexer.TEnum:
		if !p.options.ts.Parse {
			p.lexer.Unexpected()
		}
		return p.parseTypeScriptEnumStmt(loc, opts)

	case js_lexer.TAt:
		// Parse decorators before class statements, which are potentially exported
		scopeIndex := len(p.scopesInOrder)
		decorators := p.parseDecorators(p.currentScope, logger.Range{}, 0)

		// "@x export @y class Foo {}"
		if opts.deferredDecorators != nil {
			p.log.AddError(&p.tracker, logger.Range{Loc: loc, Len: 1}, "Decorators are not valid here")
			p.discardScopesUpTo(scopeIndex)
			return p.parseStmt(opts)
		}

		// If this turns out to be a "declare class" statement, we need to undo the
		// scopes that were potentially pushed while parsing the decorator arguments.
		// That can look like any one of the following:
		//
		//   "@decorator declare class Foo {}"
		//   "@decorator declare abstract class Foo {}"
		//   "@decorator export declare class Foo {}"
		//   "@decorator export declare abstract class Foo {}"
		//
		opts.deferredDecorators = &deferredDecorators{
			decorators: decorators,
		}

		stmt := p.parseStmt(opts)

		// Check for valid decorator targets
		switch s := stmt.Data.(type) {
		case *js_ast.SClass:
			return stmt

		case *js_ast.SExportDefault:
			switch s.Value.Data.(type) {
			case *js_ast.SClass:
				return stmt
			}

		case *js_ast.STypeScript:
			if s.WasDeclareClass {
				// If this is a type declaration, discard any scopes that were pushed
				// while parsing decorators. Unlike with the class statements above,
				// these scopes won't end up being visited during the upcoming visit
				// pass because type declarations aren't visited at all.
				p.discardScopesUpTo(scopeIndex)
				return stmt
			}
		}

		// Forbid decorators on anything other than a class statement
		p.log.AddError(&p.tracker, logger.Range{Loc: loc, Len: 1}, "Decorators are not valid here")
		stmt.Data = js_ast.STypeScriptShared
		p.discardScopesUpTo(scopeIndex)
		return stmt

	case js_lexer.TClass:
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(loc)
		}
		return p.parseClassStmt(loc, opts)

	case js_lexer.TVar:
		p.lexer.Next()
		decls := p.parseAndDeclareDecls(ast.SymbolHoisted, opts)
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
			Kind:     js_ast.LocalVar,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case js_lexer.TConst:
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(loc)
		}
		p.markSyntaxFeature(compat.ConstAndLet, p.lexer.Range())
		p.lexer.Next()

		if p.options.ts.Parse && p.lexer.Token == js_lexer.TEnum {
			return p.parseTypeScriptEnumStmt(loc, opts)
		}

		decls := p.parseAndDeclareDecls(ast.SymbolConst, opts)
		p.lexer.ExpectOrInsertSemicolon()
		if !opts.isTypeScriptDeclare {
			p.requireInitializers(js_ast.LocalConst, decls)
		}
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
			Kind:     js_ast.LocalConst,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case js_lexer.TIf:
		p.lexer.Next()
		p.lexer.Expect(js_lexer.TOpenParen)
		test := p.parseExpr(js_ast.LLowest)
		p.lexer.Expect(js_lexer.TCloseParen)
		isSingleLineYes := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
		yes := p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowFnInsideIf})
		var noOrNil js_ast.Stmt
		var isSingleLineNo bool
		if p.lexer.Token == js_lexer.TElse {
			p.lexer.Next()
			isSingleLineNo = !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
			noOrNil = p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowFnInsideIf})
		}
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SIf{Test: test, Yes: yes, NoOrNil: noOrNil, IsSingleLineYes: isSingleLineYes, IsSingleLineNo: isSingleLineNo}}

	case js_lexer.TDo:
		p.lexer.Next()
		body := p.parseStmt(parseStmtOpts{})
		p.lexer.Expect(js_lexer.TWhile)
		p.lexer.Expect(js_lexer.TOpenParen)
		test := p.parseExpr(js_ast.LLowest)
		p.lexer.Expect(js_lexer.TCloseParen)

		// This is a weird corner case where automatic semicolon insertion applies
		// even without a newline present
		if p.lexer.Token == js_lexer.TSemicolon {
			p.lexer.Next()
		}
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SDoWhile{Body: body, Test: test}}

	case js_lexer.TWhile:
		p.lexer.Next()
		p.lexer.Expect(js_lexer.TOpenParen)
		test := p.parseExpr(js_ast.LLowest)
		p.lexer.Expect(js_lexer.TCloseParen)
		isSingleLineBody := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
		body := p.parseStmt(parseStmtOpts{})
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SWhile{Test: test, Body: body, IsSingleLineBody: isSingleLineBody}}

	case js_lexer.TWith:
		p.lexer.Next()
		p.lexer.Expect(js_lexer.TOpenParen)
		test := p.parseExpr(js_ast.LLowest)
		bodyLoc := p.lexer.Loc()
		p.lexer.Expect(js_lexer.TCloseParen)

		// Push a scope so we make sure to prevent any bare identifiers referenced
		// within the body from being renamed. Renaming them might change the
		// semantics of the code.
		p.pushScopeForParsePass(js_ast.ScopeWith, bodyLoc)
		isSingleLineBody := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
		body := p.parseStmt(parseStmtOpts{})
		p.popScope()

		return js_ast.Stmt{Loc: loc, Data: &js_ast.SWith{Value: test, BodyLoc: bodyLoc, Body: body, IsSingleLineBody: isSingleLineBody}}

	case js_lexer.TSwitch:
		p.lexer.Next()
		p.lexer.Expect(js_lexer.TOpenParen)
		test := p.parseExpr(js_ast.LLowest)
		p.lexer.Expect(js_lexer.TCloseParen)

		bodyLoc := p.lexer.Loc()
		p.pushScopeForParsePass(js_ast.ScopeBlock, bodyLoc)
		defer p.popScope()

		p.lexer.Expect(js_lexer.TOpenBrace)
		cases := []js_ast.Case{}
		foundDefault := false
		switchScopeStart := len(p.scopesInOrder)
		var caseScopeMap map[*js_ast.Scope]struct{}

		for p.lexer.Token != js_lexer.TCloseBrace {
			var value js_ast.Expr
			body := []js_ast.Stmt{}
			caseLoc := p.saveExprCommentsHere()
			caseScopeStart := len(p.scopesInOrder)

			if p.lexer.Token == js_lexer.TDefault {
				if foundDefault {
					p.log.AddError(&p.tracker, p.lexer.Range(), "Multiple default clauses are not allowed")
					panic(js_lexer.LexerPanic{})
				}
				foundDefault = true
				p.lexer.Next()
				p.lexer.Expect(js_lexer.TColon)
			} else {
				p.lexer.Expect(js_lexer.TCase)
				value = p.parseExpr(js_ast.LLowest)
				p.lexer.Expect(js_lexer.TColon)
			}

			// Keep track of any scopes created by case values. This can happen if
			// code uses anonymous functions inside a case value. For example:
			//
			//   switch (x) {
			//     case y.map(z => -z).join(':'):
			//       return y
			//   }
			//
			if caseScopeStart < len(p.scopesInOrder) {
				if caseScopeMap == nil {
					caseScopeMap = make(map[*js_ast.Scope]struct{})
				}
				for {
					caseScopeMap[p.scopesInOrder[caseScopeStart].scope] = struct{}{}
					caseScopeStart++
					if caseScopeStart == len(p.scopesInOrder) {
						break
					}
				}
			}

		caseBody:
			for {
				switch p.lexer.Token {
				case js_lexer.TCloseBrace, js_lexer.TCase, js_lexer.TDefault:
					break caseBody

				default:
					body = append(body, p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowAll}))
				}
			}

			cases = append(cases, js_ast.Case{ValueOrNil: value, Body: body, Loc: caseLoc})
		}

		// If any case contains values that create a scope, reorder those scopes to
		// come first before any scopes created by case bodies. This reflects the
		// order in which we will visit the AST in our second parsing pass. The
		// second parsing pass visits things in a different order because it uses
		// case values to determine liveness, and then uses the liveness information
		// when visiting the case bodies (e.g. avoid "require()" calls in dead code).
		// For example:
		//
		//   switch (1) {
		//     case y(() => 1):
		//       z = () => 2;
		//       break;
		//
		//     case y(() => 3):
		//       z = () => 4;
		//       break;
		//   }
		//
		// This is parsed in the order 1,2,3,4 but visited in the order 1,3,2,4.
		if len(caseScopeMap) > 0 {
			caseScopes := make([]scopeOrder, 0, len(caseScopeMap))
			bodyScopes := make([]scopeOrder, 0, len(p.scopesInOrder)-switchScopeStart-len(caseScopeMap))
			for i := switchScopeStart; i < len(p.scopesInOrder); i++ {
				it := p.scopesInOrder[i]
				if _, ok := caseScopeMap[it.scope]; ok {
					caseScopes = append(caseScopes, it)
				} else {
					bodyScopes = append(bodyScopes, it)
				}
			}
			copy(p.scopesInOrder[switchScopeStart:switchScopeStart+len(caseScopeMap)], caseScopes)
			copy(p.scopesInOrder[switchScopeStart+len(caseScopeMap):], bodyScopes)
		}

		closeBraceLoc := p.lexer.Loc()
		p.lexer.Expect(js_lexer.TCloseBrace)
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SSwitch{
			Test:          test,
			Cases:         cases,
			BodyLoc:       bodyLoc,
			CloseBraceLoc: closeBraceLoc,
		}}

	case js_lexer.TTry:
		p.lexer.Next()
		blockLoc := p.lexer.Loc()
		p.lexer.Expect(js_lexer.TOpenBrace)
		p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
		body := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
		p.popScope()
		closeBraceLoc := p.lexer.Loc()
		p.lexer.Next()

		var catch *js_ast.Catch = nil
		var finally *js_ast.Finally = nil

		if p.lexer.Token == js_lexer.TCatch {
			catchLoc := p.lexer.Loc()
			p.pushScopeForParsePass(js_ast.ScopeCatchBinding, catchLoc)
			p.lexer.Next()
			var bindingOrNil js_ast.Binding

			// The catch binding is optional, and can be omitted
			if p.lexer.Token == js_lexer.TOpenBrace {
				if p.options.unsupportedJSFeatures.Has(compat.OptionalCatchBinding) {
					// Generate a new symbol for the catch binding for older browsers
					ref := p.newSymbol(ast.SymbolOther, "e")
					p.currentScope.Generated = append(p.currentScope.Generated, ref)
					bindingOrNil = js_ast.Binding{Loc: p.lexer.Loc(), Data: &js_ast.BIdentifier{Ref: ref}}
				}
			} else {
				p.lexer.Expect(js_lexer.TOpenParen)
				bindingOrNil = p.parseBinding(parseBindingOpts{})

				// Skip over types
				if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon {
					p.lexer.Expect(js_lexer.TColon)
					p.skipTypeScriptType(js_ast.LLowest)
				}

				p.lexer.Expect(js_lexer.TCloseParen)

				// Bare identifiers are a special case
				kind := ast.SymbolOther
				if _, ok := bindingOrNil.Data.(*js_ast.BIdentifier); ok {
					kind = ast.SymbolCatchIdentifier
				}
				p.declareBinding(kind, bindingOrNil, parseStmtOpts{})
			}

			blockLoc := p.lexer.Loc()
			p.lexer.Expect(js_lexer.TOpenBrace)

			p.pushScopeForParsePass(js_ast.ScopeBlock, blockLoc)
			stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
			p.popScope()

			closeBraceLoc := p.lexer.Loc()
			p.lexer.Next()
			catch = &js_ast.Catch{Loc: catchLoc, BindingOrNil: bindingOrNil, BlockLoc: blockLoc, Block: js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc}}
			p.popScope()
		}

		if p.lexer.Token == js_lexer.TFinally || catch == nil {
			finallyLoc := p.lexer.Loc()
			p.pushScopeForParsePass(js_ast.ScopeBlock, finallyLoc)
			p.lexer.Expect(js_lexer.TFinally)
			p.lexer.Expect(js_lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
			closeBraceLoc := p.lexer.Loc()
			p.lexer.Next()
			finally = &js_ast.Finally{Loc: finallyLoc, Block: js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc}}
			p.popScope()
		}

		return js_ast.Stmt{Loc: loc, Data: &js_ast.STry{
			BlockLoc: blockLoc,
			Block:    js_ast.SBlock{Stmts: body, CloseBraceLoc: closeBraceLoc},
			Catch:    catch,
			Finally:  finally,
		}}

	case js_lexer.TFor:
		p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()

		// "for await (let x of y) {}"
		var awaitRange logger.Range
		if p.lexer.IsContextualKeyword("await") {
			awaitRange = p.lexer.Range()
			if p.fnOrArrowDataParse.await != allowExpr {
				p.log.AddError(&p.tracker, awaitRange, "Cannot use \"await\" outside an async function")
				awaitRange = logger.Range{}
			} else {
				didGenerateError := false
				if p.fnOrArrowDataParse.isTopLevel {
					p.topLevelAwaitKeyword = awaitRange
				}
				if !didGenerateError && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) && p.options.unsupportedJSFeatures.Has(compat.Generator) {
					// If for-await loops aren't supported, then we only support lowering
					// if either async/await or generators is supported. Otherwise we
					// cannot lower for-await loops.
					p.markSyntaxFeature(compat.ForAwait, awaitRange)
				}
			}
			p.lexer.Next()
		}

		p.lexer.Expect(js_lexer.TOpenParen)

		var initOrNil js_ast.Stmt
		var testOrNil js_ast.Expr
		var updateOrNil js_ast.Expr

		// "in" expressions aren't allowed here
		p.allowIn = false

		var badLetRange logger.Range
		if p.lexer.IsContextualKeyword("let") {
			badLetRange = p.lexer.Range()
		}
		decls := []js_ast.Decl{}
		initLoc := p.lexer.Loc()
		isVar := false
		switch p.lexer.Token {
		case js_lexer.TVar:
			isVar = true
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolHoisted, parseStmtOpts{})
			initOrNil = js_ast.Stmt{Loc: initLoc, Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: decls}}

		case js_lexer.TConst:
			p.markSyntaxFeature(compat.ConstAndLet, p.lexer.Range())
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolConst, parseStmtOpts{})
			initOrNil = js_ast.Stmt{Loc: initLoc, Data: &js_ast.SLocal{Kind: js_ast.LocalConst, Decls: decls}}

		case js_lexer.TSemicolon:

		default:
			var expr js_ast.Expr
			var stmt js_ast.Stmt
			expr, stmt, decls = p.parseExprOrLetOrUsingStmt(parseStmtOpts{
				lexicalDecl:        lexicalDeclAllowAll,
				isForLoopInit:      true,
				isForAwaitLoopInit: awaitRange.Len > 0,
			})
			if stmt.Data != nil {
				badLetRange = logger.Range{}
				initOrNil = stmt
			} else {
				initOrNil = js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}}
			}
		}

		// "in" expressions are allowed again
		p.allowIn = true

		// Detect for-of loops
		if p.lexer.IsContextualKeyword("of") || awaitRange.Len > 0 {
			if badLetRange.Len > 0 {
				p.log.AddError(&p.tracker, badLetRange, "\"let\" must be wrapped in parentheses to be used as an expression here:")
			}
			if awaitRange.Len > 0 && !p.lexer.IsContextualKeyword("of") {
				if initOrNil.Data != nil {
					p.lexer.ExpectedString("\"of\"")
				} else {
					p.lexer.Unexpected()
				}
			}
			p.forbidInitializers(decls, "of", false)
			p.markSyntaxFeature(compat.ForOf, p.lexer.Range())
			p.lexer.Next()
			value := p.parseExpr(js_ast.LComma)
			p.lexer.Expect(js_lexer.TCloseParen)
			isSingleLineBody := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
			body := p.parseStmt(parseStmtOpts{})
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SForOf{Await: awaitRange, Init: initOrNil, Value: value, Body: body, IsSingleLineBody: isSingleLineBody}}
		}

		// Detect for-in loops
		if p.lexer.Token == js_lexer.TIn {
			p.forbidInitializers(decls, "in", isVar)
			if len(decls) == 1 {
				if local, ok := initOrNil.Data.(*js_ast.SLocal); ok {
					if local.Kind == js_ast.LocalUsing {
						p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, initOrNil.Loc), "\"using\" declarations are not allowed here")
					} else if local.Kind == js_ast.LocalAwaitUsing {
						p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, initOrNil.Loc), "\"await using\" declarations are not allowed here")
					}
				}
			}
			p.lexer.Next()
			value := p.parseExpr(js_ast.LLowest)
			p.lexer.Expect(js_lexer.TCloseParen)
			isSingleLineBody := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
			body := p.parseStmt(parseStmtOpts{})
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SForIn{Init: initOrNil, Value: value, Body: body, IsSingleLineBody: isSingleLineBody}}
		}

		p.lexer.Expect(js_lexer.TSemicolon)

		// "await using" declarations are only allowed in for-of loops
		if local, ok := initOrNil.Data.(*js_ast.SLocal); ok && local.Kind == js_ast.LocalAwaitUsing {
			p.log.AddError(&p.tracker, js_lexer.RangeOfIdentifier(p.source, initOrNil.Loc), "\"await using\" declarations are not allowed here")
		}

		// Only require "const" statement initializers when we know we're a normal for loop
		if local, ok := initOrNil.Data.(*js_ast.SLocal); ok && (local.Kind == js_ast.LocalConst || local.Kind == js_ast.LocalUsing) {
			p.requireInitializers(local.Kind, decls)
		}

		if p.lexer.Token != js_lexer.TSemicolon {
			testOrNil = p.parseExpr(js_ast.LLowest)
		}

		p.lexer.Expect(js_lexer.TSemicolon)

		if p.lexer.Token != js_lexer.TCloseParen {
			updateOrNil = p.parseExpr(js_ast.LLowest)
		}

		p.lexer.Expect(js_lexer.TCloseParen)
		isSingleLineBody := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
		body := p.parseStmt(parseStmtOpts{})
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SFor{
			InitOrNil:        initOrNil,
			TestOrNil:        testOrNil,
			UpdateOrNil:      updateOrNil,
			Body:             body,
			IsSingleLineBody: isSingleLineBody,
		}}

	case js_lexer.TImport:
		previousImportStatementKeyword := p.esmImportStatementKeyword
		p.esmImportStatementKeyword = p.lexer.Range()
		p.lexer.Next()
		stmt := js_ast.SImport{}
		wasOriginallyBareImport := false

		// "export import foo = bar"
		// "import foo = bar" in a namespace
		if (opts.isExport || (opts.isNamespaceScope && !opts.isTypeScriptDeclare)) && p.lexer.Token != js_lexer.TIdentifier {
			p.lexer.Expected(js_lexer.TIdentifier)
		}

	syntaxBeforePath:
		switch p.lexer.Token {
		case js_lexer.TOpenParen, js_lexer.TDot:
			// "import('path')"
			// "import.meta"
			p.esmImportStatementKeyword = previousImportStatementKeyword // This wasn't an ESM import statement after all
			expr := p.parseSuffix(p.parseImportExpr(loc, js_ast.LLowest), js_ast.LLowest, nil, 0)
			p.lexer.ExpectOrInsertSemicolon()
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}}

		case js_lexer.TStringLiteral, js_lexer.TNoSubstitutionTemplateLiteral:
			// "import 'path'"
			if !opts.isModuleScope && (!opts.isNamespaceScope || !opts.isTypeScriptDeclare) {
				p.lexer.Unexpected()
				return js_ast.Stmt{}
			}

			wasOriginallyBareImport = true

		case js_lexer.TAsterisk:
			// "import * as ns from 'path'"
			if !opts.isModuleScope && (!opts.isNamespaceScope || !opts.isTypeScriptDeclare) {
				p.lexer.Unexpected()
				return js_ast.Stmt{}
			}

			p.lexer.Next()
			p.lexer.ExpectContextualKeyword("as")
			stmt.NamespaceRef = p.storeNameInRef(p.lexer.Identifier)
			starLoc := p.lexer.Loc()
			stmt.StarNameLoc = &starLoc
			p.lexer.Expect(js_lexer.TIdentifier)
			p.lexer.ExpectContextualKeyword("from")

		case js_lexer.TOpenBrace:
			// "import {item1, item2} from 'path'"
			if !opts.isModuleScope && (!opts.isNamespaceScope || !opts.isTypeScriptDeclare) {
				p.lexer.Unexpected()
				return js_ast.Stmt{}
			}

			items, isSingleLine := p.parseImportClause()
			stmt.Items = &items
			stmt.IsSingleLine = isSingleLine
			p.lexer.ExpectContextualKeyword("from")

		case js_lexer.TIdentifier:
			// "import defaultItem from 'path'"
			// "import foo = bar"
			if !opts.isModuleScope && !opts.isNamespaceScope {
				p.lexer.Unexpected()
				return js_ast.Stmt{}
			}

			defaultName := p.lexer.Identifier
			stmt.DefaultName = &ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(defaultName)}
			p.lexer.Next()

			if p.options.ts.Parse {
				// Skip over type-only imports
				if defaultName.String == "type" {
					switch p.lexer.Token {
					case js_lexer.TIdentifier:
						nameSubstring := p.lexer.Identifier
						nameLoc := p.lexer.Loc()
						p.lexer.Next()
						if p.lexer.Token == js_lexer.TEquals {
							// "import type foo = require('bar');"
							// "import type foo = bar.baz;"
							opts.isTypeScriptDeclare = true
							return p.parseTypeScriptImportEqualsStmt(loc, opts, nameLoc, nameSubstring.String)
						} else if p.lexer.Token == js_lexer.TStringLiteral && nameSubstring.String == "from" {
							// "import type from 'bar';"
							break syntaxBeforePath
						} else {
							// "import type foo from 'bar';"
							p.lexer.ExpectContextualKeyword("from")
							p.parsePath()
							p.lexer.ExpectOrInsertSemicolon()
							return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
						}

					case js_lexer.TAsterisk:
						// "import type * as foo from 'bar';"
						p.lexer.Next()
						p.lexer.ExpectContextualKeyword("as")
						p.lexer.Expect(js_lexer.TIdentifier)
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}

					case js_lexer.TOpenBrace:
						// "import type {foo} from 'bar';"
						p.parseImportClause()
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
					}
				}

				// Parse TypeScript import assignment statements
				if p.lexer.Token == js_lexer.TEquals || opts.isExport || (opts.isNamespaceScope && !opts.isTypeScriptDeclare) {
					p.esmImportStatementKeyword = previousImportStatementKeyword // This wasn't an ESM import statement after all
					return p.parseTypeScriptImportEqualsStmt(loc, opts, stmt.DefaultName.Loc, defaultName.String)
				}
			}

			if p.lexer.Token == js_lexer.TComma {
				p.lexer.Next()
				switch p.lexer.Token {
				case js_lexer.TAsterisk:
					// "import defaultItem, * as ns from 'path'"
					p.lexer.Next()
					p.lexer.ExpectContextualKeyword("as")
					stmt.NamespaceRef = p.storeNameInRef(p.lexer.Identifier)
					starLoc := p.lexer.Loc()
					stmt.StarNameLoc = &starLoc
					p.lexer.Expect(js_lexer.TIdentifier)

				case js_lexer.TOpenBrace:
					// "import defaultItem, {item1, item2} from 'path'"
					items, isSingleLine := p.parseImportClause()
					stmt.Items = &items
					stmt.IsSingleLine = isSingleLine

				default:
					p.lexer.Unexpected()
				}
			}

			p.lexer.ExpectContextualKeyword("from")

		default:
			p.lexer.Unexpected()
			return js_ast.Stmt{}
		}

		pathLoc, pathText, assertOrWith, flags := p.parsePath()
		p.lexer.ExpectOrInsertSemicolon()

		// If TypeScript's "preserveValueImports": true setting is active, TypeScript's
		// "importsNotUsedAsValues": "preserve" setting is NOT active, and the import
		// clause is present and empty (or is non-empty but filled with type-only
		// items), then the import statement should still be removed entirely to match
		// the behavior of the TypeScript compiler:
		//
		//   // Keep these
		//   import 'x'
		//   import { y } from 'x'
		//   import { y, type z } from 'x'
		//
		//   // Remove these
		//   import {} from 'x'
		//   import { type y } from 'x'
		//
		//   // Remove the items from these
		//   import d, {} from 'x'
		//   import d, { type y } from 'x'
		//
		if p.options.ts.Parse && p.options.ts.Config.UnusedImportFlags() == config.TSUnusedImport_KeepValues && stmt.Items != nil && len(*stmt.Items) == 0 {
			if stmt.DefaultName == nil {
				return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
			}
			stmt.Items = nil
		}

		if wasOriginallyBareImport {
			flags |= ast.WasOriginallyBareImport
		}
		stmt.ImportRecordIndex = p.addImportRecord(ast.ImportStmt, pathLoc, pathText, assertOrWith, flags)

		if stmt.StarNameLoc != nil {
			name := p.loadNameFromRef(stmt.NamespaceRef)
			stmt.NamespaceRef = p.declareSymbol(ast.SymbolImport, *stmt.StarNameLoc, name)
		} else {
			// Generate a symbol for the namespace
			name := "import_" + js_ast.GenerateNonUniqueNameFromPath(pathText)
			stmt.NamespaceRef = p.newSymbol(ast.SymbolOther, name)
			p.currentScope.Generated = append(p.currentScope.Generated, stmt.NamespaceRef)
		}
		itemRefs := make(map[string]ast.LocRef)

		// Link the default item to the namespace
		if stmt.DefaultName != nil {
			name := p.loadNameFromRef(stmt.DefaultName.Ref)
			ref := p.declareSymbol(ast.SymbolImport, stmt.DefaultName.Loc, name)
			p.isImportItem[ref] = true
			stmt.DefaultName.Ref = ref
		}

		// Link each import item to the namespace
		if stmt.Items != nil {
			for i, item := range *stmt.Items {
				name := p.loadNameFromRef(item.Name.Ref)
				ref := p.declareSymbol(ast.SymbolImport, item.Name.Loc, name)
				p.checkForUnrepresentableIdentifier(item.AliasLoc, item.Alias)
				p.isImportItem[ref] = true
				(*stmt.Items)[i].Name.Ref = ref
				itemRefs[item.Alias] = ast.LocRef{Loc: item.Name.Loc, Ref: ref}
			}
		}

		// Track the items for this namespace
		p.importItemsForNamespace[stmt.NamespaceRef] = namespaceImportItems{
			entries:           itemRefs,
			importRecordIndex: stmt.ImportRecordIndex,
		}

		// Import statements anywhere in the file disable top-level const
		// local prefix because import cycles can be used to trigger TDZ
		p.currentScope.IsAfterConstLocalPrefix = true
		return js_ast.Stmt{Loc: loc, Data: &stmt}

	case js_lexer.TBreak:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SBreak{Label: name}}

	case js_lexer.TContinue:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SContinue{Label: name}}

	case js_lexer.TReturn:
		if p.fnOrArrowDataParse.isReturnDisallowed {
			p.log.AddError(&p.tracker, p.lexer.Range(), "A return statement cannot be used here:")
		}
		p.lexer.Next()
		var value js_ast.Expr
		if p.lexer.Token != js_lexer.TSemicolon &&
			!p.lexer.HasNewlineBefore &&
			p.lexer.Token != js_lexer.TCloseBrace &&
			p.lexer.Token != js_lexer.TEndOfFile {
			value = p.parseExpr(js_ast.LLowest)
		}
		p.latestReturnHadSemicolon = p.lexer.Token == js_lexer.TSemicolon
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SReturn{ValueOrNil: value}}

	case js_lexer.TThrow:
		p.lexer.Next()
		if p.lexer.HasNewlineBefore {
			endLoc := logger.Loc{Start: loc.Start + 5}
			p.log.AddError(&p.tracker, logger.Range{Loc: endLoc},
				"Unexpected newline after \"throw\"")
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SThrow{Value: js_ast.Expr{Loc: endLoc, Data: js_ast.ENullShared}}}
		}
		expr := p.parseExpr(js_ast.LLowest)
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SThrow{Value: expr}}

	case js_lexer.TDebugger:
		p.lexer.Next()
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: js_ast.SDebuggerShared}

	case js_lexer.TOpenBrace:
		p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()
		stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
		closeBraceLoc := p.lexer.Loc()
		p.lexer.Next()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc}}

	default:
		isIdentifier := p.lexer.Token == js_lexer.TIdentifier
		nameRange := p.lexer.Range()
		name := p.lexer.Identifier.String

		// Parse either an async function, an async expression, or a normal expression
		var expr js_ast.Expr
		if isIdentifier && p.lexer.Raw() == "async" {
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TFunction && !p.lexer.HasNewlineBefore {
				p.lexer.Next()
				return p.parseFnStmt(nameRange.Loc, opts, true /* isAsync */, nameRange)
			}
			expr = p.parseSuffix(p.parseAsyncPrefixExpr(nameRange, js_ast.LLowest, 0), js_ast.LLowest, nil, 0)
		} else {
			var stmt js_ast.Stmt
			expr, stmt, _ = p.parseExprOrLetOrUsingStmt(opts)
			if stmt.Data != nil {
				p.lexer.ExpectOrInsertSemicolon()
				return stmt
			}
		}

		if isIdentifier {
			if ident, ok := expr.Data.(*js_ast.EIdentifier); ok {
				if p.lexer.Token == js_lexer.TColon && opts.deferredDecorators == nil {
					p.pushScopeForParsePass(js_ast.ScopeLabel, loc)
					defer p.popScope()

					// Parse a labeled statement
					p.lexer.Next()
					name := ast.LocRef{Loc: expr.Loc, Ref: ident.Ref}
					nestedOpts := parseStmtOpts{}
					if opts.lexicalDecl == lexicalDeclAllowAll || opts.lexicalDecl == lexicalDeclAllowFnInsideLabel {
						nestedOpts.lexicalDecl = lexicalDeclAllowFnInsideLabel
					}
					isSingleLineStmt := !p.lexer.HasNewlineBefore && p.lexer.Token != js_lexer.TOpenBrace
					stmt := p.parseStmt(nestedOpts)
					return js_ast.Stmt{Loc: loc, Data: &js_ast.SLabel{Name: name, Stmt: stmt, IsSingleLineStmt: isSingleLineStmt}}
				}

				if p.options.ts.Parse {
					switch name {
					case "type":
						if !p.lexer.HasNewlineBefore && p.lexer.Token == js_lexer.TIdentifier {
							// "type Foo = any"
							p.skipTypeScriptTypeStmt(parseStmtOpts{isModuleScope: opts.isModuleScope})
							return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
						}

					case "namespace", "module":
						// "namespace Foo {}"
						// "module Foo {}"
						// "declare module 'fs' {}"
						// "declare module 'fs';"
						if !p.lexer.HasNewlineBefore && (opts.isModuleScope || opts.isNamespaceScope) && (p.lexer.Token == js_lexer.TIdentifier ||
							(p.lexer.Token == js_lexer.TStringLiteral && opts.isTypeScriptDeclare)) {
							return p.parseTypeScriptNamespaceStmt(loc, opts)
						}

					case "interface":
						// "interface Foo {}"
						// "export default interface Foo {}"
						// "export default interface \n Foo {}"
						if !p.lexer.HasNewlineBefore || opts.isExportDefault {
							p.skipTypeScriptInterfaceStmt(parseStmtOpts{isModuleScope: opts.isModuleScope})
							return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
						}

						// "interface \n Foo {}"
						// "export interface \n Foo {}"
						if opts.isExport {
							p.log.AddError(&p.tracker, nameRange, "Unexpected \"interface\"")
							panic(js_lexer.LexerPanic{})
						}

					case "abstract":
						if !p.lexer.HasNewlineBefore && p.lexer.Token == js_lexer.TClass {
							return p.parseClassStmt(loc, opts)
						}

					case "global":
						// "declare module 'fs' { global { namespace NodeJS {} } }"
						if opts.isNamespaceScope && opts.isTypeScriptDeclare && p.lexer.Token == js_lexer.TOpenBrace {
							p.lexer.Next()
							p.parseStmtsUpTo(js_lexer.TCloseBrace, opts)
							p.lexer.Next()
							return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
						}

					case "declare":
						if !p.lexer.HasNewlineBefore {
							opts.lexicalDecl = lexicalDeclAllowAll
							opts.isTypeScriptDeclare = true

							// "declare global { ... }"
							if p.lexer.IsContextualKeyword("global") {
								p.lexer.Next()
								p.lexer.Expect(js_lexer.TOpenBrace)
								p.parseStmtsUpTo(js_lexer.TCloseBrace, opts)
								p.lexer.Next()
								return js_ast.Stmt{Loc: loc, Data: js_ast.STypeScriptShared}
							}

							// "declare const x: any"
							scopeIndex := len(p.scopesInOrder)
							oldLexer := p.lexer
							stmt := p.parseStmt(opts)
							typeDeclarationData := js_ast.STypeScriptShared
							switch s := stmt.Data.(type) {
							case *js_ast.SEmpty:
								return js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}}

							case *js_ast.STypeScript:
								// Type declarations are expected. Propagate the "declare class"
								// status in case our caller is a decorator that needs to know
								// this was a "declare class" statement.
								typeDeclarationData = s

							case *js_ast.SLocal:
								// This is also a type declaration (but doesn't use "STypeScript"
								// because we need to be able to handle namespace exports below)

							default:
								// Anything that we don't expect is a syntax error. For example,
								// we consider this a syntax error:
								//
								//   declare let declare: any, foo: any
								//   declare foo
								//
								// Strangely TypeScript allows this code starting with version
								// 4.4, but I assume this is a bug. This bug was reported here:
								// https://github.com/microsoft/TypeScript/issues/54602
								p.lexer = oldLexer
								p.lexer.Unexpected()
							}
							p.discardScopesUpTo(scopeIndex)

							// Unlike almost all uses of "declare", statements that use
							// "export declare" with "var/let/const" inside a namespace affect
							// code generation. They cause any declared bindings to be
							// considered exports of the namespace. Identifier references to
							// those names must be converted into property accesses off the
							// namespace object:
							//
							//   namespace ns {
							//     export declare const x
							//     export function y() { return x }
							//   }
							//
							//   (ns as any).x = 1
							//   console.log(ns.y())
							//
							// In this example, "return x" must be replaced with "return ns.x".
							// This is handled by replacing each "export declare" statement
							// inside a namespace with an "export var" statement containing all
							// of the declared bindings. That "export var" statement will later
							// cause identifiers to be transformed into property accesses.
							if opts.isNamespaceScope && opts.isExport {
								var decls []js_ast.Decl
								if s, ok := stmt.Data.(*js_ast.SLocal); ok {
									js_ast.ForEachIdentifierBindingInDecls(s.Decls, func(loc logger.Loc, b *js_ast.BIdentifier) {
										decls = append(decls, js_ast.Decl{Binding: js_ast.Binding{Loc: loc, Data: b}})
									})
								}
								if len(decls) > 0 {
									return js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
										Kind:     js_ast.LocalVar,
										IsExport: true,
										Decls:    decls,
									}}
								}
							}

							return js_ast.Stmt{Loc: loc, Data: typeDeclarationData}
						}
					}
				}
			}
		}

		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}}
	}
}

func (p *parser) addImportRecord(kind ast.ImportKind, pathRange logger.Range, text string, assertOrWith *ast.ImportAssertOrWith, flags ast.ImportRecordFlags) uint32 {
	index := uint32(len(p.importRecords))
	p.importRecords = append(p.importRecords, ast.ImportRecord{
		Kind:         kind,
		Range:        pathRange,
		Path:         logger.Path{Text: text},
		AssertOrWith: assertOrWith,
		Flags:        flags,
	})
	return index
}

func (p *parser) parseFnBody(data fnOrArrowDataParse) js_ast.FnBody {
	oldFnOrArrowData := p.fnOrArrowDataParse
	oldAllowIn := p.allowIn
	p.fnOrArrowDataParse = data
	p.allowIn = true

	loc := p.lexer.Loc()
	p.pushScopeForParsePass(js_ast.ScopeFunctionBody, loc)
	defer p.popScope()

	p.lexer.Expect(js_lexer.TOpenBrace)
	stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{
		allowDirectivePrologue: true,
	})
	closeBraceLoc := p.lexer.Loc()
	p.lexer.Next()

	p.allowIn = oldAllowIn
	p.fnOrArrowDataParse = oldFnOrArrowData
	return js_ast.FnBody{Loc: loc, Block: js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc}}
}

func (p *parser) forbidLexicalDecl(loc logger.Loc) {
	r := js_lexer.RangeOfIdentifier(p.source, loc)
	p.log.AddError(&p.tracker, r, "Cannot use a declaration in a single-statement context")
}

func (p *parser) parseStmtsUpTo(end js_lexer.T, opts parseStmtOpts) []js_ast.Stmt {
	stmts := []js_ast.Stmt{}
	returnWithoutSemicolonStart := int32(-1)
	opts.lexicalDecl = lexicalDeclAllowAll
	isDirectivePrologue := opts.allowDirectivePrologue

	for {
		// Preserve some statement-level comments
		comments := p.lexer.LegalCommentsBeforeToken
		if len(comments) > 0 {
			for _, comment := range comments {
				stmts = append(stmts, js_ast.Stmt{
					Loc: comment.Loc,
					Data: &js_ast.SComment{
						Text:           p.source.CommentTextWithoutIndent(comment),
						IsLegalComment: true,
					},
				})
			}
		}

		if p.lexer.Token == end {
			break
		}

		stmt := p.parseStmt(opts)

		// Skip TypeScript types entirely
		if p.options.ts.Parse {
			if _, ok := stmt.Data.(*js_ast.STypeScript); ok {
				continue
			}
		}

		// Parse one or more directives at the beginning
		if isDirectivePrologue {
			isDirectivePrologue = false
			if expr, ok := stmt.Data.(*js_ast.SExpr); ok {
				if str, ok := expr.Value.Data.(*js_ast.EString); ok && !str.PreferTemplate {
					stmt.Data = &js_ast.SDirective{Value: str.Value, LegacyOctalLoc: str.LegacyOctalLoc}
					isDirectivePrologue = true

					if helpers.UTF16EqualsString(str.Value, "use strict") {
						// Track "use strict" directives
						p.currentScope.StrictMode = js_ast.ExplicitStrictMode
						p.currentScope.UseStrictLoc = expr.Value.Loc

						// Inside a function, strict mode actually propagates from the child
						// scope to the parent scope:
						//
						//   // This is a syntax error
						//   function fn(arguments) {
						//     "use strict";
						//   }
						//
						if p.currentScope.Kind == js_ast.ScopeFunctionBody &&
							p.currentScope.Parent.Kind == js_ast.ScopeFunctionArgs &&
							p.currentScope.Parent.StrictMode == js_ast.SloppyMode {
							p.currentScope.Parent.StrictMode = js_ast.ExplicitStrictMode
							p.currentScope.Parent.UseStrictLoc = expr.Value.Loc
						}
					} else if helpers.UTF16EqualsString(str.Value, "use asm") {
						// Deliberately remove "use asm" directives. The asm.js subset of
						// JavaScript has complicated validation rules that are triggered
						// by this directive. This parser is not designed with asm.js in
						// mind and round-tripping asm.js code through esbuild will very
						// likely cause it to no longer validate as asm.js. When this
						// happens, V8 prints a warning and people don't like seeing the
						// warning.
						//
						// We deliberately do not attempt to preserve the validity of
						// asm.js code because it's a complicated legacy format and it's
						// obsolete now that WebAssembly exists. By removing this directive
						// it will just become normal JavaScript, which will work fine and
						// won't generate a warning (but will run slower). We don't generate
						// a warning ourselves in this case because there isn't necessarily
						// anything easy and actionable that the user can do to fix this.
						stmt.Data = &js_ast.SEmpty{}
					}
				}
			}
		}

		stmts = append(stmts, stmt)

		// Warn about ASI and return statements. Here's an example of code with
		// this problem: https://github.com/rollup/rollup/issues/3729
		if !p.suppressWarningsAboutWeirdCode {
			if s, ok := stmt.Data.(*js_ast.SReturn); ok && s.ValueOrNil.Data == nil && !p.latestReturnHadSemicolon {
				returnWithoutSemicolonStart = stmt.Loc.Start
			} else {
				if returnWithoutSemicolonStart != -1 {
					if _, ok := stmt.Data.(*js_ast.SExpr); ok {
						p.log.AddID(logger.MsgID_JS_SemicolonAfterReturn, logger.Warning, &p.tracker, logger.Range{Loc: logger.Loc{Start: returnWithoutSemicolonStart + 6}},
							"The following expression is not returned because of an automatically-inserted semicolon")
					}
				}
				returnWithoutSemicolonStart = -1
			}
		}
	}

	return stmts
}

type generateTempRefArg uint8

const (
	tempRefNeedsDeclare generateTempRefArg = iota
	tempRefNoDeclare

	// This is used when the generated temporary may a) be used inside of a loop
	// body and b) may be used inside of a closure. In that case we can't use
	// "var" for the temporary and we can't declare the temporary at the top of
	// the enclosing function. Instead, we need to use "let" and we need to
	// declare the temporary in the enclosing block (so it's inside of the loop
	// body).
	tempRefNeedsDeclareMayBeCapturedInsideLoop
)

func (p *parser) generateTempRef(declare generateTempRefArg, optionalName string) ast.Ref {
	scope := p.currentScope

	if declare != tempRefNeedsDeclareMayBeCapturedInsideLoop {
		for !scope.Kind.StopsHoisting() {
			scope = scope.Parent
		}
	}

	if optionalName == "" {
		optionalName = "_" + ast.DefaultNameMinifierJS.NumberToMinifiedName(p.tempRefCount)
		p.tempRefCount++
	}
	ref := p.newSymbol(ast.SymbolOther, optionalName)

	if declare == tempRefNeedsDeclareMayBeCapturedInsideLoop && !scope.Kind.StopsHoisting() {
		p.tempLetsToDeclare = append(p.tempLetsToDeclare, ref)
	} else if declare != tempRefNoDeclare {
		p.tempRefsToDeclare = append(p.tempRefsToDeclare, tempRef{ref: ref})
	}

	scope.Generated = append(scope.Generated, ref)
	return ref
}

func (p *parser) generateTopLevelTempRef() ast.Ref {
	ref := p.newSymbol(ast.SymbolOther, "_"+ast.DefaultNameMinifierJS.NumberToMinifiedName(p.topLevelTempRefCount))
	p.topLevelTempRefsToDeclare = append(p.topLevelTempRefsToDeclare, tempRef{ref: ref})
	p.moduleScope.Generated = append(p.moduleScope.Generated, ref)
	p.topLevelTempRefCount++
	return ref
}

func (p *parser) pushScopeForVisitPass(kind js_ast.ScopeKind, loc logger.Loc) {
	order := p.scopesInOrder[0]

	// Sanity-check that the scopes generated by the first and second passes match
	if order.loc != loc || order.scope.Kind != kind {
		panic(fmt.Sprintf("Expected scope (%d, %d) in %s, found scope (%d, %d)",
			kind, loc.Start,
			p.source.PrettyPath,
			order.scope.Kind, order.loc.Start))
	}

	p.scopesInOrder = p.scopesInOrder[1:]
	p.currentScope = order.scope
	p.scopesForCurrentPart = append(p.scopesForCurrentPart, order.scope)
}

type findSymbolResult struct {
	ref               ast.Ref
	declareLoc        logger.Loc
	isInsideWithScope bool
}

func (p *parser) findSymbol(loc logger.Loc, name string) findSymbolResult {
	var ref ast.Ref
	var declareLoc logger.Loc
	isInsideWithScope := false
	didForbidArguments := false
	s := p.currentScope

	for {
		// Track if we're inside a "with" statement body
		if s.Kind == js_ast.ScopeWith {
			isInsideWithScope = true
		}

		// Forbid referencing "arguments" inside class bodies
		if s.ForbidArguments && name == "arguments" && !didForbidArguments {
			r := js_lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddError(&p.tracker, r, fmt.Sprintf("Cannot access %q here:", name))
			didForbidArguments = true
		}

		// Is the symbol a member of this scope?
		if member, ok := s.Members[name]; ok {
			ref = member.Ref
			declareLoc = member.Loc
			break
		}

		// Is the symbol a member of this scope's TypeScript namespace?
		if tsNamespace := s.TSNamespace; tsNamespace != nil {
			if member, ok := tsNamespace.ExportedMembers[name]; ok && tsNamespace.IsEnumScope == member.IsEnumValue {
				// If this is an identifier from a sibling TypeScript namespace, then we're
				// going to have to generate a property access instead of a simple reference.
				// Lazily-generate an identifier that represents this property access.
				cache := tsNamespace.LazilyGeneratedProperyAccesses
				if cache == nil {
					cache = make(map[string]ast.Ref)
					tsNamespace.LazilyGeneratedProperyAccesses = cache
				}
				ref, ok = cache[name]
				if !ok {
					ref = p.newSymbol(ast.SymbolOther, name)
					p.symbols[ref.InnerIndex].NamespaceAlias = &ast.NamespaceAlias{
						NamespaceRef: tsNamespace.ArgRef,
						Alias:        name,
					}
					cache[name] = ref
				}
				declareLoc = member.Loc
				break
			}
		}

		s = s.Parent
		if s == nil {
			// Allocate an "unbound" symbol
			p.checkForUnrepresentableIdentifier(loc, name)
			ref = p.newSymbol(ast.SymbolUnbound, name)
			declareLoc = loc
			p.moduleScope.Members[name] = js_ast.ScopeMember{Ref: ref, Loc: logger.Loc{Start: -1}}
			break
		}
	}

	// If we had to pass through a "with" statement body to get to the symbol
	// declaration, then this reference could potentially also refer to a
	// property on the target object of the "with" statement. We must not rename
	// it or we risk changing the behavior of the code.
	if isInsideWithScope {
		p.symbols[ref.InnerIndex].Flags |= ast.MustNotBeRenamed
	}

	// Track how many times we've referenced this symbol
	p.recordUsage(ref)
	return findSymbolResult{ref, declareLoc, isInsideWithScope}
}

func (p *parser) findLabelSymbol(loc logger.Loc, name string) (ref ast.Ref, isLoop bool, ok bool) {
	for s := p.currentScope; s != nil && !s.Kind.StopsHoisting(); s = s.Parent {
		if s.Kind == js_ast.ScopeLabel && name == p.symbols[s.Label.Ref.InnerIndex].OriginalName {
			// Track how many times we've referenced this symbol
			p.recordUsage(s.Label.Ref)
			ref = s.Label.Ref
			isLoop = s.LabelStmtIsLoop
			ok = true
			return
		}
	}

	r := js_lexer.RangeOfIdentifier(p.source, loc)
	p.log.AddError(&p.tracker, r, fmt.Sprintf("There is no containing label named %q", name))

	// Allocate an "unbound" symbol
	ref = p.newSymbol(ast.SymbolUnbound, name)

	// Track how many times we've referenced this symbol
	p.recordUsage(ref)
	return
}

func findIdentifiers(binding js_ast.Binding, identifiers []js_ast.Decl) []js_ast.Decl {
	switch b := binding.Data.(type) {
	case *js_ast.BIdentifier:
		identifiers = append(identifiers, js_ast.Decl{Binding: binding})

	case *js_ast.BArray:
		for _, item := range b.Items {
			identifiers = findIdentifiers(item.Binding, identifiers)
		}

	case *js_ast.BObject:
		for _, property := range b.Properties {
			identifiers = findIdentifiers(property.Value, identifiers)
		}
	}

	return identifiers
}

// If this is in a dead branch, then we want to trim as much dead code as we
// can. Everything can be trimmed except for hoisted declarations ("var" and
// "function"), which affect the parent scope. For example:
//
//	function foo() {
//	  if (false) { var x; }
//	  x = 1;
//	}
//
// We can't trim the entire branch as dead or calling foo() will incorrectly
// assign to a global variable instead.
func shouldKeepStmtInDeadControlFlow(stmt js_ast.Stmt) bool {
	switch s := stmt.Data.(type) {
	case *js_ast.SEmpty, *js_ast.SExpr, *js_ast.SThrow, *js_ast.SReturn,
		*js_ast.SBreak, *js_ast.SContinue, *js_ast.SClass, *js_ast.SDebugger:
		// Omit these statements entirely
		return false

	case *js_ast.SLocal:
		if s.Kind != js_ast.LocalVar {
			// Omit these statements entirely
			return false
		}

		// Omit everything except the identifiers
		identifiers := []js_ast.Decl{}
		for _, decl := range s.Decls {
			identifiers = findIdentifiers(decl.Binding, identifiers)
		}
		if len(identifiers) == 0 {
			return false
		}
		s.Decls = identifiers
		return true

	case *js_ast.SBlock:
		for _, child := range s.Stmts {
			if shouldKeepStmtInDeadControlFlow(child) {
				return true
			}
		}
		return false

	case *js_ast.SIf:
		return shouldKeepStmtInDeadControlFlow(s.Yes) || (s.NoOrNil.Data != nil && shouldKeepStmtInDeadControlFlow(s.NoOrNil))

	case *js_ast.SWhile:
		return shouldKeepStmtInDeadControlFlow(s.Body)

	case *js_ast.SDoWhile:
		return shouldKeepStmtInDeadControlFlow(s.Body)

	case *js_ast.SFor:
		return (s.InitOrNil.Data != nil && shouldKeepStmtInDeadControlFlow(s.InitOrNil)) || shouldKeepStmtInDeadControlFlow(s.Body)

	case *js_ast.SForIn:
		return shouldKeepStmtInDeadControlFlow(s.Init) || shouldKeepStmtInDeadControlFlow(s.Body)

	case *js_ast.SForOf:
		return shouldKeepStmtInDeadControlFlow(s.Init) || shouldKeepStmtInDeadControlFlow(s.Body)

	case *js_ast.SLabel:
		return shouldKeepStmtInDeadControlFlow(s.Stmt)

	default:
		// Everything else must be kept
		return true
	}
}

type prependTempRefsOpts struct {
	fnBodyLoc *logger.Loc
	kind      stmtsKind
}

func (p *parser) visitStmtsAndPrependTempRefs(stmts []js_ast.Stmt, opts prependTempRefsOpts) []js_ast.Stmt {
	oldTempRefs := p.tempRefsToDeclare
	oldTempRefCount := p.tempRefCount
	p.tempRefsToDeclare = nil
	p.tempRefCount = 0

	stmts = p.visitStmts(stmts, opts.kind)

	// Prepend values for "this" and "arguments"
	if opts.fnBodyLoc != nil {
		// Capture "this"
		if ref := p.fnOnlyDataVisit.thisCaptureRef; ref != nil {
			p.tempRefsToDeclare = append(p.tempRefsToDeclare, tempRef{
				ref:        *ref,
				valueOrNil: js_ast.Expr{Loc: *opts.fnBodyLoc, Data: js_ast.EThisShared},
			})
			p.currentScope.Generated = append(p.currentScope.Generated, *ref)
		}

		// Capture "arguments"
		if ref := p.fnOnlyDataVisit.argumentsCaptureRef; ref != nil {
			p.tempRefsToDeclare = append(p.tempRefsToDeclare, tempRef{
				ref:        *ref,
				valueOrNil: js_ast.Expr{Loc: *opts.fnBodyLoc, Data: &js_ast.EIdentifier{Ref: *p.fnOnlyDataVisit.argumentsRef}},
			})
			p.currentScope.Generated = append(p.currentScope.Generated, *ref)
		}
	}

	// There may also be special top-level-only temporaries to declare
	if p.currentScope == p.moduleScope && p.topLevelTempRefsToDeclare != nil {
		p.tempRefsToDeclare = append(p.tempRefsToDeclare, p.topLevelTempRefsToDeclare...)
		p.topLevelTempRefsToDeclare = nil
	}

	// Prepend the generated temporary variables to the beginning of the statement list
	decls := []js_ast.Decl{}
	for _, temp := range p.tempRefsToDeclare {
		if p.symbols[temp.ref.InnerIndex].UseCountEstimate > 0 {
			decls = append(decls, js_ast.Decl{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: temp.ref}}, ValueOrNil: temp.valueOrNil})
			p.recordDeclaredSymbol(temp.ref)
		}
	}
	if len(decls) > 0 {
		// Skip past leading directives and comments
		split := 0
		for split < len(stmts) {
			switch stmts[split].Data.(type) {
			case *js_ast.SComment, *js_ast.SDirective:
				split++
				continue
			}
			break
		}
		stmts = append(
			append(
				append(
					[]js_ast.Stmt{},
					stmts[:split]...),
				js_ast.Stmt{Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: decls}}),
			stmts[split:]...)
	}

	p.tempRefsToDeclare = oldTempRefs
	p.tempRefCount = oldTempRefCount
	return stmts
}

type stmtsKind uint8

const (
	stmtsNormal stmtsKind = iota
	stmtsSwitch
	stmtsLoopBody
	stmtsFnBody
)

func (p *parser) visitStmts(stmts []js_ast.Stmt, kind stmtsKind) []js_ast.Stmt {
	// Save the current control-flow liveness. This represents if we are
	// currently inside an "if (false) { ... }" block.
	oldIsControlFlowDead := p.isControlFlowDead

	oldTempLetsToDeclare := p.tempLetsToDeclare
	p.tempLetsToDeclare = nil

	// Visit all statements first
	visited := make([]js_ast.Stmt, 0, len(stmts))
	var before []js_ast.Stmt
	var after []js_ast.Stmt
	var preprocessedEnums map[int][]js_ast.Stmt
	if p.scopesInOrderForEnum != nil {
		// Preprocess TypeScript enums to improve code generation. Otherwise
		// uses of an enum before that enum has been declared won't be inlined:
		//
		//   console.log(Foo.FOO) // We want "FOO" to be inlined here
		//   const enum Foo { FOO = 0 }
		//
		// The TypeScript compiler itself contains code with this pattern, so
		// it's important to implement this optimization.
		for i, stmt := range stmts {
			if _, ok := stmt.Data.(*js_ast.SEnum); ok {
				if preprocessedEnums == nil {
					preprocessedEnums = make(map[int][]js_ast.Stmt)
				}
				oldScopesInOrder := p.scopesInOrder
				p.scopesInOrder = p.scopesInOrderForEnum[stmt.Loc]
				preprocessedEnums[i] = p.visitAndAppendStmt(nil, stmt)
				p.scopesInOrder = oldScopesInOrder
			}
		}
	}
	for i, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SExportEquals:
			// TypeScript "export = value;" becomes "module.exports = value;". This
			// must happen at the end after everything is parsed because TypeScript
			// moves this statement to the end when it generates code.
			after = p.visitAndAppendStmt(after, stmt)
			continue

		case *js_ast.SFunction:
			// Manually hoist block-level function declarations to preserve semantics.
			// This is only done for function declarations that are not generators
			// or async functions, since this is a backwards-compatibility hack from
			// Annex B of the JavaScript standard.
			if !p.currentScope.Kind.StopsHoisting() && p.symbols[int(s.Fn.Name.Ref.InnerIndex)].Kind == ast.SymbolHoistedFunction {
				before = p.visitAndAppendStmt(before, stmt)
				continue
			}

		case *js_ast.SEnum:
			visited = append(visited, preprocessedEnums[i]...)
			p.scopesInOrder = p.scopesInOrder[len(p.scopesInOrderForEnum[stmt.Loc]):]
			continue
		}
		visited = p.visitAndAppendStmt(visited, stmt)
	}

	// This is used for temporary variables that could be captured in a closure,
	// and therefore need to be generated inside the nearest enclosing block in
	// case they are generated inside a loop.
	if len(p.tempLetsToDeclare) > 0 {
		decls := make([]js_ast.Decl, 0, len(p.tempLetsToDeclare))
		for _, ref := range p.tempLetsToDeclare {
			decls = append(decls, js_ast.Decl{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: ref}}})
		}
		before = append(before, js_ast.Stmt{Data: &js_ast.SLocal{Kind: js_ast.LocalLet, Decls: decls}})
	}
	p.tempLetsToDeclare = oldTempLetsToDeclare

	// Transform block-level function declarations into variable declarations
	if len(before) > 0 {
		var letDecls []js_ast.Decl
		var varDecls []js_ast.Decl
		var nonFnStmts []js_ast.Stmt
		fnStmts := make(map[ast.Ref]int)
		for _, stmt := range before {
			s, ok := stmt.Data.(*js_ast.SFunction)
			if !ok {
				// We may get non-function statements here in certain scenarios such as when "KeepNames" is enabled
				nonFnStmts = append(nonFnStmts, stmt)
				continue
			}

			// This transformation of function declarations in nested scopes is
			// intended to preserve the hoisting semantics of the original code. In
			// JavaScript, function hoisting works differently in strict mode vs.
			// sloppy mode code. We want the code we generate to use the semantics of
			// the original environment, not the generated environment. However, if
			// direct "eval" is present then it's not possible to preserve the
			// semantics because we need two identifiers to do that and direct "eval"
			// means neither identifier can be renamed to something else. So in that
			// case we give up and do not preserve the semantics of the original code.
			if p.currentScope.ContainsDirectEval {
				if hoistedRef, ok := p.hoistedRefForSloppyModeBlockFn[s.Fn.Name.Ref]; ok {
					// Merge the two identifiers back into a single one
					p.symbols[hoistedRef.InnerIndex].Link = s.Fn.Name.Ref
				}
				nonFnStmts = append(nonFnStmts, stmt)
				continue
			}

			index, ok := fnStmts[s.Fn.Name.Ref]
			if !ok {
				index = len(letDecls)
				fnStmts[s.Fn.Name.Ref] = index
				letDecls = append(letDecls, js_ast.Decl{Binding: js_ast.Binding{
					Loc: s.Fn.Name.Loc, Data: &js_ast.BIdentifier{Ref: s.Fn.Name.Ref}}})

				// Also write the function to the hoisted sibling symbol if applicable
				if hoistedRef, ok := p.hoistedRefForSloppyModeBlockFn[s.Fn.Name.Ref]; ok {
					p.recordDeclaredSymbol(hoistedRef)
					p.recordUsage(s.Fn.Name.Ref)
					varDecls = append(varDecls, js_ast.Decl{
						Binding:    js_ast.Binding{Loc: s.Fn.Name.Loc, Data: &js_ast.BIdentifier{Ref: hoistedRef}},
						ValueOrNil: js_ast.Expr{Loc: s.Fn.Name.Loc, Data: &js_ast.EIdentifier{Ref: s.Fn.Name.Ref}},
					})
				}
			}

			// The last function statement for a given symbol wins
			s.Fn.Name = nil
			letDecls[index].ValueOrNil = js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EFunction{Fn: s.Fn}}
		}

		// Reuse memory from "before"
		before = before[:0]
		kind := js_ast.LocalLet
		if p.options.unsupportedJSFeatures.Has(compat.ConstAndLet) {
			kind = js_ast.LocalVar
		}
		if len(letDecls) > 0 {
			before = append(before, js_ast.Stmt{Loc: letDecls[0].ValueOrNil.Loc, Data: &js_ast.SLocal{Kind: kind, Decls: letDecls}})
		}
		if len(varDecls) > 0 {
			// Potentially relocate "var" declarations to the top level
			if assign, ok := p.maybeRelocateVarsToTopLevel(varDecls, relocateVarsNormal); ok {
				if assign.Data != nil {
					before = append(before, assign)
				}
			} else {
				before = append(before, js_ast.Stmt{Loc: varDecls[0].ValueOrNil.Loc, Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: varDecls}})
			}
		}
		before = append(before, nonFnStmts...)
		visited = append(before, visited...)
	}

	// Move TypeScript "export =" statements to the end
	visited = append(visited, after...)

	// Restore the current control-flow liveness if it was changed inside the
	// loop above. This is important because the caller will not restore it.
	p.isControlFlowDead = oldIsControlFlowDead

	// Lower using declarations
	if kind != stmtsSwitch && p.shouldLowerUsingDeclarations(visited) {
		ctx := p.lowerUsingDeclarationContext()
		ctx.scanStmts(p, visited)
		visited = ctx.finalize(p, visited, p.currentScope.Parent == nil)
	}

	// Stop now if we're not mangling
	if !p.options.minifySyntax {
		return visited
	}

	// If this is in a dead branch, trim as much dead code as we can
	if p.isControlFlowDead {
		end := 0
		for _, stmt := range visited {
			if !shouldKeepStmtInDeadControlFlow(stmt) {
				continue
			}

			// Merge adjacent var statements
			if s, ok := stmt.Data.(*js_ast.SLocal); ok && s.Kind == js_ast.LocalVar && end > 0 {
				prevStmt := visited[end-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SLocal); ok && prevS.Kind == js_ast.LocalVar && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

			visited[end] = stmt
			end++
		}
		return visited[:end]
	}

	return p.mangleStmts(visited, kind)
}

func (p *parser) mangleStmts(stmts []js_ast.Stmt, kind stmtsKind) []js_ast.Stmt {
	// Remove inlined constants now that we know whether any of these statements
	// contained a direct eval() or not. This can't be done earlier when we
	// encounter the constant because we haven't encountered the eval() yet.
	// Inlined constants are not removed if they are in a top-level scope or
	// if they are exported (which could be in a nested TypeScript namespace).
	if p.currentScope.Parent != nil && !p.currentScope.ContainsDirectEval {
		for i, stmt := range stmts {
			switch s := stmt.Data.(type) {
			case *js_ast.SEmpty, *js_ast.SComment, *js_ast.SDirective, *js_ast.SDebugger, *js_ast.STypeScript:
				continue

			case *js_ast.SLocal:
				if !s.IsExport {
					end := 0
					for _, d := range s.Decls {
						if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
							if _, ok := p.constValues[id.Ref]; ok && p.symbols[id.Ref.InnerIndex].UseCountEstimate == 0 {
								continue
							}
						}
						s.Decls[end] = d
						end++
					}
					if end == 0 {
						stmts[i].Data = js_ast.SEmptyShared
					} else {
						s.Decls = s.Decls[:end]
					}
				}
				continue
			}
			break
		}
	}

	// Merge adjacent statements during mangling
	result := make([]js_ast.Stmt, 0, len(stmts))
	isControlFlowDead := false
	for i, stmt := range stmts {
		if isControlFlowDead && !shouldKeepStmtInDeadControlFlow(stmt) {
			// Strip unnecessary statements if the control flow is dead here
			continue
		}

		// Inline single-use variable declarations where possible:
		//
		//   // Before
		//   let x = fn();
		//   return x.y();
		//
		//   // After
		//   return fn().y();
		//
		// The declaration must not be exported. We can't just check for the
		// "export" keyword because something might do "export {id};" later on.
		// Instead we just ignore all top-level declarations for now. That means
		// this optimization currently only applies in nested scopes.
		//
		// Ignore declarations if the scope is shadowed by a direct "eval" call.
		// The eval'd code may indirectly reference this symbol and the actual
		// use count may be greater than 1.
		if p.currentScope != p.moduleScope && !p.currentScope.ContainsDirectEval {
			// Keep inlining variables until a failure or until there are none left.
			// That handles cases like this:
			//
			//   // Before
			//   let x = fn();
			//   let y = x.prop;
			//   return y;
			//
			//   // After
			//   return fn().prop;
			//
			for len(result) > 0 {
				// Ignore "var" declarations since those have function-level scope and
				// we may not have visited all of their uses yet by this point. We
				// should have visited all the uses of "let" and "const" declarations
				// by now since they are scoped to this block which we just finished
				// visiting.
				if prevS, ok := result[len(result)-1].Data.(*js_ast.SLocal); ok && prevS.Kind != js_ast.LocalVar {
					last := prevS.Decls[len(prevS.Decls)-1]

					// The binding must be an identifier that is only used once.
					// Ignore destructuring bindings since that's not the simple case.
					// Destructuring bindings could potentially execute side-effecting
					// code which would invalidate reordering.
					if id, ok := last.Binding.Data.(*js_ast.BIdentifier); ok {
						// Don't do this if "__name" was called on this symbol. In that
						// case there is actually more than one use even though it says
						// there is only one. The "__name" use isn't counted so that
						// tree shaking still works when names are kept.
						if symbol := p.symbols[id.Ref.InnerIndex]; symbol.UseCountEstimate == 1 && !symbol.Flags.Has(ast.DidKeepName) {
							replacement := last.ValueOrNil

							// The variable must be initialized, since we will be substituting
							// the value into the usage.
							if replacement.Data == nil {
								replacement = js_ast.Expr{Loc: last.Binding.Loc, Data: js_ast.EUndefinedShared}
							}

							// Try to substitute the identifier with the initializer. This will
							// fail if something with side effects is in between the declaration
							// and the usage.
							if p.substituteSingleUseSymbolInStmt(stmt, id.Ref, replacement) {
								// Remove the previous declaration, since the substitution was
								// successful.
								if len(prevS.Decls) == 1 {
									result = result[:len(result)-1]
								} else {
									prevS.Decls = prevS.Decls[:len(prevS.Decls)-1]
								}

								// Loop back to try again
								continue
							}
						}
					}
				}

				// Substitution failed so stop trying
				break
			}
		}

		switch s := stmt.Data.(type) {
		case *js_ast.SEmpty:
			// Strip empty statements
			continue

		case *js_ast.SLocal:
			// Merge adjacent local statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SLocal); ok && s.Kind == prevS.Kind && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

		case *js_ast.SExpr:
			// Merge adjacent expression statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok {
					if !s.IsFromClassOrFnThatCanBeRemovedIfUnused {
						prevS.IsFromClassOrFnThatCanBeRemovedIfUnused = false
					}
					prevS.Value = js_ast.JoinWithComma(prevS.Value, s.Value)
					continue
				}
			}

		case *js_ast.SSwitch:
			// Absorb a previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok {
					s.Test = js_ast.JoinWithComma(prevS.Value, s.Test)
					result = result[:len(result)-1]
				}
			}

		case *js_ast.SIf:
			// Absorb a previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok {
					s.Test = js_ast.JoinWithComma(prevS.Value, s.Test)
					result = result[:len(result)-1]
				}
			}

			if isJumpStatement(s.Yes.Data) {
				optimizeImplicitJump := false

				// Absorb a previous if statement
				if len(result) > 0 {
					prevStmt := result[len(result)-1]
					if prevS, ok := prevStmt.Data.(*js_ast.SIf); ok && prevS.NoOrNil.Data == nil && jumpStmtsLookTheSame(prevS.Yes.Data, s.Yes.Data) {
						// "if (a) break c; if (b) break c;" => "if (a || b) break c;"
						// "if (a) continue c; if (b) continue c;" => "if (a || b) continue c;"
						// "if (a) return c; if (b) return c;" => "if (a || b) return c;"
						// "if (a) throw c; if (b) throw c;" => "if (a || b) throw c;"
						s.Test = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, prevS.Test, s.Test)
						result = result[:len(result)-1]
					}
				}

				// "while (x) { if (y) continue; z(); }" => "while (x) { if (!y) z(); }"
				// "while (x) { if (y) continue; else z(); w(); }" => "while (x) { if (!y) { z(); w(); } }" => "for (; x;) !y && (z(), w());"
				if kind == stmtsLoopBody {
					if continueS, ok := s.Yes.Data.(*js_ast.SContinue); ok && continueS.Label == nil {
						optimizeImplicitJump = true
					}
				}

				// "let x = () => { if (y) return; z(); };" => "let x = () => { if (!y) z(); };"
				// "let x = () => { if (y) return; else z(); w(); };" => "let x = () => { if (!y) { z(); w(); } };" => "let x = () => { !y && (z(), w()); };"
				if kind == stmtsFnBody {
					if returnS, ok := s.Yes.Data.(*js_ast.SReturn); ok && returnS.ValueOrNil.Data == nil {
						optimizeImplicitJump = true
					}
				}

				if optimizeImplicitJump {
					var body []js_ast.Stmt
					if s.NoOrNil.Data != nil {
						body = append(body, s.NoOrNil)
					}
					body = append(body, stmts[i+1:]...)

					// Don't do this transformation if the branch condition could
					// potentially access symbols declared later on on this scope below.
					// If so, inverting the branch condition and nesting statements after
					// this in a block would break that access which is a behavior change.
					//
					//   // This transformation is incorrect
					//   if (a()) return; function a() {}
					//   if (!a()) { function a() {} }
					//
					//   // This transformation is incorrect
					//   if (a(() => b)) return; let b;
					//   if (a(() => b)) { let b; }
					//
					canMoveBranchConditionOutsideScope := true
					for _, stmt := range body {
						if statementCaresAboutScope(stmt) {
							canMoveBranchConditionOutsideScope = false
							break
						}
					}

					if canMoveBranchConditionOutsideScope {
						body = p.mangleStmts(body, kind)
						bodyLoc := s.Yes.Loc
						if len(body) > 0 {
							bodyLoc = body[0].Loc
						}
						return p.mangleIf(result, stmt.Loc, &js_ast.SIf{
							Test: p.astHelpers.SimplifyBooleanExpr(js_ast.Not(s.Test)),
							Yes:  stmtsToSingleStmt(bodyLoc, body, logger.Loc{}),
						})
					}
				}

				if s.NoOrNil.Data != nil {
					// "if (a) return b; else if (c) return d; else return e;" => "if (a) return b; if (c) return d; return e;"
					for {
						result = append(result, stmt)
						stmt = s.NoOrNil
						s.NoOrNil = js_ast.Stmt{}
						var ok bool
						s, ok = stmt.Data.(*js_ast.SIf)
						if !ok || !isJumpStatement(s.Yes.Data) || s.NoOrNil.Data == nil {
							break
						}
					}
					result = appendIfOrLabelBodyPreservingScope(result, stmt)
					if isJumpStatement(stmt.Data) {
						isControlFlowDead = true
					}
					continue
				}
			}

		case *js_ast.SReturn:
			// Merge return statements with the previous expression statement
			if len(result) > 0 && s.ValueOrNil.Data != nil {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok {
					result[len(result)-1] = js_ast.Stmt{Loc: prevStmt.Loc,
						Data: &js_ast.SReturn{ValueOrNil: js_ast.JoinWithComma(prevS.Value, s.ValueOrNil)}}
					continue
				}
			}

			isControlFlowDead = true

		case *js_ast.SThrow:
			// Merge throw statements with the previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok {
					result[len(result)-1] = js_ast.Stmt{Loc: prevStmt.Loc, Data: &js_ast.SThrow{Value: js_ast.JoinWithComma(prevS.Value, s.Value)}}
					continue
				}
			}

			isControlFlowDead = true

		case *js_ast.SBreak, *js_ast.SContinue:
			isControlFlowDead = true

		case *js_ast.SFor:
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok {
					// Insert the previous expression into the for loop initializer
					if s.InitOrNil.Data == nil {
						result[len(result)-1] = stmt
						s.InitOrNil = js_ast.Stmt{Loc: prevStmt.Loc, Data: &js_ast.SExpr{Value: prevS.Value}}
						continue
					} else if s2, ok := s.InitOrNil.Data.(*js_ast.SExpr); ok {
						result[len(result)-1] = stmt
						s.InitOrNil = js_ast.Stmt{Loc: prevStmt.Loc, Data: &js_ast.SExpr{Value: js_ast.JoinWithComma(prevS.Value, s2.Value)}}
						continue
					}
				} else {
					// Insert the previous variable declaration into the for loop
					// initializer if it's a "var" declaration, since the scope
					// doesn't matter due to scope hoisting
					if s.InitOrNil.Data == nil {
						if s2, ok := prevStmt.Data.(*js_ast.SLocal); ok && s2.Kind == js_ast.LocalVar && !s2.IsExport {
							result[len(result)-1] = stmt
							s.InitOrNil = prevStmt
							continue
						}
					} else {
						if s2, ok := prevStmt.Data.(*js_ast.SLocal); ok && s2.Kind == js_ast.LocalVar && !s2.IsExport {
							if s3, ok := s.InitOrNil.Data.(*js_ast.SLocal); ok && s3.Kind == js_ast.LocalVar {
								result[len(result)-1] = stmt
								s.InitOrNil.Data = &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: append(s2.Decls, s3.Decls...)}
								continue
							}
						}
					}
				}
			}

		case *js_ast.STry:
			// Drop an unused identifier binding if the optional catch binding feature is supported
			if !p.options.unsupportedJSFeatures.Has(compat.OptionalCatchBinding) && s.Catch != nil {
				if id, ok := s.Catch.BindingOrNil.Data.(*js_ast.BIdentifier); ok {
					if symbol := p.symbols[id.Ref.InnerIndex]; symbol.UseCountEstimate == 0 {
						if symbol.Link != ast.InvalidRef {
							// We cannot transform "try { x() } catch (y) { var y = 1 }" into
							// "try { x() } catch { var y = 1 }" even though "y" is never used
							// because the hoisted variable "y" would have different values
							// after the statement ends due to a strange JavaScript quirk:
							//
							//   try { x() } catch (y) { var y = 1 }
							//   console.log(y) // undefined
							//
							//   try { x() } catch { var y = 1 }
							//   console.log(y) // 1
							//
						} else if p.currentScope.ContainsDirectEval {
							// We cannot transform "try { x() } catch (y) { eval('z = y') }"
							// into "try { x() } catch { eval('z = y') }" because the variable
							// "y" is actually still used.
						} else {
							// "try { x() } catch (y) {}" => "try { x() } catch {}"
							s.Catch.BindingOrNil.Data = nil
						}
					}
				}
			}
		}

		result = append(result, stmt)
	}

	// Drop a trailing unconditional jump statement if applicable
	if len(result) > 0 {
		switch kind {
		case stmtsLoopBody:
			// "while (x) { y(); continue; }" => "while (x) { y(); }"
			if continueS, ok := result[len(result)-1].Data.(*js_ast.SContinue); ok && continueS.Label == nil {
				result = result[:len(result)-1]
			}

		case stmtsFnBody:
			// "function f() { x(); return; }" => "function f() { x(); }"
			if returnS, ok := result[len(result)-1].Data.(*js_ast.SReturn); ok && returnS.ValueOrNil.Data == nil {
				result = result[:len(result)-1]
			}
		}
	}

	// Merge certain statements in reverse order
	if len(result) >= 2 {
		lastStmt := result[len(result)-1]

		if lastReturn, ok := lastStmt.Data.(*js_ast.SReturn); ok {
			// "if (a) return b; if (c) return d; return e;" => "return a ? b : c ? d : e;"
		returnLoop:
			for len(result) >= 2 {
				prevIndex := len(result) - 2
				prevStmt := result[prevIndex]

				switch prevS := prevStmt.Data.(type) {
				case *js_ast.SExpr:
					// This return statement must have a value
					if lastReturn.ValueOrNil.Data == nil {
						break returnLoop
					}

					// "a(); return b;" => "return a(), b;"
					lastReturn = &js_ast.SReturn{ValueOrNil: js_ast.JoinWithComma(prevS.Value, lastReturn.ValueOrNil)}

					// Merge the last two statements
					lastStmt = js_ast.Stmt{Loc: prevStmt.Loc, Data: lastReturn}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				case *js_ast.SIf:
					// The previous statement must be an if statement with no else clause
					if prevS.NoOrNil.Data != nil {
						break returnLoop
					}

					// The then clause must be a return
					prevReturn, ok := prevS.Yes.Data.(*js_ast.SReturn)
					if !ok {
						break returnLoop
					}

					// Handle some or all of the values being undefined
					left := prevReturn.ValueOrNil
					right := lastReturn.ValueOrNil
					if left.Data == nil {
						// "if (a) return; return b;" => "return a ? void 0 : b;"
						left = js_ast.Expr{Loc: prevS.Yes.Loc, Data: js_ast.EUndefinedShared}
					}
					if right.Data == nil {
						// "if (a) return a; return;" => "return a ? b : void 0;"
						right = js_ast.Expr{Loc: lastStmt.Loc, Data: js_ast.EUndefinedShared}
					}

					// "if (!a) return b; return c;" => "return a ? c : b;"
					if not, ok := prevS.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
						prevS.Test = not.Value
						left, right = right, left
					}

					if comma, ok := prevS.Test.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
						// "if (a, b) return c; return d;" => "return a, b ? c : d;"
						lastReturn = &js_ast.SReturn{ValueOrNil: js_ast.JoinWithComma(comma.Left,
							p.astHelpers.MangleIfExpr(comma.Right.Loc, &js_ast.EIf{Test: comma.Right, Yes: left, No: right}, p.options.unsupportedJSFeatures))}
					} else {
						// "if (a) return b; return c;" => "return a ? b : c;"
						lastReturn = &js_ast.SReturn{ValueOrNil: p.astHelpers.MangleIfExpr(
							prevS.Test.Loc, &js_ast.EIf{Test: prevS.Test, Yes: left, No: right}, p.options.unsupportedJSFeatures)}
					}

					// Merge the last two statements
					lastStmt = js_ast.Stmt{Loc: prevStmt.Loc, Data: lastReturn}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				default:
					break returnLoop
				}
			}
		} else if lastThrow, ok := lastStmt.Data.(*js_ast.SThrow); ok {
			// "if (a) throw b; if (c) throw d; throw e;" => "throw a ? b : c ? d : e;"
		throwLoop:
			for len(result) >= 2 {
				prevIndex := len(result) - 2
				prevStmt := result[prevIndex]

				switch prevS := prevStmt.Data.(type) {
				case *js_ast.SExpr:
					// "a(); throw b;" => "throw a(), b;"
					lastThrow = &js_ast.SThrow{Value: js_ast.JoinWithComma(prevS.Value, lastThrow.Value)}

					// Merge the last two statements
					lastStmt = js_ast.Stmt{Loc: prevStmt.Loc, Data: lastThrow}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				case *js_ast.SIf:
					// The previous statement must be an if statement with no else clause
					if prevS.NoOrNil.Data != nil {
						break throwLoop
					}

					// The then clause must be a throw
					prevThrow, ok := prevS.Yes.Data.(*js_ast.SThrow)
					if !ok {
						break throwLoop
					}

					left := prevThrow.Value
					right := lastThrow.Value

					// "if (!a) throw b; throw c;" => "throw a ? c : b;"
					if not, ok := prevS.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
						prevS.Test = not.Value
						left, right = right, left
					}

					// Merge the last two statements
					if comma, ok := prevS.Test.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
						// "if (a, b) return c; return d;" => "return a, b ? c : d;"
						lastThrow = &js_ast.SThrow{Value: js_ast.JoinWithComma(comma.Left,
							p.astHelpers.MangleIfExpr(comma.Right.Loc, &js_ast.EIf{Test: comma.Right, Yes: left, No: right}, p.options.unsupportedJSFeatures))}
					} else {
						// "if (a) return b; return c;" => "return a ? b : c;"
						lastThrow = &js_ast.SThrow{
							Value: p.astHelpers.MangleIfExpr(prevS.Test.Loc, &js_ast.EIf{Test: prevS.Test, Yes: left, No: right}, p.options.unsupportedJSFeatures)}
					}
					lastStmt = js_ast.Stmt{Loc: prevStmt.Loc, Data: lastThrow}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				default:
					break throwLoop
				}
			}
		}
	}

	return result
}

func (p *parser) substituteSingleUseSymbolInStmt(stmt js_ast.Stmt, ref ast.Ref, replacement js_ast.Expr) bool {
	var expr *js_ast.Expr

	switch s := stmt.Data.(type) {
	case *js_ast.SExpr:
		expr = &s.Value
	case *js_ast.SThrow:
		expr = &s.Value
	case *js_ast.SReturn:
		expr = &s.ValueOrNil
	case *js_ast.SIf:
		expr = &s.Test
	case *js_ast.SSwitch:
		expr = &s.Test
	case *js_ast.SLocal:
		// Only try substituting into the initializer for the first declaration
		if first := &s.Decls[0]; first.ValueOrNil.Data != nil {
			// Make sure there isn't destructuring, which could evaluate code
			if _, ok := first.Binding.Data.(*js_ast.BIdentifier); ok {
				expr = &first.ValueOrNil
			}
		}
	}

	if expr != nil {
		// Only continue trying to insert this replacement into sub-expressions
		// after the first one if the replacement has no side effects:
		//
		//   // Substitution is ok
		//   let replacement = 123;
		//   return x + replacement;
		//
		//   // Substitution is not ok because "fn()" may change "x"
		//   let replacement = fn();
		//   return x + replacement;
		//
		//   // Substitution is not ok because "x == x" may change "x" due to "valueOf()" evaluation
		//   let replacement = [x];
		//   return (x == x) + replacement;
		//
		replacementCanBeRemoved := p.astHelpers.ExprCanBeRemovedIfUnused(replacement)

		if new, status := p.substituteSingleUseSymbolInExpr(*expr, ref, replacement, replacementCanBeRemoved); status == substituteSuccess {
			*expr = new
			return true
		}
	}

	return false
}

type substituteStatus uint8

const (
	substituteContinue substituteStatus = iota
	substituteSuccess
	substituteFailure
)

func (p *parser) substituteSingleUseSymbolInExpr(
	expr js_ast.Expr,
	ref ast.Ref,
	replacement js_ast.Expr,
	replacementCanBeRemoved bool,
) (js_ast.Expr, substituteStatus) {
	switch e := expr.Data.(type) {
	case *js_ast.EIdentifier:
		if e.Ref == ref {
			p.ignoreUsage(ref)
			return replacement, substituteSuccess
		}

	case *js_ast.ESpread:
		if value, status := p.substituteSingleUseSymbolInExpr(e.Value, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Value = value
			return expr, status
		}

	case *js_ast.EAwait:
		if value, status := p.substituteSingleUseSymbolInExpr(e.Value, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Value = value
			return expr, status
		}

	case *js_ast.EYield:
		if e.ValueOrNil.Data != nil {
			if value, status := p.substituteSingleUseSymbolInExpr(e.ValueOrNil, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.ValueOrNil = value
				return expr, status
			}
		}

	case *js_ast.EImportCall:
		if value, status := p.substituteSingleUseSymbolInExpr(e.Expr, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Expr = value
			return expr, status
		}

		// The "import()" expression has side effects but the side effects are
		// always asynchronous so there is no way for the side effects to modify
		// the replacement value. So it's ok to reorder the replacement value
		// past the "import()" expression assuming everything else checks out.
		if replacementCanBeRemoved && p.astHelpers.ExprCanBeRemovedIfUnused(e.Expr) {
			return expr, substituteContinue
		}

	case *js_ast.EUnary:
		switch e.Op {
		case js_ast.UnOpPreInc, js_ast.UnOpPostInc, js_ast.UnOpPreDec, js_ast.UnOpPostDec, js_ast.UnOpDelete:
			// Do not substitute into an assignment position

		default:
			if value, status := p.substituteSingleUseSymbolInExpr(e.Value, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.Value = value
				return expr, status
			}
		}

	case *js_ast.EDot:
		if value, status := p.substituteSingleUseSymbolInExpr(e.Target, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Target = value
			return expr, status
		}

	case *js_ast.EBinary:
		// Do not substitute into an assignment position
		if e.Op.BinaryAssignTarget() == js_ast.AssignTargetNone {
			if value, status := p.substituteSingleUseSymbolInExpr(e.Left, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.Left = value
				return expr, status
			}
		} else if !p.astHelpers.ExprCanBeRemovedIfUnused(e.Left) {
			// Do not reorder past a side effect in an assignment target, as that may
			// change the replacement value. For example, "fn()" may change "a" here:
			//
			//   let a = 1;
			//   foo[fn()] = a;
			//
			return expr, substituteFailure
		} else if e.Op.BinaryAssignTarget() == js_ast.AssignTargetUpdate && !replacementCanBeRemoved {
			// If this is a read-modify-write assignment and the replacement has side
			// effects, don't reorder it past the assignment target. The assignment
			// target is being read so it may be changed by the side effect. For
			// example, "fn()" may change "foo" here:
			//
			//   let a = fn();
			//   foo += a;
			//
			return expr, substituteFailure
		}

		// If we get here then it should be safe to attempt to substitute the
		// replacement past the left operand into the right operand.
		if value, status := p.substituteSingleUseSymbolInExpr(e.Right, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Right = value
			return expr, status
		}

	case *js_ast.EIf:
		if value, status := p.substituteSingleUseSymbolInExpr(e.Test, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Test = value
			return expr, status
		}

		// Do not substitute our unconditionally-executed value into a branch
		// unless the value itself has no side effects
		if replacementCanBeRemoved {
			// Unlike other branches in this function such as "a && b" or "a?.[b]",
			// the "a ? b : c" form has potential code evaluation along both control
			// flow paths. Handle this by allowing substitution into either branch.
			// Side effects in one branch should not prevent the substitution into
			// the other branch.

			yesValue, yesStatus := p.substituteSingleUseSymbolInExpr(e.Yes, ref, replacement, replacementCanBeRemoved)
			if yesStatus == substituteSuccess {
				e.Yes = yesValue
				return expr, yesStatus
			}

			noValue, noStatus := p.substituteSingleUseSymbolInExpr(e.No, ref, replacement, replacementCanBeRemoved)
			if noStatus == substituteSuccess {
				e.No = noValue
				return expr, noStatus
			}

			// Side effects in either branch should stop us from continuing to try to
			// substitute the replacement after the control flow branches merge again.
			if yesStatus != substituteContinue || noStatus != substituteContinue {
				return expr, substituteFailure
			}
		}

	case *js_ast.EIndex:
		if value, status := p.substituteSingleUseSymbolInExpr(e.Target, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Target = value
			return expr, status
		}

		// Do not substitute our unconditionally-executed value into a branch
		// unless the value itself has no side effects
		if replacementCanBeRemoved || e.OptionalChain == js_ast.OptionalChainNone {
			if value, status := p.substituteSingleUseSymbolInExpr(e.Index, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.Index = value
				return expr, status
			}
		}

	case *js_ast.ECall:
		// Don't substitute something into a call target that could change "this"
		_, isDot := replacement.Data.(*js_ast.EDot)
		_, isIndex := replacement.Data.(*js_ast.EIndex)
		if isDot || isIndex {
			if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok && id.Ref == ref {
				break
			}
		}

		if value, status := p.substituteSingleUseSymbolInExpr(e.Target, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
			e.Target = value
			return expr, status
		}

		// Do not substitute our unconditionally-executed value into a branch
		// unless the value itself has no side effects
		if replacementCanBeRemoved || e.OptionalChain == js_ast.OptionalChainNone {
			for i, arg := range e.Args {
				if value, status := p.substituteSingleUseSymbolInExpr(arg, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
					e.Args[i] = value
					return expr, status
				}
			}
		}

	case *js_ast.EArray:
		for i, item := range e.Items {
			if value, status := p.substituteSingleUseSymbolInExpr(item, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.Items[i] = value
				return expr, status
			}
		}

	case *js_ast.EObject:
		for i, property := range e.Properties {
			// Check the key
			if property.Flags.Has(js_ast.PropertyIsComputed) {
				if value, status := p.substituteSingleUseSymbolInExpr(property.Key, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
					e.Properties[i].Key = value
					return expr, status
				}

				// Stop now because both computed keys and property spread have side effects
				return expr, substituteFailure
			}

			// Check the value
			if property.ValueOrNil.Data != nil {
				if value, status := p.substituteSingleUseSymbolInExpr(property.ValueOrNil, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
					e.Properties[i].ValueOrNil = value
					return expr, status
				}
			}
		}

	case *js_ast.ETemplate:
		if e.TagOrNil.Data != nil {
			if value, status := p.substituteSingleUseSymbolInExpr(e.TagOrNil, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.TagOrNil = value
				return expr, status
			}
		}

		for i, part := range e.Parts {
			if value, status := p.substituteSingleUseSymbolInExpr(part.Value, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.Parts[i].Value = value

				// If we substituted a primitive, merge it into the template
				if js_ast.IsPrimitiveLiteral(value.Data) {
					expr = js_ast.InlinePrimitivesIntoTemplate(expr.Loc, e)
				}
				return expr, status
			}
		}
	}

	// If both the replacement and this expression have no observable side
	// effects, then we can reorder the replacement past this expression
	if replacementCanBeRemoved && p.astHelpers.ExprCanBeRemovedIfUnused(expr) {
		return expr, substituteContinue
	}

	// We can always reorder past primitive values
	if js_ast.IsPrimitiveLiteral(expr.Data) || js_ast.IsPrimitiveLiteral(replacement.Data) {
		return expr, substituteContinue
	}

	// Otherwise we should stop trying to substitute past this point
	return expr, substituteFailure
}

func (p *parser) visitLoopBody(stmt js_ast.Stmt) js_ast.Stmt {
	oldIsInsideLoop := p.fnOrArrowDataVisit.isInsideLoop
	p.fnOrArrowDataVisit.isInsideLoop = true
	p.loopBody = stmt.Data
	stmt = p.visitSingleStmt(stmt, stmtsLoopBody)
	p.fnOrArrowDataVisit.isInsideLoop = oldIsInsideLoop
	return stmt
}

func (p *parser) visitSingleStmt(stmt js_ast.Stmt, kind stmtsKind) js_ast.Stmt {
	// To reduce stack depth, special-case blocks and process their children directly
	if block, ok := stmt.Data.(*js_ast.SBlock); ok {
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		block.Stmts = p.visitStmts(block.Stmts, kind)
		p.popScope()
		if p.options.minifySyntax {
			stmt = stmtsToSingleStmt(stmt.Loc, block.Stmts, block.CloseBraceLoc)
		}
		return stmt
	}

	// Introduce a fake block scope for function declarations inside if statements
	fn, ok := stmt.Data.(*js_ast.SFunction)
	hasIfScope := ok && fn.Fn.HasIfScope
	if hasIfScope {
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		if p.isStrictMode() {
			p.markStrictModeFeature(ifElseFunctionStmt, js_lexer.RangeOfIdentifier(p.source, stmt.Loc), "")
		}
	}

	stmts := p.visitStmts([]js_ast.Stmt{stmt}, kind)

	// Balance the fake block scope introduced above
	if hasIfScope {
		p.popScope()
	}

	return stmtsToSingleStmt(stmt.Loc, stmts, logger.Loc{})
}

// One statement could potentially expand to several statements
func stmtsToSingleStmt(loc logger.Loc, stmts []js_ast.Stmt, closeBraceLoc logger.Loc) js_ast.Stmt {
	if len(stmts) == 0 {
		return js_ast.Stmt{Loc: loc, Data: js_ast.SEmptyShared}
	}
	if len(stmts) == 1 && !statementCaresAboutScope(stmts[0]) {
		return stmts[0]
	}
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc}}
}

func (p *parser) visitForLoopInit(stmt js_ast.Stmt, isInOrOf bool) js_ast.Stmt {
	switch s := stmt.Data.(type) {
	case *js_ast.SExpr:
		assignTarget := js_ast.AssignTargetNone
		if isInOrOf {
			assignTarget = js_ast.AssignTargetReplace
		}
		p.stmtExprValue = s.Value.Data
		s.Value, _ = p.visitExprInOut(s.Value, exprIn{assignTarget: assignTarget})

	case *js_ast.SLocal:
		for i := range s.Decls {
			d := &s.Decls[i]
			p.visitBinding(d.Binding, bindingOpts{})
			if d.ValueOrNil.Data != nil {
				d.ValueOrNil = p.visitExpr(d.ValueOrNil)
			}
		}
		s.Decls = p.lowerObjectRestInDecls(s.Decls)
		s.Kind = p.selectLocalKind(s.Kind)

	default:
		panic("Internal error")
	}

	return stmt
}

func (p *parser) recordDeclaredSymbol(ref ast.Ref) {
	p.declaredSymbols = append(p.declaredSymbols, js_ast.DeclaredSymbol{
		Ref:        ref,
		IsTopLevel: p.currentScope == p.moduleScope,
	})
}

type bindingOpts struct {
	duplicateArgCheck map[string]logger.Range
}

func (p *parser) visitBinding(binding js_ast.Binding, opts bindingOpts) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		p.recordDeclaredSymbol(b.Ref)
		name := p.symbols[b.Ref.InnerIndex].OriginalName
		p.validateDeclaredSymbolName(binding.Loc, name)
		if opts.duplicateArgCheck != nil {
			r := js_lexer.RangeOfIdentifier(p.source, binding.Loc)
			if firstRange := opts.duplicateArgCheck[name]; firstRange.Len > 0 {
				p.log.AddErrorWithNotes(&p.tracker, r,
					fmt.Sprintf("%q cannot be bound multiple times in the same parameter list", name),
					[]logger.MsgData{p.tracker.MsgData(firstRange, fmt.Sprintf("The name %q was originally bound here:", name))})
			} else {
				opts.duplicateArgCheck[name] = r
			}
		}

	case *js_ast.BArray:
		for i := range b.Items {
			item := &b.Items[i]
			p.visitBinding(item.Binding, opts)
			if item.DefaultValueOrNil.Data != nil {
				// Propagate the name to keep from the binding into the initializer
				if id, ok := item.Binding.Data.(*js_ast.BIdentifier); ok {
					p.nameToKeep = p.symbols[id.Ref.InnerIndex].OriginalName
					p.nameToKeepIsFor = item.DefaultValueOrNil.Data
				}

				item.DefaultValueOrNil = p.visitExpr(item.DefaultValueOrNil)
			}
		}

	case *js_ast.BObject:
		for i, property := range b.Properties {
			if !property.IsSpread {
				property.Key, _ = p.visitExprInOut(property.Key, exprIn{
					shouldMangleStringsAsProps: true,
				})
			}
			p.visitBinding(property.Value, opts)
			if property.DefaultValueOrNil.Data != nil {
				// Propagate the name to keep from the binding into the initializer
				if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok {
					p.nameToKeep = p.symbols[id.Ref.InnerIndex].OriginalName
					p.nameToKeepIsFor = property.DefaultValueOrNil.Data
				}

				property.DefaultValueOrNil = p.visitExpr(property.DefaultValueOrNil)
			}
			b.Properties[i] = property
		}

	default:
		panic("Internal error")
	}
}

func statementCaresAboutScope(stmt js_ast.Stmt) bool {
	switch s := stmt.Data.(type) {
	case *js_ast.SBlock, *js_ast.SEmpty, *js_ast.SDebugger, *js_ast.SExpr, *js_ast.SIf,
		*js_ast.SFor, *js_ast.SForIn, *js_ast.SForOf, *js_ast.SDoWhile, *js_ast.SWhile,
		*js_ast.SWith, *js_ast.STry, *js_ast.SSwitch, *js_ast.SReturn, *js_ast.SThrow,
		*js_ast.SBreak, *js_ast.SContinue, *js_ast.SDirective, *js_ast.SLabel:
		return false

	case *js_ast.SLocal:
		return s.Kind != js_ast.LocalVar

	default:
		return true
	}
}

func dropFirstStatement(body js_ast.Stmt, replaceOrNil js_ast.Stmt) js_ast.Stmt {
	if block, ok := body.Data.(*js_ast.SBlock); ok && len(block.Stmts) > 0 {
		if replaceOrNil.Data != nil {
			block.Stmts[0] = replaceOrNil
		} else if len(block.Stmts) == 2 && !statementCaresAboutScope(block.Stmts[1]) {
			return block.Stmts[1]
		} else {
			block.Stmts = block.Stmts[1:]
		}
		return body
	}
	if replaceOrNil.Data != nil {
		return replaceOrNil
	}
	return js_ast.Stmt{Loc: body.Loc, Data: js_ast.SEmptyShared}
}

func mangleFor(s *js_ast.SFor) {
	// Get the first statement in the loop
	first := s.Body
	if block, ok := first.Data.(*js_ast.SBlock); ok && len(block.Stmts) > 0 {
		first = block.Stmts[0]
	}

	if ifS, ok := first.Data.(*js_ast.SIf); ok {
		// "for (;;) if (x) break;" => "for (; !x;) ;"
		// "for (; a;) if (x) break;" => "for (; a && !x;) ;"
		// "for (;;) if (x) break; else y();" => "for (; !x;) y();"
		// "for (; a;) if (x) break; else y();" => "for (; a && !x;) y();"
		if breakS, ok := ifS.Yes.Data.(*js_ast.SBreak); ok && breakS.Label == nil {
			var not js_ast.Expr
			if unary, ok := ifS.Test.Data.(*js_ast.EUnary); ok && unary.Op == js_ast.UnOpNot {
				not = unary.Value
			} else {
				not = js_ast.Not(ifS.Test)
			}
			if s.TestOrNil.Data != nil {
				s.TestOrNil = js_ast.Expr{Loc: s.TestOrNil.Loc, Data: &js_ast.EBinary{
					Op:    js_ast.BinOpLogicalAnd,
					Left:  s.TestOrNil,
					Right: not,
				}}
			} else {
				s.TestOrNil = not
			}
			s.Body = dropFirstStatement(s.Body, ifS.NoOrNil)
			return
		}

		// "for (;;) if (x) y(); else break;" => "for (; x;) y();"
		// "for (; a;) if (x) y(); else break;" => "for (; a && x;) y();"
		if ifS.NoOrNil.Data != nil {
			if breakS, ok := ifS.NoOrNil.Data.(*js_ast.SBreak); ok && breakS.Label == nil {
				if s.TestOrNil.Data != nil {
					s.TestOrNil = js_ast.Expr{Loc: s.TestOrNil.Loc, Data: &js_ast.EBinary{
						Op:    js_ast.BinOpLogicalAnd,
						Left:  s.TestOrNil,
						Right: ifS.Test,
					}}
				} else {
					s.TestOrNil = ifS.Test
				}
				s.Body = dropFirstStatement(s.Body, ifS.Yes)
				return
			}
		}
	}
}

func appendIfOrLabelBodyPreservingScope(stmts []js_ast.Stmt, body js_ast.Stmt) []js_ast.Stmt {
	if block, ok := body.Data.(*js_ast.SBlock); ok {
		keepBlock := false
		for _, stmt := range block.Stmts {
			if statementCaresAboutScope(stmt) {
				keepBlock = true
				break
			}
		}
		if !keepBlock {
			return append(stmts, block.Stmts...)
		}
	}

	if statementCaresAboutScope(body) {
		return append(stmts, js_ast.Stmt{Loc: body.Loc, Data: &js_ast.SBlock{Stmts: []js_ast.Stmt{body}}})
	}

	return append(stmts, body)
}

func (p *parser) mangleIf(stmts []js_ast.Stmt, loc logger.Loc, s *js_ast.SIf) []js_ast.Stmt {
	// Constant folding using the test expression
	if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(s.Test.Data); ok {
		if boolean {
			// The test is truthy
			if s.NoOrNil.Data == nil || !shouldKeepStmtInDeadControlFlow(s.NoOrNil) {
				// We can drop the "no" branch
				if sideEffects == js_ast.CouldHaveSideEffects {
					// Keep the condition if it could have side effects (but is still known to be truthy)
					if test := p.astHelpers.SimplifyUnusedExpr(s.Test, p.options.unsupportedJSFeatures); test.Data != nil {
						stmts = append(stmts, js_ast.Stmt{Loc: s.Test.Loc, Data: &js_ast.SExpr{Value: test}})
					}
				}
				return appendIfOrLabelBodyPreservingScope(stmts, s.Yes)
			} else {
				// We have to keep the "no" branch
			}
		} else {
			// The test is falsy
			if !shouldKeepStmtInDeadControlFlow(s.Yes) {
				// We can drop the "yes" branch
				if sideEffects == js_ast.CouldHaveSideEffects {
					// Keep the condition if it could have side effects (but is still known to be falsy)
					if test := p.astHelpers.SimplifyUnusedExpr(s.Test, p.options.unsupportedJSFeatures); test.Data != nil {
						stmts = append(stmts, js_ast.Stmt{Loc: s.Test.Loc, Data: &js_ast.SExpr{Value: test}})
					}
				}
				if s.NoOrNil.Data == nil {
					return stmts
				}
				return appendIfOrLabelBodyPreservingScope(stmts, s.NoOrNil)
			} else {
				// We have to keep the "yes" branch
			}
		}

		// Use "1" and "0" instead of "true" and "false" to be shorter
		if sideEffects == js_ast.NoSideEffects {
			if boolean {
				s.Test.Data = &js_ast.ENumber{Value: 1}
			} else {
				s.Test.Data = &js_ast.ENumber{Value: 0}
			}
		}
	}

	var expr js_ast.Expr

	if yes, ok := s.Yes.Data.(*js_ast.SExpr); ok {
		// "yes" is an expression
		if s.NoOrNil.Data == nil {
			if not, ok := s.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
				// "if (!a) b();" => "a || b();"
				expr = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, not.Value, yes.Value)
			} else {
				// "if (a) b();" => "a && b();"
				expr = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, s.Test, yes.Value)
			}
		} else if no, ok := s.NoOrNil.Data.(*js_ast.SExpr); ok {
			// "if (a) b(); else c();" => "a ? b() : c();"
			expr = p.astHelpers.MangleIfExpr(loc, &js_ast.EIf{
				Test: s.Test,
				Yes:  yes.Value,
				No:   no.Value,
			}, p.options.unsupportedJSFeatures)
		}
	} else if _, ok := s.Yes.Data.(*js_ast.SEmpty); ok {
		// "yes" is missing
		if s.NoOrNil.Data == nil {
			// "yes" and "no" are both missing
			if p.astHelpers.ExprCanBeRemovedIfUnused(s.Test) {
				// "if (1) {}" => ""
				return stmts
			} else {
				// "if (a) {}" => "a;"
				expr = s.Test
			}
		} else if no, ok := s.NoOrNil.Data.(*js_ast.SExpr); ok {
			if not, ok := s.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
				// "if (!a) {} else b();" => "a && b();"
				expr = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, not.Value, no.Value)
			} else {
				// "if (a) {} else b();" => "a || b();"
				expr = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, s.Test, no.Value)
			}
		} else {
			// "yes" is missing and "no" is not missing (and is not an expression)
			if not, ok := s.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
				// "if (!a) {} else throw b;" => "if (a) throw b;"
				s.Test = not.Value
				s.Yes = s.NoOrNil
				s.NoOrNil = js_ast.Stmt{}
			} else {
				// "if (a) {} else throw b;" => "if (!a) throw b;"
				s.Test = js_ast.Not(s.Test)
				s.Yes = s.NoOrNil
				s.NoOrNil = js_ast.Stmt{}
			}
		}
	} else {
		// "yes" is not missing (and is not an expression)
		if s.NoOrNil.Data != nil {
			// "yes" is not missing (and is not an expression) and "no" is not missing
			if not, ok := s.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
				// "if (!a) return b; else return c;" => "if (a) return c; else return b;"
				s.Test = not.Value
				s.Yes, s.NoOrNil = s.NoOrNil, s.Yes
			}
		} else {
			// "no" is missing
			if s2, ok := s.Yes.Data.(*js_ast.SIf); ok && s2.NoOrNil.Data == nil {
				// "if (a) if (b) return c;" => "if (a && b) return c;"
				s.Test = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, s.Test, s2.Test)
				s.Yes = s2.Yes
			}
		}
	}

	// Return an expression if we replaced the if statement with an expression above
	if expr.Data != nil {
		expr = p.astHelpers.SimplifyUnusedExpr(expr, p.options.unsupportedJSFeatures)
		return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}})
	}

	return append(stmts, js_ast.Stmt{Loc: loc, Data: s})
}

func (p *parser) keepExprSymbolName(value js_ast.Expr, name string) js_ast.Expr {
	value = p.callRuntime(value.Loc, "__name", []js_ast.Expr{value,
		{Loc: value.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(name)}},
	})

	// Make sure tree shaking removes this if the function is never used
	value.Data.(*js_ast.ECall).CanBeUnwrappedIfUnused = true
	return value
}

func (p *parser) keepClassOrFnSymbolName(loc logger.Loc, expr js_ast.Expr, name string) js_ast.Stmt {
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{
		Value: p.callRuntime(loc, "__name", []js_ast.Expr{
			expr,
			{Loc: loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(name)}},
		}),
		IsFromClassOrFnThatCanBeRemovedIfUnused: true,
	}}
}

func (p *parser) visitAndAppendStmt(stmts []js_ast.Stmt, stmt js_ast.Stmt) []js_ast.Stmt {
	// By default any statement ends the const local prefix
	wasAfterAfterConstLocalPrefix := p.currentScope.IsAfterConstLocalPrefix
	p.currentScope.IsAfterConstLocalPrefix = true

	switch s := stmt.Data.(type) {
	case *js_ast.SEmpty, *js_ast.SComment:
		// Comments do not end the const local prefix
		p.currentScope.IsAfterConstLocalPrefix = wasAfterAfterConstLocalPrefix

	case *js_ast.SDebugger:
		// Debugger statements do not end the const local prefix
		p.currentScope.IsAfterConstLocalPrefix = wasAfterAfterConstLocalPrefix

		if p.options.dropDebugger {
			return stmts
		}

	case *js_ast.STypeScript:
		// Type annotations do not end the const local prefix
		p.currentScope.IsAfterConstLocalPrefix = wasAfterAfterConstLocalPrefix

		// Erase TypeScript constructs from the output completely
		return stmts

	case *js_ast.SDirective:
		// Directives do not end the const local prefix
		p.currentScope.IsAfterConstLocalPrefix = wasAfterAfterConstLocalPrefix

		if p.isStrictMode() && s.LegacyOctalLoc.Start > 0 {
			p.markStrictModeFeature(legacyOctalEscape, p.source.RangeOfLegacyOctalEscape(s.LegacyOctalLoc), "")
		}

	case *js_ast.SImport:
		p.recordDeclaredSymbol(s.NamespaceRef)

		if s.DefaultName != nil {
			p.recordDeclaredSymbol(s.DefaultName.Ref)
		}

		if s.Items != nil {
			for _, item := range *s.Items {
				p.recordDeclaredSymbol(item.Name.Ref)
			}
		}

	case *js_ast.SExportClause:
		// "export {foo}"
		end := 0
		for _, item := range s.Items {
			name := p.loadNameFromRef(item.Name.Ref)
			ref := p.findSymbol(item.AliasLoc, name).ref

			if p.symbols[ref.InnerIndex].Kind == ast.SymbolUnbound {
				// Silently strip exports of non-local symbols in TypeScript, since
				// those likely correspond to type-only exports. But report exports of
				// non-local symbols as errors in JavaScript.
				if !p.options.ts.Parse {
					r := js_lexer.RangeOfIdentifier(p.source, item.Name.Loc)
					p.log.AddError(&p.tracker, r, fmt.Sprintf("%q is not declared in this file", name))
				}
				continue
			}

			item.Name.Ref = ref
			s.Items[end] = item
			end++
		}

		// Note: do not remove empty export statements since TypeScript uses them as module markers
		s.Items = s.Items[:end]

	case *js_ast.SExportFrom:
		// "export {foo} from 'path'"
		name := p.loadNameFromRef(s.NamespaceRef)
		s.NamespaceRef = p.newSymbol(ast.SymbolOther, name)
		p.currentScope.Generated = append(p.currentScope.Generated, s.NamespaceRef)
		p.recordDeclaredSymbol(s.NamespaceRef)

		// This is a re-export and the symbols created here are used to reference
		// names in another file. This means the symbols are really aliases.
		for i, item := range s.Items {
			name := p.loadNameFromRef(item.Name.Ref)
			ref := p.newSymbol(ast.SymbolOther, name)
			p.currentScope.Generated = append(p.currentScope.Generated, ref)
			p.recordDeclaredSymbol(ref)
			s.Items[i].Name.Ref = ref
		}

	case *js_ast.SExportStar:
		// "export * from 'path'"
		// "export * as ns from 'path'"
		name := p.loadNameFromRef(s.NamespaceRef)
		s.NamespaceRef = p.newSymbol(ast.SymbolOther, name)
		p.currentScope.Generated = append(p.currentScope.Generated, s.NamespaceRef)
		p.recordDeclaredSymbol(s.NamespaceRef)

		// "export * as ns from 'path'"
		if s.Alias != nil {
			// "import * as ns from 'path'"
			// "export {ns}"
			if p.options.unsupportedJSFeatures.Has(compat.ExportStarAs) {
				p.recordUsage(s.NamespaceRef)
				return append(stmts,
					js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SImport{
						NamespaceRef:      s.NamespaceRef,
						StarNameLoc:       &s.Alias.Loc,
						ImportRecordIndex: s.ImportRecordIndex,
					}},
					js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SExportClause{
						Items: []js_ast.ClauseItem{{
							Alias:        s.Alias.OriginalName,
							OriginalName: s.Alias.OriginalName,
							AliasLoc:     s.Alias.Loc,
							Name:         ast.LocRef{Loc: s.Alias.Loc, Ref: s.NamespaceRef},
						}},
						IsSingleLine: true,
					}},
				)
			}
		}

	case *js_ast.SExportDefault:
		p.recordDeclaredSymbol(s.DefaultName.Ref)

		switch s2 := s.Value.Data.(type) {
		case *js_ast.SExpr:
			// Propagate the name to keep from the export into the value
			p.nameToKeep = "default"
			p.nameToKeepIsFor = s2.Value.Data

			s2.Value = p.visitExpr(s2.Value)

			// Discard type-only export default statements
			if p.options.ts.Parse {
				if id, ok := s2.Value.Data.(*js_ast.EIdentifier); ok {
					symbol := p.symbols[id.Ref.InnerIndex]
					if symbol.Kind == ast.SymbolUnbound && p.localTypeNames[symbol.OriginalName] {
						return stmts
					}
				}
			}

			// If there are lowered "using" declarations, change this into a "var"
			if p.currentScope.Parent == nil && p.willWrapModuleInTryCatchForUsing {
				stmts = append(stmts,
					js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SLocal{
						Decls: []js_ast.Decl{{
							Binding:    js_ast.Binding{Loc: s.DefaultName.Loc, Data: &js_ast.BIdentifier{Ref: s.DefaultName.Ref}},
							ValueOrNil: s2.Value,
						}},
					}},
					js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SExportClause{Items: []js_ast.ClauseItem{{
						Alias:    "default",
						AliasLoc: s.DefaultName.Loc,
						Name:     s.DefaultName,
					}}}},
				)
				break
			}

			stmts = append(stmts, stmt)

		case *js_ast.SFunction:
			// If we need to preserve the name but there is no name, generate a name
			var name string
			if p.options.keepNames {
				if s2.Fn.Name == nil {
					clone := s.DefaultName
					s2.Fn.Name = &clone
					name = "default"
				} else {
					name = p.symbols[s2.Fn.Name.Ref.InnerIndex].OriginalName
				}
			}

			p.visitFn(&s2.Fn, s2.Fn.OpenParenLoc, visitFnOpts{})
			stmts = append(stmts, stmt)

			// Optionally preserve the name
			if p.options.keepNames {
				p.symbols[s2.Fn.Name.Ref.InnerIndex].Flags |= ast.DidKeepName
				fn := js_ast.Expr{Loc: s2.Fn.Name.Loc, Data: &js_ast.EIdentifier{Ref: s2.Fn.Name.Ref}}
				stmts = append(stmts, p.keepClassOrFnSymbolName(s2.Fn.Name.Loc, fn, name))
			}

		case *js_ast.SClass:
			result := p.visitClass(s.Value.Loc, &s2.Class, s.DefaultName.Ref, "default")

			// Lower class field syntax for browsers that don't support it
			classStmts, _ := p.lowerClass(stmt, js_ast.Expr{}, result, "")

			// Remember if the class was side-effect free before lowering
			if result.canBeRemovedIfUnused {
				for _, classStmt := range classStmts {
					if s2, ok := classStmt.Data.(*js_ast.SExpr); ok {
						s2.IsFromClassOrFnThatCanBeRemovedIfUnused = true
					}
				}
			}

			stmts = append(stmts, classStmts...)

		default:
			panic("Internal error")
		}

		// Use a more friendly name than "default" now that "--keep-names" has
		// been applied and has made sure to enforce the name "default"
		if p.symbols[s.DefaultName.Ref.InnerIndex].OriginalName == "default" {
			p.symbols[s.DefaultName.Ref.InnerIndex].OriginalName = p.source.IdentifierName + "_default"
		}

		return stmts

	case *js_ast.SExportEquals:
		// "module.exports = value"
		stmts = append(stmts, js_ast.AssignStmt(
			js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EDot{
				Target:  js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: p.moduleRef}},
				Name:    "exports",
				NameLoc: stmt.Loc,
			}},
			p.visitExpr(s.Value),
		))
		p.recordUsage(p.moduleRef)
		return stmts

	case *js_ast.SBreak:
		if s.Label != nil {
			name := p.loadNameFromRef(s.Label.Ref)
			s.Label.Ref, _, _ = p.findLabelSymbol(s.Label.Loc, name)
		} else if !p.fnOrArrowDataVisit.isInsideLoop && !p.fnOrArrowDataVisit.isInsideSwitch {
			r := js_lexer.RangeOfIdentifier(p.source, stmt.Loc)
			p.log.AddError(&p.tracker, r, "Cannot use \"break\" here:")
		}

	case *js_ast.SContinue:
		if s.Label != nil {
			name := p.loadNameFromRef(s.Label.Ref)
			var isLoop, ok bool
			s.Label.Ref, isLoop, ok = p.findLabelSymbol(s.Label.Loc, name)
			if ok && !isLoop {
				r := js_lexer.RangeOfIdentifier(p.source, s.Label.Loc)
				p.log.AddError(&p.tracker, r, fmt.Sprintf("Cannot continue to label \"%s\"", name))
			}
		} else if !p.fnOrArrowDataVisit.isInsideLoop {
			r := js_lexer.RangeOfIdentifier(p.source, stmt.Loc)
			p.log.AddError(&p.tracker, r, "Cannot use \"continue\" here:")
		}

	case *js_ast.SLabel:
		// Forbid functions inside labels in strict mode
		if p.isStrictMode() {
			if _, ok := s.Stmt.Data.(*js_ast.SFunction); ok {
				p.markStrictModeFeature(labelFunctionStmt, js_lexer.RangeOfIdentifier(p.source, s.Stmt.Loc), "")
			}
		}

		p.pushScopeForVisitPass(js_ast.ScopeLabel, stmt.Loc)
		name := p.loadNameFromRef(s.Name.Ref)
		if js_lexer.StrictModeReservedWords[name] {
			p.markStrictModeFeature(reservedWord, js_lexer.RangeOfIdentifier(p.source, s.Name.Loc), name)
		}
		ref := p.newSymbol(ast.SymbolLabel, name)
		s.Name.Ref = ref

		// Duplicate labels are an error
		for scope := p.currentScope.Parent; scope != nil; scope = scope.Parent {
			if scope.Label.Ref != ast.InvalidRef && name == p.symbols[scope.Label.Ref.InnerIndex].OriginalName {
				p.log.AddErrorWithNotes(&p.tracker, js_lexer.RangeOfIdentifier(p.source, s.Name.Loc),
					fmt.Sprintf("Duplicate label %q", name),
					[]logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, scope.Label.Loc),
						fmt.Sprintf("The original label %q is here:", name))})
				break
			}
			if scope.Kind == js_ast.ScopeFunctionBody {
				// Labels are only visible within the function they are defined in.
				break
			}
		}

		p.currentScope.Label = ast.LocRef{Loc: s.Name.Loc, Ref: ref}
		switch s.Stmt.Data.(type) {
		case *js_ast.SFor, *js_ast.SForIn, *js_ast.SForOf, *js_ast.SWhile, *js_ast.SDoWhile:
			p.currentScope.LabelStmtIsLoop = true
		}

		// If we're dropping this statement, consider control flow to be dead
		_, shouldDropLabel := p.dropLabelsMap[name]
		old := p.isControlFlowDead
		if shouldDropLabel {
			p.isControlFlowDead = true
		}

		s.Stmt = p.visitSingleStmt(s.Stmt, stmtsNormal)
		p.popScope()

		// Drop this entire statement if requested
		if shouldDropLabel {
			p.isControlFlowDead = old
			return stmts
		}

		if p.options.minifySyntax {
			// Optimize "x: break x" which some people apparently write by hand
			if child, ok := s.Stmt.Data.(*js_ast.SBreak); ok && child.Label != nil && child.Label.Ref == s.Name.Ref {
				return stmts
			}

			// Remove the label if it's not necessary
			if p.symbols[ref.InnerIndex].UseCountEstimate == 0 {
				return appendIfOrLabelBodyPreservingScope(stmts, s.Stmt)
			}
		}

		// Handle "for await" that has been lowered by moving this label inside the "try"
		if try, ok := s.Stmt.Data.(*js_ast.STry); ok && len(try.Block.Stmts) == 1 {
			if _, ok := try.Block.Stmts[0].Data.(*js_ast.SFor); ok {
				try.Block.Stmts[0] = js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SLabel{
					Stmt:             try.Block.Stmts[0],
					Name:             s.Name,
					IsSingleLineStmt: s.IsSingleLineStmt,
				}}
				return append(stmts, s.Stmt)
			}
		}

	case *js_ast.SLocal:
		// Silently remove unsupported top-level "await" in dead code branches
		if s.Kind == js_ast.LocalAwaitUsing && p.fnOrArrowDataVisit.isOutsideFnOrArrow {
			if p.isControlFlowDead && (p.options.unsupportedJSFeatures.Has(compat.TopLevelAwait) || !p.options.outputFormat.KeepESMImportExportSyntax()) {
				s.Kind = js_ast.LocalUsing
			} else {
				p.liveTopLevelAwaitKeyword = logger.Range{Loc: stmt.Loc, Len: 5}
				p.markSyntaxFeature(compat.TopLevelAwait, logger.Range{Loc: stmt.Loc, Len: 5})
			}
		}

		// Local statements do not end the const local prefix
		p.currentScope.IsAfterConstLocalPrefix = wasAfterAfterConstLocalPrefix

		for i := range s.Decls {
			d := &s.Decls[i]
			p.visitBinding(d.Binding, bindingOpts{})

			// Visit the initializer
			if d.ValueOrNil.Data != nil {
				// Fold numeric constants in the initializer
				oldShouldFoldTypeScriptConstantExpressions := p.shouldFoldTypeScriptConstantExpressions
				p.shouldFoldTypeScriptConstantExpressions = p.options.minifySyntax && !p.currentScope.IsAfterConstLocalPrefix

				// Propagate the name to keep from the binding into the initializer
				if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
					p.nameToKeep = p.symbols[id.Ref.InnerIndex].OriginalName
					p.nameToKeepIsFor = d.ValueOrNil.Data
				}

				d.ValueOrNil = p.visitExpr(d.ValueOrNil)

				p.shouldFoldTypeScriptConstantExpressions = oldShouldFoldTypeScriptConstantExpressions

				// Initializing to undefined is implicit, but be careful to not
				// accidentally cause a syntax error or behavior change by removing
				// the value
				//
				// Good:
				//   "let a = undefined;" => "let a;"
				//
				// Bad (a syntax error):
				//   "let {} = undefined;" => "let {};"
				//
				// Bad (a behavior change):
				//   "a = 123; var a = undefined;" => "a = 123; var a;"
				//
				if p.options.minifySyntax && s.Kind == js_ast.LocalLet {
					if _, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
						if _, ok := d.ValueOrNil.Data.(*js_ast.EUndefined); ok {
							d.ValueOrNil = js_ast.Expr{}
						}
					}
				}

				// Yarn's PnP data may be stored in a variable: https://github.com/yarnpkg/berry/pull/4320
				if p.options.decodeHydrateRuntimeStateYarnPnP {
					if str, ok := d.ValueOrNil.Data.(*js_ast.EString); ok {
						if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
							if p.stringLocalsForYarnPnP == nil {
								p.stringLocalsForYarnPnP = make(map[ast.Ref]stringLocalForYarnPnP)
							}
							p.stringLocalsForYarnPnP[id.Ref] = stringLocalForYarnPnP{value: str.Value, loc: d.ValueOrNil.Loc}
						}
					}
				}
			}

			// Attempt to continue the const local prefix
			if p.options.minifySyntax && !p.currentScope.IsAfterConstLocalPrefix {
				if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
					if s.Kind == js_ast.LocalConst && d.ValueOrNil.Data != nil {
						if value := js_ast.ExprToConstValue(d.ValueOrNil); value.Kind != js_ast.ConstValueNone {
							if p.constValues == nil {
								p.constValues = make(map[ast.Ref]js_ast.ConstValue)
							}
							p.constValues[id.Ref] = value
							continue
						}
					}

					if d.ValueOrNil.Data != nil && !isSafeForConstLocalPrefix(d.ValueOrNil) {
						p.currentScope.IsAfterConstLocalPrefix = true
					}
				} else {
					// A non-identifier binding ends the const local prefix
					p.currentScope.IsAfterConstLocalPrefix = true
				}
			}
		}

		// Handle being exported inside a namespace
		if s.IsExport && p.enclosingNamespaceArgRef != nil {
			wrapIdentifier := func(loc logger.Loc, ref ast.Ref) js_ast.Expr {
				p.recordUsage(*p.enclosingNamespaceArgRef)
				return js_ast.Expr{Loc: loc, Data: p.dotOrMangledPropVisit(
					js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					p.symbols[ref.InnerIndex].OriginalName,
					loc,
				)}
			}
			for _, decl := range s.Decls {
				if decl.ValueOrNil.Data != nil {
					target := js_ast.ConvertBindingToExpr(decl.Binding, wrapIdentifier)
					if result, ok := p.lowerAssign(target, decl.ValueOrNil, objRestReturnValueIsUnused); ok {
						target = result
					} else {
						target = js_ast.Assign(target, decl.ValueOrNil)
					}
					stmts = append(stmts, js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SExpr{Value: target}})
				}
			}
			return stmts
		}

		s.Decls = p.lowerObjectRestInDecls(s.Decls)

		// Optimization: Avoid unnecessary "using" machinery by changing ones
		// initialized to "null" or "undefined" into a normal variable. Note that
		// "await using" still needs the "await", so we can't do it for those.
		if p.options.minifySyntax && s.Kind == js_ast.LocalUsing {
			s.Kind = js_ast.LocalConst
			for _, decl := range s.Decls {
				if t := js_ast.KnownPrimitiveType(decl.ValueOrNil.Data); t != js_ast.PrimitiveNull && t != js_ast.PrimitiveUndefined {
					s.Kind = js_ast.LocalUsing
					break
				}
			}
		}

		s.Kind = p.selectLocalKind(s.Kind)

		// Potentially relocate "var" declarations to the top level
		if s.Kind == js_ast.LocalVar {
			if assign, ok := p.maybeRelocateVarsToTopLevel(s.Decls, relocateVarsNormal); ok {
				if assign.Data != nil {
					stmts = append(stmts, assign)
				}
				return stmts
			}
		}

	case *js_ast.SExpr:
		shouldTrimUnsightlyPrimitives := !p.options.minifySyntax && !isUnsightlyPrimitive(s.Value.Data)
		p.stmtExprValue = s.Value.Data
		s.Value = p.visitExpr(s.Value)

		// Expressions that have been simplified down to a single primitive don't
		// have any effect, and are automatically removed during minification.
		// However, some people are really bothered by seeing them. Remove them
		// so we don't bother these people.
		if shouldTrimUnsightlyPrimitives && isUnsightlyPrimitive(s.Value.Data) {
			return stmts
		}

		// Trim expressions without side effects
		if p.options.minifySyntax {
			s.Value = p.astHelpers.SimplifyUnusedExpr(s.Value, p.options.unsupportedJSFeatures)
			if s.Value.Data == nil {
				return stmts
			}
		}

	case *js_ast.SThrow:
		s.Value = p.visitExpr(s.Value)

	case *js_ast.SReturn:
		// Forbid top-level return inside modules with ECMAScript syntax
		if p.fnOrArrowDataVisit.isOutsideFnOrArrow {
			if p.isFileConsideredESM {
				_, notes := p.whyESModule()
				p.log.AddErrorWithNotes(&p.tracker, js_lexer.RangeOfIdentifier(p.source, stmt.Loc),
					"Top-level return cannot be used inside an ECMAScript module", notes)
			} else {
				p.hasTopLevelReturn = true
			}
		}

		if s.ValueOrNil.Data != nil {
			s.ValueOrNil = p.visitExpr(s.ValueOrNil)

			// Returning undefined is implicit except when inside an async generator
			// function, where "return undefined" behaves like "return await undefined"
			// but just "return" has no "await".
			if p.options.minifySyntax && (!p.fnOrArrowDataVisit.isAsync || !p.fnOrArrowDataVisit.isGenerator) {
				if _, ok := s.ValueOrNil.Data.(*js_ast.EUndefined); ok {
					s.ValueOrNil = js_ast.Expr{}
				}
			}
		}

	case *js_ast.SBlock:
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)

		// Pass the "is loop body" status on to the direct children of a block used
		// as a loop body. This is used to enable optimizations specific to the
		// topmost scope in a loop body block.
		if p.loopBody == s {
			s.Stmts = p.visitStmts(s.Stmts, stmtsLoopBody)
		} else {
			s.Stmts = p.visitStmts(s.Stmts, stmtsNormal)
		}

		p.popScope()

		if p.options.minifySyntax {
			if len(s.Stmts) == 1 && !statementCaresAboutScope(s.Stmts[0]) {
				// Unwrap blocks containing a single statement
				stmt = s.Stmts[0]
			} else if len(s.Stmts) == 0 {
				// Trim empty blocks
				stmt = js_ast.Stmt{Loc: stmt.Loc, Data: js_ast.SEmptyShared}
			}
		}

	case *js_ast.SWith:
		p.markStrictModeFeature(withStatement, js_lexer.RangeOfIdentifier(p.source, stmt.Loc), "")
		s.Value = p.visitExpr(s.Value)
		p.pushScopeForVisitPass(js_ast.ScopeWith, s.BodyLoc)
		s.Body = p.visitSingleStmt(s.Body, stmtsNormal)
		p.popScope()

	case *js_ast.SWhile:
		s.Test = p.visitExpr(s.Test)
		s.Body = p.visitLoopBody(s.Body)

		if p.options.minifySyntax {
			s.Test = p.astHelpers.SimplifyBooleanExpr(s.Test)

			// A true value is implied
			testOrNil := s.Test
			if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(s.Test.Data); ok && boolean && sideEffects == js_ast.NoSideEffects {
				testOrNil = js_ast.Expr{}
			}

			// "while (a) {}" => "for (;a;) {}"
			forS := &js_ast.SFor{TestOrNil: testOrNil, Body: s.Body, IsSingleLineBody: s.IsSingleLineBody}
			mangleFor(forS)
			stmt = js_ast.Stmt{Loc: stmt.Loc, Data: forS}
		}

	case *js_ast.SDoWhile:
		s.Body = p.visitLoopBody(s.Body)
		s.Test = p.visitExpr(s.Test)

		if p.options.minifySyntax {
			s.Test = p.astHelpers.SimplifyBooleanExpr(s.Test)
		}

	case *js_ast.SIf:
		s.Test = p.visitExpr(s.Test)

		if p.options.minifySyntax {
			s.Test = p.astHelpers.SimplifyBooleanExpr(s.Test)
		}

		// Fold constants
		boolean, _, ok := js_ast.ToBooleanWithSideEffects(s.Test.Data)

		// Mark the control flow as dead if the branch is never taken
		if ok && !boolean {
			old := p.isControlFlowDead
			p.isControlFlowDead = true
			s.Yes = p.visitSingleStmt(s.Yes, stmtsNormal)
			p.isControlFlowDead = old
		} else {
			s.Yes = p.visitSingleStmt(s.Yes, stmtsNormal)
		}

		// The "else" clause is optional
		if s.NoOrNil.Data != nil {
			// Mark the control flow as dead if the branch is never taken
			if ok && boolean {
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				s.NoOrNil = p.visitSingleStmt(s.NoOrNil, stmtsNormal)
				p.isControlFlowDead = old
			} else {
				s.NoOrNil = p.visitSingleStmt(s.NoOrNil, stmtsNormal)
			}

			// Trim unnecessary "else" clauses
			if p.options.minifySyntax {
				if _, ok := s.NoOrNil.Data.(*js_ast.SEmpty); ok {
					s.NoOrNil = js_ast.Stmt{}
				}
			}
		}

		if p.options.minifySyntax {
			return p.mangleIf(stmts, stmt.Loc, s)
		}

	case *js_ast.SFor:
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		if s.InitOrNil.Data != nil {
			p.visitForLoopInit(s.InitOrNil, false)
		}

		if s.TestOrNil.Data != nil {
			s.TestOrNil = p.visitExpr(s.TestOrNil)

			if p.options.minifySyntax {
				s.TestOrNil = p.astHelpers.SimplifyBooleanExpr(s.TestOrNil)

				// A true value is implied
				if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(s.TestOrNil.Data); ok && boolean && sideEffects == js_ast.NoSideEffects {
					s.TestOrNil = js_ast.Expr{}
				}
			}
		}

		if s.UpdateOrNil.Data != nil {
			s.UpdateOrNil = p.visitExpr(s.UpdateOrNil)
		}
		s.Body = p.visitLoopBody(s.Body)

		// Potentially relocate "var" declarations to the top level. Note that this
		// must be done inside the scope of the for loop or they won't be relocated.
		if s.InitOrNil.Data != nil {
			if init, ok := s.InitOrNil.Data.(*js_ast.SLocal); ok && init.Kind == js_ast.LocalVar {
				if assign, ok := p.maybeRelocateVarsToTopLevel(init.Decls, relocateVarsNormal); ok {
					if assign.Data != nil {
						s.InitOrNil = assign
					} else {
						s.InitOrNil = js_ast.Stmt{}
					}
				}
			}
		}

		p.popScope()

		if p.options.minifySyntax {
			mangleFor(s)
		}

	case *js_ast.SForIn:
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		p.visitForLoopInit(s.Init, true)
		s.Value = p.visitExpr(s.Value)
		s.Body = p.visitLoopBody(s.Body)

		// Check for a variable initializer
		if local, ok := s.Init.Data.(*js_ast.SLocal); ok && local.Kind == js_ast.LocalVar && len(local.Decls) == 1 {
			decl := &local.Decls[0]
			if id, ok := decl.Binding.Data.(*js_ast.BIdentifier); ok && decl.ValueOrNil.Data != nil {
				p.markStrictModeFeature(forInVarInit, p.source.RangeOfOperatorBefore(decl.ValueOrNil.Loc, "="), "")

				// Lower for-in variable initializers in case the output is used in strict mode
				stmts = append(stmts, js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SExpr{Value: js_ast.Assign(
					js_ast.Expr{Loc: decl.Binding.Loc, Data: &js_ast.EIdentifier{Ref: id.Ref}},
					decl.ValueOrNil,
				)}})
				decl.ValueOrNil = js_ast.Expr{}
			}
		}

		// Potentially relocate "var" declarations to the top level. Note that this
		// must be done inside the scope of the for loop or they won't be relocated.
		if init, ok := s.Init.Data.(*js_ast.SLocal); ok && init.Kind == js_ast.LocalVar {
			if replacement, ok := p.maybeRelocateVarsToTopLevel(init.Decls, relocateVarsForInOrForOf); ok {
				s.Init = replacement
			}
		}

		p.popScope()

		p.lowerObjectRestInForLoopInit(s.Init, &s.Body)

	case *js_ast.SForOf:
		// Silently remove unsupported top-level "await" in dead code branches
		if s.Await.Len > 0 && p.fnOrArrowDataVisit.isOutsideFnOrArrow {
			if p.isControlFlowDead && (p.options.unsupportedJSFeatures.Has(compat.TopLevelAwait) || !p.options.outputFormat.KeepESMImportExportSyntax()) {
				s.Await = logger.Range{}
			} else {
				p.liveTopLevelAwaitKeyword = s.Await
				p.markSyntaxFeature(compat.TopLevelAwait, s.Await)
			}
		}

		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		p.visitForLoopInit(s.Init, true)
		s.Value = p.visitExpr(s.Value)
		s.Body = p.visitLoopBody(s.Body)

		// Potentially relocate "var" declarations to the top level. Note that this
		// must be done inside the scope of the for loop or they won't be relocated.
		if init, ok := s.Init.Data.(*js_ast.SLocal); ok && init.Kind == js_ast.LocalVar {
			if replacement, ok := p.maybeRelocateVarsToTopLevel(init.Decls, relocateVarsForInOrForOf); ok {
				s.Init = replacement
			}
		}

		// Handle "for (using x of y)" and "for (await using x of y)"
		if local, ok := s.Init.Data.(*js_ast.SLocal); ok {
			if local.Kind == js_ast.LocalUsing && p.options.unsupportedJSFeatures.Has(compat.Using) {
				p.lowerUsingDeclarationInForOf(s.Init.Loc, local, &s.Body)
			} else if local.Kind == js_ast.LocalAwaitUsing {
				if p.fnOrArrowDataVisit.isOutsideFnOrArrow {
					if p.isControlFlowDead && (p.options.unsupportedJSFeatures.Has(compat.TopLevelAwait) || !p.options.outputFormat.KeepESMImportExportSyntax()) {
						// Silently remove unsupported top-level "await" in dead code branches
						local.Kind = js_ast.LocalUsing
					} else {
						p.liveTopLevelAwaitKeyword = logger.Range{Loc: s.Init.Loc, Len: 5}
						p.markSyntaxFeature(compat.TopLevelAwait, p.liveTopLevelAwaitKeyword)
					}
					if p.options.unsupportedJSFeatures.Has(compat.Using) {
						p.lowerUsingDeclarationInForOf(s.Init.Loc, local, &s.Body)
					}
				} else if p.options.unsupportedJSFeatures.Has(compat.Using) || p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) ||
					(p.options.unsupportedJSFeatures.Has(compat.AsyncGenerator) && p.fnOrArrowDataVisit.isGenerator) {
					p.lowerUsingDeclarationInForOf(s.Init.Loc, local, &s.Body)
				}
			}
		}

		p.popScope()

		p.lowerObjectRestInForLoopInit(s.Init, &s.Body)

		// Lower "for await" if it's unsupported if it's in a lowered async generator
		if s.Await.Len > 0 && (p.options.unsupportedJSFeatures.Has(compat.ForAwait) ||
			(p.options.unsupportedJSFeatures.Has(compat.AsyncGenerator) && p.fnOrArrowDataVisit.isGenerator)) {
			return p.lowerForAwaitLoop(stmt.Loc, s, stmts)
		}

	case *js_ast.STry:
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		if p.fnOrArrowDataVisit.tryBodyCount == 0 {
			if s.Catch != nil {
				p.fnOrArrowDataVisit.tryCatchLoc = s.Catch.Loc
			} else {
				p.fnOrArrowDataVisit.tryCatchLoc = stmt.Loc
			}
		}
		p.fnOrArrowDataVisit.tryBodyCount++
		s.Block.Stmts = p.visitStmts(s.Block.Stmts, stmtsNormal)
		p.fnOrArrowDataVisit.tryBodyCount--
		p.popScope()

		if s.Catch != nil {
			old := p.isControlFlowDead

			// If the try body is empty, then the catch body is dead
			if len(s.Block.Stmts) == 0 {
				p.isControlFlowDead = true
			}

			p.pushScopeForVisitPass(js_ast.ScopeCatchBinding, s.Catch.Loc)
			if s.Catch.BindingOrNil.Data != nil {
				p.visitBinding(s.Catch.BindingOrNil, bindingOpts{})
			}

			p.pushScopeForVisitPass(js_ast.ScopeBlock, s.Catch.BlockLoc)
			s.Catch.Block.Stmts = p.visitStmts(s.Catch.Block.Stmts, stmtsNormal)
			p.popScope()

			p.lowerObjectRestInCatchBinding(s.Catch)
			p.popScope()

			p.isControlFlowDead = old
		}

		if s.Finally != nil {
			p.pushScopeForVisitPass(js_ast.ScopeBlock, s.Finally.Loc)
			s.Finally.Block.Stmts = p.visitStmts(s.Finally.Block.Stmts, stmtsNormal)
			p.popScope()
		}

		// Drop the whole thing if the try body is empty
		if p.options.minifySyntax && len(s.Block.Stmts) == 0 {
			keepCatch := false

			// Certain "catch" blocks need to be preserved:
			//
			//   try {} catch { let foo } // Can be removed
			//   try {} catch { var foo } // Must be kept
			//
			if s.Catch != nil {
				for _, stmt2 := range s.Catch.Block.Stmts {
					if shouldKeepStmtInDeadControlFlow(stmt2) {
						keepCatch = true
						break
					}
				}
			}

			// Make sure to preserve the "finally" block if present
			if !keepCatch {
				if s.Finally == nil {
					return stmts
				}
				finallyNeedsBlock := false
				for _, stmt2 := range s.Finally.Block.Stmts {
					if statementCaresAboutScope(stmt2) {
						finallyNeedsBlock = true
						break
					}
				}
				if !finallyNeedsBlock {
					return append(stmts, s.Finally.Block.Stmts...)
				}
				block := s.Finally.Block
				stmt = js_ast.Stmt{Loc: s.Finally.Loc, Data: &block}
			}
		}

	case *js_ast.SSwitch:
		s.Test = p.visitExpr(s.Test)
		p.pushScopeForVisitPass(js_ast.ScopeBlock, s.BodyLoc)
		oldIsInsideSwitch := p.fnOrArrowDataVisit.isInsideSwitch
		p.fnOrArrowDataVisit.isInsideSwitch = true

		// Visit case values first
		for i := range s.Cases {
			c := &s.Cases[i]
			if c.ValueOrNil.Data != nil {
				c.ValueOrNil = p.visitExpr(c.ValueOrNil)
				p.warnAboutEqualityCheck("case", c.ValueOrNil, c.ValueOrNil.Loc)
				p.warnAboutTypeofAndString(s.Test, c.ValueOrNil, onlyCheckOriginalOrder)
			}
		}

		// Check for duplicate case values
		p.duplicateCaseChecker.reset()
		for _, c := range s.Cases {
			if c.ValueOrNil.Data != nil {
				p.duplicateCaseChecker.check(p, c.ValueOrNil)
			}
		}

		// Then analyze the cases to determine which ones are live and/or dead
		cases := analyzeSwitchCasesForLiveness(s)

		// Then visit case bodies, and potentially filter out dead cases
		end := 0
		for i, c := range s.Cases {
			isAlwaysDead := cases[i].status == alwaysDead

			// Potentially treat the case body as dead code
			old := p.isControlFlowDead
			if isAlwaysDead {
				p.isControlFlowDead = true
			}
			c.Body = p.visitStmts(c.Body, stmtsSwitch)
			p.isControlFlowDead = old

			// Filter out this case when minifying if it's known to be dead. Visiting
			// the body above should already have removed any statements that can be
			// removed safely, so if the body isn't empty then that means it contains
			// some statements that can't be removed safely (e.g. a hoisted "var").
			// So don't remove this case if the body isn't empty.
			if p.options.minifySyntax && isAlwaysDead && len(c.Body) == 0 {
				continue
			}

			// Make sure the assignment to the body above is preserved
			s.Cases[end] = c
			end++
		}
		s.Cases = s.Cases[:end]

		p.fnOrArrowDataVisit.isInsideSwitch = oldIsInsideSwitch
		p.popScope()

		// Unwrap switch statements in dead code
		if p.options.minifySyntax && p.isControlFlowDead {
			for _, c := range s.Cases {
				stmts = append(stmts, c.Body...)
			}
			return stmts
		}

		// "using" declarations inside switch statements must be special-cased
		if lowered := p.maybeLowerUsingDeclarationsInSwitch(stmt.Loc, s); lowered != nil {
			return append(stmts, lowered...)
		}

		// Attempt to remove statically-determined switch statements
		if p.options.minifySyntax {
			if len(s.Cases) == 0 {
				if p.astHelpers.ExprCanBeRemovedIfUnused(s.Test) {
					// Remove everything
					return stmts
				} else {
					// Just keep the test expression
					return append(stmts, js_ast.Stmt{Loc: s.Test.Loc, Data: &js_ast.SExpr{Value: s.Test}})
				}
			} else if len(s.Cases) == 1 {
				c := s.Cases[0]
				var isTaken bool
				var ok bool
				if c.ValueOrNil.Data != nil {
					// Non-default case
					isTaken, ok = js_ast.CheckEqualityIfNoSideEffects(s.Test.Data, c.ValueOrNil.Data, js_ast.StrictEquality)
				} else {
					// Default case
					isTaken, ok = true, p.astHelpers.ExprCanBeRemovedIfUnused(s.Test)
				}
				if ok && isTaken {
					if body, ok := tryToInlineCaseBody(s.BodyLoc, c.Body, s.CloseBraceLoc); ok {
						// Inline the case body
						return append(stmts, body...)
					}
				}
			}
		}

	case *js_ast.SFunction:
		p.visitFn(&s.Fn, s.Fn.OpenParenLoc, visitFnOpts{})

		// Strip this function declaration if it was overwritten
		if p.symbols[s.Fn.Name.Ref.InnerIndex].Flags.Has(ast.RemoveOverwrittenFunctionDeclaration) && !s.IsExport {
			return stmts
		}

		if p.options.minifySyntax && !s.Fn.IsGenerator && !s.Fn.IsAsync && !s.Fn.HasRestArg && s.Fn.Name != nil {
			if len(s.Fn.Body.Block.Stmts) == 0 {
				// Mark if this function is an empty function
				hasSideEffectFreeArguments := true
				for _, arg := range s.Fn.Args {
					if _, ok := arg.Binding.Data.(*js_ast.BIdentifier); !ok {
						hasSideEffectFreeArguments = false
						break
					}
				}
				if hasSideEffectFreeArguments {
					p.symbols[s.Fn.Name.Ref.InnerIndex].Flags |= ast.IsEmptyFunction
				}
			} else if len(s.Fn.Args) == 1 && len(s.Fn.Body.Block.Stmts) == 1 {
				// Mark if this function is an identity function
				if arg := s.Fn.Args[0]; arg.DefaultOrNil.Data == nil {
					if id, ok := arg.Binding.Data.(*js_ast.BIdentifier); ok {
						if ret, ok := s.Fn.Body.Block.Stmts[0].Data.(*js_ast.SReturn); ok {
							if retID, ok := ret.ValueOrNil.Data.(*js_ast.EIdentifier); ok && id.Ref == retID.Ref {
								p.symbols[s.Fn.Name.Ref.InnerIndex].Flags |= ast.IsIdentityFunction
							}
						}
					}
				}
			}
		}

		// Handle exporting this function from a namespace
		if s.IsExport && p.enclosingNamespaceArgRef != nil {
			s.IsExport = false
			stmts = append(stmts, stmt, js_ast.AssignStmt(
				js_ast.Expr{Loc: stmt.Loc, Data: p.dotOrMangledPropVisit(
					js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					p.symbols[s.Fn.Name.Ref.InnerIndex].OriginalName,
					s.Fn.Name.Loc,
				)},
				js_ast.Expr{Loc: s.Fn.Name.Loc, Data: &js_ast.EIdentifier{Ref: s.Fn.Name.Ref}},
			))
		} else {
			stmts = append(stmts, stmt)
		}

		// Optionally preserve the name
		if p.options.keepNames {
			symbol := &p.symbols[s.Fn.Name.Ref.InnerIndex]
			symbol.Flags |= ast.DidKeepName
			fn := js_ast.Expr{Loc: s.Fn.Name.Loc, Data: &js_ast.EIdentifier{Ref: s.Fn.Name.Ref}}
			stmts = append(stmts, p.keepClassOrFnSymbolName(s.Fn.Name.Loc, fn, symbol.OriginalName))
		}
		return stmts

	case *js_ast.SClass:
		result := p.visitClass(stmt.Loc, &s.Class, ast.InvalidRef, "")

		// Remove the export flag inside a namespace
		var nameToExport string
		wasExportInsideNamespace := s.IsExport && p.enclosingNamespaceArgRef != nil
		if wasExportInsideNamespace {
			nameToExport = p.symbols[s.Class.Name.Ref.InnerIndex].OriginalName
			s.IsExport = false
		}

		// Lower class field syntax for browsers that don't support it
		classStmts, _ := p.lowerClass(stmt, js_ast.Expr{}, result, "")

		// Remember if the class was side-effect free before lowering
		if result.canBeRemovedIfUnused {
			for _, classStmt := range classStmts {
				if s2, ok := classStmt.Data.(*js_ast.SExpr); ok {
					s2.IsFromClassOrFnThatCanBeRemovedIfUnused = true
				}
			}
		}

		stmts = append(stmts, classStmts...)

		// Handle exporting this class from a namespace
		if wasExportInsideNamespace {
			stmts = append(stmts, js_ast.AssignStmt(
				js_ast.Expr{Loc: stmt.Loc, Data: p.dotOrMangledPropVisit(
					js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					nameToExport,
					s.Class.Name.Loc,
				)},
				js_ast.Expr{Loc: s.Class.Name.Loc, Data: &js_ast.EIdentifier{Ref: s.Class.Name.Ref}},
			))
		}

		return stmts

	case *js_ast.SEnum:
		// Do not end the const local prefix after TypeScript enums. We process
		// them first within their scope so that they are inlined into all code in
		// that scope. We don't want that to cause the const local prefix to end.
		p.currentScope.IsAfterConstLocalPrefix = wasAfterAfterConstLocalPrefix

		// Track cross-module enum constants during bundling
		var tsTopLevelEnumValues map[string]js_ast.TSEnumValue
		if p.currentScope == p.moduleScope && p.options.mode == config.ModeBundle {
			tsTopLevelEnumValues = make(map[string]js_ast.TSEnumValue)
		}

		p.recordDeclaredSymbol(s.Name.Ref)
		p.pushScopeForVisitPass(js_ast.ScopeEntry, stmt.Loc)
		p.recordDeclaredSymbol(s.Arg)

		// Scan ahead for any variables inside this namespace. This must be done
		// ahead of time before visiting any statements inside the namespace
		// because we may end up visiting the uses before the declarations.
		// We need to convert the uses into property accesses on the namespace.
		for _, value := range s.Values {
			if value.Ref != ast.InvalidRef {
				p.isExportedInsideNamespace[value.Ref] = s.Arg
			}
		}

		// Values without initializers are initialized to one more than the
		// previous value if the previous value is numeric. Otherwise values
		// without initializers are initialized to undefined.
		nextNumericValue := float64(0)
		hasNumericValue := true
		valueExprs := []js_ast.Expr{}
		allValuesArePure := true

		// Update the exported members of this enum as we constant fold each one
		exportedMembers := p.currentScope.TSNamespace.ExportedMembers

		// We normally don't fold numeric constants because they might increase code
		// size, but it's important to fold numeric constants inside enums since
		// that's what the TypeScript compiler does.
		oldShouldFoldTypeScriptConstantExpressions := p.shouldFoldTypeScriptConstantExpressions
		p.shouldFoldTypeScriptConstantExpressions = true

		// Create an assignment for each enum value
		for _, value := range s.Values {
			name := helpers.UTF16ToString(value.Name)
			var assignTarget js_ast.Expr
			hasStringValue := false

			if value.ValueOrNil.Data != nil {
				value.ValueOrNil = p.visitExpr(value.ValueOrNil)
				hasNumericValue = false

				// "See through" any wrapped comments
				underlyingValue := value.ValueOrNil
				if inlined, ok := value.ValueOrNil.Data.(*js_ast.EInlinedEnum); ok {
					underlyingValue = inlined.Value
				}

				switch e := underlyingValue.Data.(type) {
				case *js_ast.ENumber:
					if tsTopLevelEnumValues != nil {
						tsTopLevelEnumValues[name] = js_ast.TSEnumValue{Number: e.Value}
					}
					member := exportedMembers[name]
					member.Data = &js_ast.TSNamespaceMemberEnumNumber{Value: e.Value}
					exportedMembers[name] = member
					p.refToTSNamespaceMemberData[value.Ref] = member.Data
					hasNumericValue = true
					nextNumericValue = e.Value + 1

				case *js_ast.EString:
					if tsTopLevelEnumValues != nil {
						tsTopLevelEnumValues[name] = js_ast.TSEnumValue{String: e.Value}
					}
					member := exportedMembers[name]
					member.Data = &js_ast.TSNamespaceMemberEnumString{Value: e.Value}
					exportedMembers[name] = member
					p.refToTSNamespaceMemberData[value.Ref] = member.Data
					hasStringValue = true

				default:
					if js_ast.KnownPrimitiveType(underlyingValue.Data) == js_ast.PrimitiveString {
						hasStringValue = true
					}
					if !p.astHelpers.ExprCanBeRemovedIfUnused(underlyingValue) {
						allValuesArePure = false
					}
				}
			} else if hasNumericValue {
				if tsTopLevelEnumValues != nil {
					tsTopLevelEnumValues[name] = js_ast.TSEnumValue{Number: nextNumericValue}
				}
				member := exportedMembers[name]
				member.Data = &js_ast.TSNamespaceMemberEnumNumber{Value: nextNumericValue}
				exportedMembers[name] = member
				p.refToTSNamespaceMemberData[value.Ref] = member.Data
				value.ValueOrNil = js_ast.Expr{Loc: value.Loc, Data: &js_ast.ENumber{Value: nextNumericValue}}
				nextNumericValue++
			} else {
				value.ValueOrNil = js_ast.Expr{Loc: value.Loc, Data: js_ast.EUndefinedShared}
			}

			if p.options.minifySyntax && js_ast.IsIdentifier(name) {
				// "Enum.Name = value"
				assignTarget = js_ast.Assign(
					js_ast.Expr{Loc: value.Loc, Data: &js_ast.EDot{
						Target:  js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIdentifier{Ref: s.Arg}},
						Name:    name,
						NameLoc: value.Loc,
					}},
					value.ValueOrNil,
				)
			} else {
				// "Enum['Name'] = value"
				assignTarget = js_ast.Assign(
					js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIndex{
						Target: js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIdentifier{Ref: s.Arg}},
						Index:  js_ast.Expr{Loc: value.Loc, Data: &js_ast.EString{Value: value.Name}},
					}},
					value.ValueOrNil,
				)
			}
			p.recordUsage(s.Arg)

			// String-valued enums do not form a two-way map
			if hasStringValue {
				valueExprs = append(valueExprs, assignTarget)
			} else {
				// "Enum[assignTarget] = 'Name'"
				valueExprs = append(valueExprs, js_ast.Assign(
					js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIndex{
						Target: js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIdentifier{Ref: s.Arg}},
						Index:  assignTarget,
					}},
					js_ast.Expr{Loc: value.Loc, Data: &js_ast.EString{Value: value.Name}},
				))
				p.recordUsage(s.Arg)
			}
		}

		p.popScope()
		p.shouldFoldTypeScriptConstantExpressions = oldShouldFoldTypeScriptConstantExpressions

		// Track all exported top-level enums for cross-module inlining
		if tsTopLevelEnumValues != nil {
			if p.tsEnums == nil {
				p.tsEnums = make(map[ast.Ref]map[string]js_ast.TSEnumValue)
			}
			p.tsEnums[s.Name.Ref] = tsTopLevelEnumValues
		}

		// Wrap this enum definition in a closure
		stmts = p.generateClosureForTypeScriptEnum(
			stmts, stmt.Loc, s.IsExport, s.Name.Loc, s.Name.Ref, s.Arg, valueExprs, allValuesArePure)
		return stmts

	case *js_ast.SNamespace:
		p.recordDeclaredSymbol(s.Name.Ref)

		// Scan ahead for any variables inside this namespace. This must be done
		// ahead of time before visiting any statements inside the namespace
		// because we may end up visiting the uses before the declarations.
		// We need to convert the uses into property accesses on the namespace.
		for _, childStmt := range s.Stmts {
			if local, ok := childStmt.Data.(*js_ast.SLocal); ok {
				if local.IsExport {
					js_ast.ForEachIdentifierBindingInDecls(local.Decls, func(loc logger.Loc, b *js_ast.BIdentifier) {
						p.isExportedInsideNamespace[b.Ref] = s.Arg
					})
				}
			}
		}

		oldEnclosingNamespaceArgRef := p.enclosingNamespaceArgRef
		p.enclosingNamespaceArgRef = &s.Arg
		p.pushScopeForVisitPass(js_ast.ScopeEntry, stmt.Loc)
		p.recordDeclaredSymbol(s.Arg)
		stmtsInsideNamespace := p.visitStmtsAndPrependTempRefs(s.Stmts, prependTempRefsOpts{kind: stmtsFnBody})
		p.popScope()
		p.enclosingNamespaceArgRef = oldEnclosingNamespaceArgRef

		// Generate a closure for this namespace
		stmts = p.generateClosureForTypeScriptNamespaceOrEnum(
			stmts, stmt.Loc, s.IsExport, s.Name.Loc, s.Name.Ref, s.Arg, stmtsInsideNamespace)
		return stmts

	default:
		panic("Internal error")
	}

	stmts = append(stmts, stmt)
	return stmts
}

func tryToInlineCaseBody(openBraceLoc logger.Loc, stmts []js_ast.Stmt, closeBraceLoc logger.Loc) ([]js_ast.Stmt, bool) {
	if len(stmts) == 1 {
		if block, ok := stmts[0].Data.(*js_ast.SBlock); ok {
			return tryToInlineCaseBody(stmts[0].Loc, block.Stmts, block.CloseBraceLoc)
		}
	}

	caresAboutScope := false

loop:
	for i, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SEmpty, *js_ast.SDirective, *js_ast.SComment, *js_ast.SExpr,
			*js_ast.SDebugger, *js_ast.SContinue, *js_ast.SReturn, *js_ast.SThrow:
			// These can all be inlined outside of the switch without problems
			continue

		case *js_ast.SLocal:
			if s.Kind != js_ast.LocalVar {
				caresAboutScope = true
			}

		case *js_ast.SBreak:
			if s.Label != nil {
				// The break label could target this switch, but we don't know whether that's the case or not here
				return nil, false
			}

			// An unlabeled "break" inside a switch breaks out of the case
			stmts = stmts[:i]
			break loop

		default:
			// Assume anything else can't be inlined
			return nil, false
		}
	}

	// If we still need a scope, wrap the result in a block
	if caresAboutScope {
		return []js_ast.Stmt{{Loc: openBraceLoc, Data: &js_ast.SBlock{Stmts: stmts, CloseBraceLoc: closeBraceLoc}}}, true
	}
	return stmts, true
}

func isUnsightlyPrimitive(data js_ast.E) bool {
	switch data.(type) {
	case *js_ast.EBoolean, *js_ast.ENull, *js_ast.EUndefined, *js_ast.ENumber, *js_ast.EBigInt, *js_ast.EString:
		return true
	}
	return false
}

// If we encounter a variable initializer that could possibly trigger access to
// a constant declared later on, then we need to end the const local prefix.
// We want to avoid situations like this:
//
//	const x = y; // This is supposed to throw due to TDZ
//	const y = 1;
//
// or this:
//
//	const x = 1;
//	const y = foo(); // This is supposed to throw due to TDZ
//	const z = 2;
//	const foo = () => z;
//
// But a situation like this is ok:
//
//	const x = 1;
//	const y = [() => x + z];
//	const z = 2;
func isSafeForConstLocalPrefix(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EMissing, *js_ast.EString, *js_ast.ERegExp, *js_ast.EBigInt, *js_ast.EFunction, *js_ast.EArrow:
		return true

	case *js_ast.EArray:
		for _, item := range e.Items {
			if !isSafeForConstLocalPrefix(item) {
				return false
			}
		}
		return true

	case *js_ast.EObject:
		// For now just allow "{}" and forbid everything else
		return len(e.Properties) == 0
	}

	return false
}

type relocateVarsMode uint8

const (
	relocateVarsNormal relocateVarsMode = iota
	relocateVarsForInOrForOf
)

// If we are currently in a hoisted child of the module scope, relocate these
// declarations to the top level and return an equivalent assignment statement.
// Make sure to check that the declaration kind is "var" before calling this.
// And make sure to check that the returned statement is not the zero value.
//
// This is done to make it easier to traverse top-level declarations in the linker
// during bundling. Now it is sufficient to just scan the top-level statements
// instead of having to traverse recursively into the statement tree.
func (p *parser) maybeRelocateVarsToTopLevel(decls []js_ast.Decl, mode relocateVarsMode) (js_ast.Stmt, bool) {
	// Only do this when bundling, and not when the scope is already top-level
	if p.options.mode != config.ModeBundle || p.currentScope == p.moduleScope {
		return js_ast.Stmt{}, false
	}

	// Only do this if we're not inside a function
	scope := p.currentScope
	for !scope.Kind.StopsHoisting() {
		scope = scope.Parent
	}
	if scope != p.moduleScope {
		return js_ast.Stmt{}, false
	}

	// Convert the declarations to assignments
	wrapIdentifier := func(loc logger.Loc, ref ast.Ref) js_ast.Expr {
		p.relocatedTopLevelVars = append(p.relocatedTopLevelVars, ast.LocRef{Loc: loc, Ref: ref})
		p.recordUsage(ref)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	}
	var value js_ast.Expr
	for _, decl := range decls {
		binding := js_ast.ConvertBindingToExpr(decl.Binding, wrapIdentifier)
		if decl.ValueOrNil.Data != nil {
			value = js_ast.JoinWithComma(value, js_ast.Assign(binding, decl.ValueOrNil))
		} else if mode == relocateVarsForInOrForOf {
			value = js_ast.JoinWithComma(value, binding)
		}
	}
	if value.Data == nil {
		// If none of the variables had any initializers, just remove the declarations
		return js_ast.Stmt{}, true
	}
	return js_ast.Stmt{Loc: value.Loc, Data: &js_ast.SExpr{Value: value}}, true
}

func (p *parser) markExprAsParenthesized(value js_ast.Expr, openParenLoc logger.Loc, isAsync bool) {
	// Don't lose comments due to parentheses. For example, we don't want to lose
	// the comment here:
	//
	//   ( /* comment */ (foo) );
	//
	if !isAsync {
		if comments, ok := p.exprComments[openParenLoc]; ok {
			delete(p.exprComments, openParenLoc)
			p.exprComments[value.Loc] = append(comments, p.exprComments[value.Loc]...)
		}
	}

	switch e := value.Data.(type) {
	case *js_ast.EArray:
		e.IsParenthesized = true
	case *js_ast.EObject:
		e.IsParenthesized = true
	}
}

func (p *parser) maybeTransposeIfExprChain(expr js_ast.Expr, visit func(js_ast.Expr) js_ast.Expr) js_ast.Expr {
	if e, ok := expr.Data.(*js_ast.EIf); ok {
		e.Yes = p.maybeTransposeIfExprChain(e.Yes, visit)
		e.No = p.maybeTransposeIfExprChain(e.No, visit)
		return expr
	}
	return visit(expr)
}

func (p *parser) iifeCanBeRemovedIfUnused(args []js_ast.Arg, body js_ast.FnBody) bool {
	for _, arg := range args {
		if arg.DefaultOrNil.Data != nil && !p.astHelpers.ExprCanBeRemovedIfUnused(arg.DefaultOrNil) {
			// The default value has a side effect
			return false
		}

		if _, ok := arg.Binding.Data.(*js_ast.BIdentifier); !ok {
			// Destructuring is a side effect (due to property access)
			return false
		}
	}

	// Check whether any statements have side effects or not. Consider return
	// statements as not having side effects because if the IIFE can be removed
	// then we know the return value is unused, so we know that returning the
	// value has no side effects.
	return p.astHelpers.StmtsCanBeRemovedIfUnused(body.Block.Stmts, js_ast.ReturnCanBeRemovedIfUnused)
}

type captureValueMode uint8

const (
	valueDefinitelyNotMutated captureValueMode = iota
	valueCouldBeMutated
)

// This is a helper function to use when you need to capture a value that may
// have side effects so you can use it multiple times. It guarantees that the
// side effects take place exactly once.
//
// Example usage:
//
//	// "value" => "value + value"
//	// "value()" => "(_a = value(), _a + _a)"
//	valueFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, value)
//	return wrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
//	  Op: js_ast.BinOpAdd,
//	  Left: valueFunc(),
//	  Right: valueFunc(),
//	}})
//
// This returns a function for generating references instead of a raw reference
// because AST nodes are supposed to be unique in memory, not aliases of other
// AST nodes. That way you can mutate one during lowering without having to
// worry about messing up other nodes.
func (p *parser) captureValueWithPossibleSideEffects(
	loc logger.Loc, // The location to use for the generated references
	count int, // The expected number of references to generate
	value js_ast.Expr, // The value that might have side effects
	mode captureValueMode, // Say if "value" might be mutated and must be captured
) (
	func() js_ast.Expr, // Generates reference expressions "_a"
	func(js_ast.Expr) js_ast.Expr, // Call this on the final expression
) {
	wrapFunc := func(expr js_ast.Expr) js_ast.Expr {
		// Make sure side effects still happen if no expression was generated
		if expr.Data == nil {
			return value
		}
		return expr
	}

	// Referencing certain expressions more than once has no side effects, so we
	// can just create them inline without capturing them in a temporary variable
	var valueFunc func() js_ast.Expr
	switch e := value.Data.(type) {
	case *js_ast.ENull:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: js_ast.ENullShared} }
	case *js_ast.EUndefined:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: js_ast.EUndefinedShared} }
	case *js_ast.EThis:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: js_ast.EThisShared} }
	case *js_ast.EBoolean:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: e.Value}} }
	case *js_ast.ENumber:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: e.Value}} }
	case *js_ast.EBigInt:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: &js_ast.EBigInt{Value: e.Value}} }
	case *js_ast.EString:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: e.Value}} }
	case *js_ast.EPrivateIdentifier:
		valueFunc = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: &js_ast.EPrivateIdentifier{Ref: e.Ref}} }
	case *js_ast.EIdentifier:
		if mode == valueDefinitelyNotMutated {
			valueFunc = func() js_ast.Expr {
				// Make sure we record this usage in the usage count so that duplicating
				// a single-use reference means it's no longer considered a single-use
				// reference. Otherwise the single-use reference inlining code may
				// incorrectly inline the initializer into the first reference, leaving
				// the second reference without a definition.
				p.recordUsage(e.Ref)
				return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: e.Ref}}
			}
		}
	}
	if valueFunc != nil {
		return valueFunc, wrapFunc
	}

	// We don't need to worry about side effects if the value won't be used
	// multiple times. This special case lets us avoid generating a temporary
	// reference.
	if count < 2 {
		return func() js_ast.Expr {
			return value
		}, wrapFunc
	}

	// Otherwise, fall back to generating a temporary reference
	tempRef := ast.InvalidRef

	// If we're in a function argument scope, then we won't be able to generate
	// symbols in this scope to store stuff, since there's nowhere to put the
	// variable declaration. We don't want to put the variable declaration
	// outside the function since some code in the argument list may cause the
	// function to be reentrant, and we can't put the variable declaration in
	// the function body since that's not accessible by the argument list.
	//
	// Instead, we use an immediately-invoked arrow function to create a new
	// symbol inline by introducing a new scope. Make sure to only use it for
	// symbol declaration and still initialize the variable inline to preserve
	// side effect order.
	if p.currentScope.Kind == js_ast.ScopeFunctionArgs {
		return func() js_ast.Expr {
				if tempRef == ast.InvalidRef {
					tempRef = p.generateTempRef(tempRefNoDeclare, "")

					// Assign inline so the order of side effects remains the same
					p.recordUsage(tempRef)
					return js_ast.Assign(js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}, value)
				}
				p.recordUsage(tempRef)
				return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}
			}, func(expr js_ast.Expr) js_ast.Expr {
				// Make sure side effects still happen if no expression was generated
				if expr.Data == nil {
					return value
				}

				// Generate a new variable using an arrow function to avoid messing with "this"
				return js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
					Target: js_ast.Expr{Loc: loc, Data: &js_ast.EArrow{
						Args:       []js_ast.Arg{{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: tempRef}}}},
						PreferExpr: true,
						Body:       js_ast.FnBody{Loc: loc, Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SReturn{ValueOrNil: expr}}}}},
					}},
				}}
			}
	}

	return func() js_ast.Expr {
		if tempRef == ast.InvalidRef {
			tempRef = p.generateTempRef(tempRefNeedsDeclare, "")
			p.recordUsage(tempRef)
			return js_ast.Assign(js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}, value)
		}
		p.recordUsage(tempRef)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}
	}, wrapFunc
}

func (p *parser) visitDecorators(decorators []js_ast.Decorator, decoratorScope *js_ast.Scope) []js_ast.Decorator {
	if decorators != nil {
		// Decorators cause us to temporarily revert to the scope that encloses the
		// class declaration, since that's where the generated code for decorators
		// will be inserted. I believe this currently only matters for parameter
		// decorators, where the scope should not be within the argument list.
		oldScope := p.currentScope
		p.currentScope = decoratorScope

		for i, decorator := range decorators {
			decorators[i].Value = p.visitExpr(decorator.Value)
		}

		// Avoid "popScope" because this decorator scope is not hierarchical
		p.currentScope = oldScope
	}

	return decorators
}

type visitClassResult struct {
	bodyScope         *js_ast.Scope
	innerClassNameRef ast.Ref
	superCtorRef      ast.Ref

	// If true, the class was determined to be safe to remove if the class is
	// never used (i.e. the class definition is side-effect free). This is
	// determined after visiting but before lowering since lowering may generate
	// class mutations that cannot be automatically analyzed as side-effect free.
	canBeRemovedIfUnused bool
}

func (p *parser) visitClass(nameScopeLoc logger.Loc, class *js_ast.Class, defaultNameRef ast.Ref, nameToKeep string) (result visitClassResult) {
	class.Decorators = p.visitDecorators(class.Decorators, p.currentScope)

	if class.Name != nil {
		p.recordDeclaredSymbol(class.Name.Ref)
		if p.options.keepNames {
			nameToKeep = p.symbols[class.Name.Ref.InnerIndex].OriginalName
		}
	}

	// Replace "this" with a reference to the class inside static field
	// initializers if static fields are being lowered, since that relocates the
	// field initializers outside of the class body and "this" will no longer
	// reference the same thing.
	classLoweringInfo := p.computeClassLoweringInfo(class)
	recomputeClassLoweringInfo := false

	// Sometimes we need to lower private members even though they are supported.
	// This flags them for lowering so that we lower references to them as we
	// traverse the class body.
	//
	// We don't need to worry about possible references to the class shadowing
	// symbol inside the class body changing our decision to lower private members
	// later on because that shouldn't be possible.
	if classLoweringInfo.lowerAllStaticFields {
		for _, prop := range class.Properties {
			// We need to lower all private members if fields of that type are lowered,
			// not just private fields (methods and accessors too):
			//
			//   class Foo {
			//     get #foo() {}
			//     static bar = new Foo().#foo
			//   }
			//
			// We can't transform that to this:
			//
			//   class Foo {
			//     get #foo() {}
			//   }
			//   Foo.bar = new Foo().#foo;
			//
			// The private getter must be lowered too.
			if private, ok := prop.Key.Data.(*js_ast.EPrivateIdentifier); ok {
				p.symbols[private.Ref.InnerIndex].Flags |= ast.PrivateSymbolMustBeLowered
				recomputeClassLoweringInfo = true
			}
		}
	}

	// Conservatively lower all private names that have been used in a private
	// brand check anywhere in the file. See the comment on this map for details.
	if p.lowerAllOfThesePrivateNames != nil {
		for _, prop := range class.Properties {
			if private, ok := prop.Key.Data.(*js_ast.EPrivateIdentifier); ok {
				if symbol := &p.symbols[private.Ref.InnerIndex]; p.lowerAllOfThesePrivateNames[symbol.OriginalName] {
					symbol.Flags |= ast.PrivateSymbolMustBeLowered
					recomputeClassLoweringInfo = true
				}
			}
		}
	}

	// If we changed private symbol lowering decisions, then recompute class
	// lowering info because that may have changed other decisions too
	if recomputeClassLoweringInfo {
		classLoweringInfo = p.computeClassLoweringInfo(class)
	}

	p.pushScopeForVisitPass(js_ast.ScopeClassName, nameScopeLoc)
	oldEnclosingClassKeyword := p.enclosingClassKeyword
	p.enclosingClassKeyword = class.ClassKeyword
	p.currentScope.RecursiveSetStrictMode(js_ast.ImplicitStrictModeClass)
	if class.Name != nil {
		p.validateDeclaredSymbolName(class.Name.Loc, p.symbols[class.Name.Ref.InnerIndex].OriginalName)
	}

	// Create the "__super" symbol if necessary. This will cause us to replace
	// all "super()" call expressions with a call to this symbol, which will
	// then be inserted into the "constructor" method.
	result.superCtorRef = ast.InvalidRef
	if classLoweringInfo.shimSuperCtorCalls {
		result.superCtorRef = p.newSymbol(ast.SymbolOther, "__super")
		p.currentScope.Generated = append(p.currentScope.Generated, result.superCtorRef)
		p.recordDeclaredSymbol(result.superCtorRef)
	}
	oldSuperCtorRef := p.superCtorRef
	p.superCtorRef = result.superCtorRef

	// Insert an immutable inner name that spans the whole class to match
	// JavaScript's semantics specifically the "CreateImmutableBinding" here:
	// https://262.ecma-international.org/6.0/#sec-runtime-semantics-classdefinitionevaluation
	// The class body (and extends clause) "captures" the original value of the
	// class name. This matters for class statements because the symbol can be
	// re-assigned to something else later. The captured values must be the
	// original value of the name, not the re-assigned value. Use "const" for
	// this symbol to match JavaScript run-time semantics. You are not allowed
	// to assign to this symbol (it throws a TypeError).
	if class.Name != nil {
		name := p.symbols[class.Name.Ref.InnerIndex].OriginalName
		result.innerClassNameRef = p.newSymbol(ast.SymbolConst, "_"+name)
		p.currentScope.Members[name] = js_ast.ScopeMember{Loc: class.Name.Loc, Ref: result.innerClassNameRef}
	} else {
		name := "_this"
		if defaultNameRef != ast.InvalidRef {
			name = "_" + p.source.IdentifierName + "_default"
		}
		result.innerClassNameRef = p.newSymbol(ast.SymbolConst, name)
	}
	p.recordDeclaredSymbol(result.innerClassNameRef)

	if class.ExtendsOrNil.Data != nil {
		class.ExtendsOrNil = p.visitExpr(class.ExtendsOrNil)
	}

	// A scope is needed for private identifiers
	p.pushScopeForVisitPass(js_ast.ScopeClassBody, class.BodyLoc)
	result.bodyScope = p.currentScope

	for i := range class.Properties {
		property := &class.Properties[i]

		if property.Kind == js_ast.PropertyClassStaticBlock {
			oldFnOrArrowData := p.fnOrArrowDataVisit
			oldFnOnlyDataVisit := p.fnOnlyDataVisit

			p.fnOrArrowDataVisit = fnOrArrowDataVisit{}
			p.fnOnlyDataVisit = fnOnlyDataVisit{
				isThisNested:           true,
				isNewTargetAllowed:     true,
				isInStaticClassContext: true,
				innerClassNameRef:      &result.innerClassNameRef,
			}

			if classLoweringInfo.lowerAllStaticFields {
				// Need to lower "this" and "super" since they won't be valid outside the class body
				p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef = true
				p.fnOrArrowDataVisit.shouldLowerSuperPropertyAccess = true
			}

			p.pushScopeForVisitPass(js_ast.ScopeClassStaticInit, property.ClassStaticBlock.Loc)

			// Make it an error to use "arguments" in a static class block
			p.currentScope.ForbidArguments = true

			property.ClassStaticBlock.Block.Stmts = p.visitStmts(property.ClassStaticBlock.Block.Stmts, stmtsFnBody)
			p.popScope()

			p.fnOrArrowDataVisit = oldFnOrArrowData
			p.fnOnlyDataVisit = oldFnOnlyDataVisit
			continue
		}

		property.Decorators = p.visitDecorators(property.Decorators, result.bodyScope)

		// Visit the property key
		if private, ok := property.Key.Data.(*js_ast.EPrivateIdentifier); ok {
			// Special-case private identifiers here
			p.recordDeclaredSymbol(private.Ref)
		} else {
			// It's forbidden to reference the class name in a computed key
			if property.Flags.Has(js_ast.PropertyIsComputed) && class.Name != nil {
				p.symbols[result.innerClassNameRef.InnerIndex].Kind = ast.SymbolClassInComputedPropertyKey
			}

			key, _ := p.visitExprInOut(property.Key, exprIn{
				shouldMangleStringsAsProps: true,
			})
			property.Key = key

			// Re-allow using the class name after visiting a computed key
			if property.Flags.Has(js_ast.PropertyIsComputed) && class.Name != nil {
				p.symbols[result.innerClassNameRef.InnerIndex].Kind = ast.SymbolConst
			}

			if p.options.minifySyntax {
				if inlined, ok := key.Data.(*js_ast.EInlinedEnum); ok {
					switch inlined.Value.Data.(type) {
					case *js_ast.EString, *js_ast.ENumber:
						key.Data = inlined.Value.Data
						property.Key.Data = key.Data
					}
				}
				switch k := key.Data.(type) {
				case *js_ast.ENumber, *js_ast.ENameOfSymbol:
					// "class { [123] }" => "class { 123 }"
					property.Flags &= ^js_ast.PropertyIsComputed
				case *js_ast.EString:
					if numberValue, ok := js_ast.StringToEquivalentNumberValue(k.Value); ok && numberValue >= 0 {
						// "class { '123' }" => "class { 123 }"
						property.Key.Data = &js_ast.ENumber{Value: numberValue}
						property.Flags &= ^js_ast.PropertyIsComputed
					} else if property.Flags.Has(js_ast.PropertyIsComputed) {
						// "class {['x'] = y}" => "class {'x' = y}"
						isInvalidConstructor := false
						if helpers.UTF16EqualsString(k.Value, "constructor") {
							if !property.Kind.IsMethodDefinition() {
								// "constructor" is an invalid name for both instance and static fields
								isInvalidConstructor = true
							} else if !property.Flags.Has(js_ast.PropertyIsStatic) {
								// Calling an instance method "constructor" is problematic so avoid that too
								isInvalidConstructor = true
							}
						}

						// A static property must not be called "prototype"
						isInvalidPrototype := property.Flags.Has(js_ast.PropertyIsStatic) && helpers.UTF16EqualsString(k.Value, "prototype")

						if !isInvalidConstructor && !isInvalidPrototype {
							property.Flags &= ^js_ast.PropertyIsComputed
						}
					}
				}
			}
		}

		// Make it an error to use "arguments" in a class body
		p.currentScope.ForbidArguments = true

		// The value of "this" and "super" is shadowed inside property values
		oldFnOnlyDataVisit := p.fnOnlyDataVisit
		oldShouldLowerSuperPropertyAccess := p.fnOrArrowDataVisit.shouldLowerSuperPropertyAccess
		p.fnOrArrowDataVisit.shouldLowerSuperPropertyAccess = false
		p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef = false
		p.fnOnlyDataVisit.isThisNested = true
		p.fnOnlyDataVisit.isNewTargetAllowed = true
		p.fnOnlyDataVisit.isInStaticClassContext = property.Flags.Has(js_ast.PropertyIsStatic)
		p.fnOnlyDataVisit.innerClassNameRef = &result.innerClassNameRef

		// We need to explicitly assign the name to the property initializer if it
		// will be transformed such that it is no longer an inline initializer.
		nameToKeep := ""
		isLoweredPrivateMethod := false
		if private, ok := property.Key.Data.(*js_ast.EPrivateIdentifier); ok {
			if !property.Kind.IsMethodDefinition() || p.privateSymbolNeedsToBeLowered(private) {
				nameToKeep = p.symbols[private.Ref.InnerIndex].OriginalName
			}

			// Lowered private methods (both instance and static) are initialized
			// outside of the class body, so we must rewrite "super" property
			// accesses inside them. Lowered private instance fields are initialized
			// inside the constructor where "super" is valid, so those don't need to
			// be rewritten.
			if property.Kind.IsMethodDefinition() && p.privateSymbolNeedsToBeLowered(private) {
				isLoweredPrivateMethod = true
			}
		} else if !property.Kind.IsMethodDefinition() && !property.Flags.Has(js_ast.PropertyIsComputed) {
			if str, ok := property.Key.Data.(*js_ast.EString); ok {
				nameToKeep = helpers.UTF16ToString(str.Value)
			}
		}

		// Handle methods
		if property.ValueOrNil.Data != nil {
			p.propMethodDecoratorScope = result.bodyScope

			// Propagate the name to keep from the method into the initializer
			if nameToKeep != "" {
				p.nameToKeep = nameToKeep
				p.nameToKeepIsFor = property.ValueOrNil.Data
			}

			// Propagate whether we're in a derived class constructor
			if class.ExtendsOrNil.Data != nil && !property.Flags.Has(js_ast.PropertyIsComputed) {
				if str, ok := property.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(str.Value, "constructor") {
					p.propDerivedCtorValue = property.ValueOrNil.Data
				}
			}

			property.ValueOrNil, _ = p.visitExprInOut(property.ValueOrNil, exprIn{
				isMethod:               true,
				isLoweredPrivateMethod: isLoweredPrivateMethod,
			})
		}

		// Handle initialized fields
		if property.InitializerOrNil.Data != nil {
			if property.Flags.Has(js_ast.PropertyIsStatic) && classLoweringInfo.lowerAllStaticFields {
				// Need to lower "this" and "super" since they won't be valid outside the class body
				p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef = true
				p.fnOrArrowDataVisit.shouldLowerSuperPropertyAccess = true
			}

			// Propagate the name to keep from the field into the initializer
			if nameToKeep != "" {
				p.nameToKeep = nameToKeep
				p.nameToKeepIsFor = property.InitializerOrNil.Data
			}

			property.InitializerOrNil = p.visitExpr(property.InitializerOrNil)
		}

		// Restore "this" so it will take the inherited value in property keys
		p.fnOnlyDataVisit = oldFnOnlyDataVisit
		p.fnOrArrowDataVisit.shouldLowerSuperPropertyAccess = oldShouldLowerSuperPropertyAccess

		// Restore the ability to use "arguments" in decorators and computed properties
		p.currentScope.ForbidArguments = false
	}

	// Check for and warn about duplicate keys in class bodies
	if !p.suppressWarningsAboutWeirdCode {
		p.warnAboutDuplicateProperties(class.Properties, duplicatePropertiesInClass)
	}

	// Analyze side effects before adding the name keeping call
	result.canBeRemovedIfUnused = p.astHelpers.ClassCanBeRemovedIfUnused(*class)

	// Implement name keeping using a static block at the start of the class body
	if p.options.keepNames && nameToKeep != "" {
		propertyPreventsKeepNames := false
		for _, prop := range class.Properties {
			// A static property called "name" shadows the automatically-generated name
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				if str, ok := prop.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(str.Value, "name") {
					propertyPreventsKeepNames = true
					break
				}
			}
		}
		if !propertyPreventsKeepNames {
			var this js_ast.Expr
			if classLoweringInfo.lowerAllStaticFields {
				p.recordUsage(result.innerClassNameRef)
				this = js_ast.Expr{Loc: class.BodyLoc, Data: &js_ast.EIdentifier{Ref: result.innerClassNameRef}}
			} else {
				this = js_ast.Expr{Loc: class.BodyLoc, Data: js_ast.EThisShared}
			}
			properties := make([]js_ast.Property, 0, 1+len(class.Properties))
			properties = append(properties, js_ast.Property{
				Kind: js_ast.PropertyClassStaticBlock,
				ClassStaticBlock: &js_ast.ClassStaticBlock{Loc: class.BodyLoc, Block: js_ast.SBlock{Stmts: []js_ast.Stmt{
					p.keepClassOrFnSymbolName(class.BodyLoc, this, nameToKeep),
				}}},
			})
			class.Properties = append(properties, class.Properties...)
		}
	}

	p.enclosingClassKeyword = oldEnclosingClassKeyword
	p.superCtorRef = oldSuperCtorRef
	p.popScope()

	if p.symbols[result.innerClassNameRef.InnerIndex].UseCountEstimate == 0 {
		// Don't generate a shadowing name if one isn't needed
		result.innerClassNameRef = ast.InvalidRef
	} else if class.Name == nil {
		// If there was originally no class name but something inside needed one
		// (e.g. there was a static property initializer that referenced "this"),
		// populate the class name. If this is an "export default class" statement,
		// use the existing default name so that things will work as expected if
		// this is turned into a regular class statement later on.
		classNameRef := defaultNameRef
		if classNameRef == ast.InvalidRef {
			classNameRef = p.newSymbol(ast.SymbolOther, "_this")
			p.currentScope.Generated = append(p.currentScope.Generated, classNameRef)
			p.recordDeclaredSymbol(classNameRef)
		}
		class.Name = &ast.LocRef{Loc: nameScopeLoc, Ref: classNameRef}
	}

	p.popScope()

	// Sanity check that the class lowering info hasn't changed before and after
	// visiting. The class transform relies on this because lowering assumes that
	// must be able to expect that visiting has done certain things.
	if classLoweringInfo != p.computeClassLoweringInfo(class) {
		panic("Internal error")
	}

	return
}

func isSimpleParameterList(args []js_ast.Arg, hasRestArg bool) bool {
	if hasRestArg {
		return false
	}
	for _, arg := range args {
		if _, ok := arg.Binding.Data.(*js_ast.BIdentifier); !ok || arg.DefaultOrNil.Data != nil {
			return false
		}
	}
	return true
}

func fnBodyContainsUseStrict(body []js_ast.Stmt) (logger.Loc, bool) {
	for _, stmt := range body {
		switch s := stmt.Data.(type) {
		case *js_ast.SComment:
			continue
		case *js_ast.SDirective:
			if helpers.UTF16EqualsString(s.Value, "use strict") {
				return stmt.Loc, true
			}
		default:
			return logger.Loc{}, false
		}
	}
	return logger.Loc{}, false
}

type visitArgsOpts struct {
	body           []js_ast.Stmt
	decoratorScope *js_ast.Scope
	hasRestArg     bool

	// This is true if the function is an arrow function or a method
	isUniqueFormalParameters bool
}

func (p *parser) visitArgs(args []js_ast.Arg, opts visitArgsOpts) {
	var duplicateArgCheck map[string]logger.Range
	useStrictLoc, hasUseStrict := fnBodyContainsUseStrict(opts.body)
	hasSimpleArgs := isSimpleParameterList(args, opts.hasRestArg)

	// Section 15.2.1 Static Semantics: Early Errors: "It is a Syntax Error if
	// FunctionBodyContainsUseStrict of FunctionBody is true and
	// IsSimpleParameterList of FormalParameters is false."
	if hasUseStrict && !hasSimpleArgs {
		p.log.AddError(&p.tracker, p.source.RangeOfString(useStrictLoc),
			"Cannot use a \"use strict\" directive in a function with a non-simple parameter list")
	}

	// Section 15.1.1 Static Semantics: Early Errors: "Multiple occurrences of
	// the same BindingIdentifier in a FormalParameterList is only allowed for
	// functions which have simple parameter lists and which are not defined in
	// strict mode code."
	if opts.isUniqueFormalParameters || hasUseStrict || !hasSimpleArgs || p.isStrictMode() {
		duplicateArgCheck = make(map[string]logger.Range)
	}

	for i := range args {
		arg := &args[i]
		arg.Decorators = p.visitDecorators(arg.Decorators, opts.decoratorScope)
		p.visitBinding(arg.Binding, bindingOpts{
			duplicateArgCheck: duplicateArgCheck,
		})
		if arg.DefaultOrNil.Data != nil {
			arg.DefaultOrNil = p.visitExpr(arg.DefaultOrNil)
		}
	}
}

func (p *parser) isDotOrIndexDefineMatch(expr js_ast.Expr, parts []string) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EDot:
		if len(parts) > 1 {
			// Intermediates must be dot expressions
			last := len(parts) - 1
			return parts[last] == e.Name && p.isDotOrIndexDefineMatch(e.Target, parts[:last])
		}

	case *js_ast.EIndex:
		if len(parts) > 1 {
			if str, ok := e.Index.Data.(*js_ast.EString); ok {
				// Intermediates must be dot expressions
				last := len(parts) - 1
				return parts[last] == helpers.UTF16ToString(str.Value) && p.isDotOrIndexDefineMatch(e.Target, parts[:last])
			}
		}

	case *js_ast.EThis:
		// Allow matching on top-level "this"
		if !p.fnOnlyDataVisit.isThisNested {
			return len(parts) == 1 && parts[0] == "this"
		}

	case *js_ast.EImportMeta:
		// Allow matching on "import.meta"
		return len(parts) == 2 && parts[0] == "import" && parts[1] == "meta"

	case *js_ast.EIdentifier:
		// The last expression must be an identifier
		if len(parts) == 1 {
			// The name must match
			name := p.loadNameFromRef(e.Ref)
			if name != parts[0] {
				return false
			}

			result := p.findSymbol(expr.Loc, name)

			// The "findSymbol" function also marks this symbol as used. But that's
			// never what we want here because we're just peeking to see what kind of
			// symbol it is to see if it's a match. If it's not a match, it will be
			// re-resolved again later and marked as used there. So we don't want to
			// mark it as used twice.
			p.ignoreUsage(result.ref)

			// We must not be in a "with" statement scope
			if result.isInsideWithScope {
				return false
			}

			// The last symbol must be unbound or injected
			return p.symbols[result.ref.InnerIndex].Kind.IsUnboundOrInjected()
		}
	}

	return false
}

func (p *parser) instantiateDefineExpr(loc logger.Loc, expr config.DefineExpr, opts identifierOpts) js_ast.Expr {
	if expr.Constant != nil {
		return js_ast.Expr{Loc: loc, Data: expr.Constant}
	}

	if expr.InjectedDefineIndex.IsValid() {
		ref := p.injectedDefineSymbols[expr.InjectedDefineIndex.GetIndex()]
		p.recordUsage(ref)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	}

	parts := expr.Parts
	if len(parts) == 0 {
		return js_ast.Expr{}
	}

	// Check both user-specified defines and known globals
	if opts.matchAgainstDefines {
		// Make sure define resolution is not recursive
		opts.matchAgainstDefines = false

		// Substitute user-specified defines
		if defines, ok := p.options.defines.DotDefines[parts[len(parts)-1]]; ok {
			for _, define := range defines {
				if define.DefineExpr != nil && helpers.StringArraysEqual(define.KeyParts, parts) {
					return p.instantiateDefineExpr(loc, *define.DefineExpr, opts)
				}
			}
		}
	}

	// Check injected dot names
	if names, ok := p.injectedDotNames[parts[len(parts)-1]]; ok {
		for _, name := range names {
			if helpers.StringArraysEqual(name.parts, parts) {
				return p.instantiateInjectDotName(loc, name, opts.assignTarget)
			}
		}
	}

	// Generate an identifier for the first part
	var value js_ast.Expr
	firstPart := parts[0]
	parts = parts[1:]
	switch firstPart {
	case "NaN":
		value = js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: math.NaN()}}

	case "Infinity":
		value = js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: math.Inf(1)}}

	case "null":
		value = js_ast.Expr{Loc: loc, Data: js_ast.ENullShared}

	case "undefined":
		value = js_ast.Expr{Loc: loc, Data: js_ast.EUndefinedShared}

	case "this":
		if thisValue, ok := p.valueForThis(loc, false /* shouldLog */, js_ast.AssignTargetNone, false, false); ok {
			value = thisValue
		} else {
			value = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
		}

	default:
		if firstPart == "import" && len(parts) > 0 && parts[0] == "meta" {
			if importMeta, ok := p.valueForImportMeta(loc); ok {
				value = importMeta
			} else {
				value = js_ast.Expr{Loc: loc, Data: &js_ast.EImportMeta{}}
			}
			parts = parts[1:]
			break
		}

		result := p.findSymbol(loc, firstPart)
		value = p.handleIdentifier(loc, &js_ast.EIdentifier{
			Ref:                   result.ref,
			MustKeepDueToWithStmt: result.isInsideWithScope,

			// Enable tree shaking
			CanBeRemovedIfUnused: true,
		}, opts)
	}

	// Build up a chain of property access expressions for subsequent parts
	for _, part := range parts {
		if expr, ok := p.maybeRewritePropertyAccess(loc, js_ast.AssignTargetNone, false, value, part, loc, false, false, false); ok {
			value = expr
		} else if p.isMangledProp(part) {
			value = js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
				Target: value,
				Index:  js_ast.Expr{Loc: loc, Data: &js_ast.ENameOfSymbol{Ref: p.symbolForMangledProp(part)}},
			}}
		} else {
			value = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
				Target:  value,
				Name:    part,
				NameLoc: loc,

				// Enable tree shaking
				CanBeRemovedIfUnused: true,
			}}
		}
	}

	return value
}

func (p *parser) instantiateInjectDotName(loc logger.Loc, name injectedDotName, assignTarget js_ast.AssignTarget) js_ast.Expr {
	// Note: We don't need to "ignoreRef" on the underlying identifier
	// because we have only parsed it but not visited it yet
	ref := p.injectedDefineSymbols[name.injectedDefineIndex]
	p.recordUsage(ref)

	if assignTarget != js_ast.AssignTargetNone {
		if where, ok := p.injectedSymbolSources[ref]; ok {
			r := js_lexer.RangeOfIdentifier(p.source, loc)
			tracker := logger.MakeLineColumnTracker(&where.source)
			joined := strings.Join(name.parts, ".")
			p.log.AddErrorWithNotes(&p.tracker, r,
				fmt.Sprintf("Cannot assign to %q because it's an import from an injected file", joined),
				[]logger.MsgData{tracker.MsgData(js_lexer.RangeOfIdentifier(where.source, where.loc),
					fmt.Sprintf("The symbol %q was exported from %q here:", joined, where.source.PrettyPath))})
		}
	}

	return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
}

func (p *parser) checkForUnrepresentableIdentifier(loc logger.Loc, name string) {
	if p.options.asciiOnly && p.options.unsupportedJSFeatures.Has(compat.UnicodeEscapes) &&
		helpers.ContainsNonBMPCodePoint(name) {
		if p.unrepresentableIdentifiers == nil {
			p.unrepresentableIdentifiers = make(map[string]bool)
		}
		if !p.unrepresentableIdentifiers[name] {
			p.unrepresentableIdentifiers[name] = true
			where := config.PrettyPrintTargetEnvironment(p.options.originalTargetEnv, p.options.unsupportedJSFeatureOverridesMask)
			r := js_lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddError(&p.tracker, r, fmt.Sprintf("%q cannot be escaped in %s but you "+
				"can set the charset to \"utf8\" to allow unescaped Unicode characters", name, where))
		}
	}
}

type typeofStringOrder uint8

const (
	onlyCheckOriginalOrder typeofStringOrder = iota
	checkBothOrders
)

func (p *parser) warnAboutTypeofAndString(a js_ast.Expr, b js_ast.Expr, order typeofStringOrder) {
	if order == checkBothOrders {
		if _, ok := a.Data.(*js_ast.EString); ok {
			a, b = b, a
		}
	}

	if typeof, ok := a.Data.(*js_ast.EUnary); ok && typeof.Op == js_ast.UnOpTypeof {
		if str, ok := b.Data.(*js_ast.EString); ok {
			value := helpers.UTF16ToString(str.Value)
			switch value {
			case "undefined", "object", "boolean", "number", "bigint", "string", "symbol", "function", "unknown":
			default:
				// Warn about typeof comparisons with values that will never be
				// returned. Here's an example of code with this problem:
				// https://github.com/olifolkerd/tabulator/issues/2962
				r := p.source.RangeOfString(b.Loc)
				text := fmt.Sprintf("The \"typeof\" operator will never evaluate to %q", value)
				kind := logger.Warning
				if p.suppressWarningsAboutWeirdCode {
					kind = logger.Debug
				}
				var notes []logger.MsgData
				if value == "null" {
					notes = append(notes, logger.MsgData{
						Text: "The expression \"typeof x\" actually evaluates to \"object\" in JavaScript, not \"null\". " +
							"You need to use \"x === null\" to test for null.",
					})
				}
				p.log.AddIDWithNotes(logger.MsgID_JS_ImpossibleTypeof, kind, &p.tracker, r, text, notes)
			}
		}
	}
}

func (p *parser) warnAboutEqualityCheck(op string, value js_ast.Expr, afterOpLoc logger.Loc) bool {
	switch e := value.Data.(type) {
	case *js_ast.ENumber:
		// "0 === -0" is true in JavaScript. Here's an example of code with this
		// problem: https://github.com/mrdoob/three.js/pull/11183
		if e.Value == 0 && math.Signbit(e.Value) {
			r := logger.Range{Loc: value.Loc, Len: 0}
			if int(r.Loc.Start) < len(p.source.Contents) && p.source.Contents[r.Loc.Start] == '-' {
				zeroRange := p.source.RangeOfNumber(logger.Loc{Start: r.Loc.Start + 1})
				r.Len = zeroRange.Len + 1
			}
			text := fmt.Sprintf("Comparison with -0 using the %q operator will also match 0", op)
			if op == "case" {
				text = "Comparison with -0 using a case clause will also match 0"
			}
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode {
				kind = logger.Debug
			}
			p.log.AddIDWithNotes(logger.MsgID_JS_EqualsNegativeZero, kind, &p.tracker, r, text,
				[]logger.MsgData{{Text: "Floating-point equality is defined such that 0 and -0 are equal, so \"x === -0\" returns true for both 0 and -0. " +
					"You need to use \"Object.is(x, -0)\" instead to test for -0."}})
			return true
		}

		// "NaN === NaN" is false in JavaScript
		if math.IsNaN(e.Value) {
			text := fmt.Sprintf("Comparison with NaN using the %q operator here is always %v", op, op[0] == '!')
			if op == "case" {
				text = "This case clause will never be evaluated because equality with NaN is always false"
			}
			r := p.source.RangeOfOperatorBefore(afterOpLoc, op)
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode {
				kind = logger.Debug
			}
			p.log.AddIDWithNotes(logger.MsgID_JS_EqualsNaN, kind, &p.tracker, r, text,
				[]logger.MsgData{{Text: "Floating-point equality is defined such that NaN is never equal to anything, so \"x === NaN\" always returns false. " +
					"You need to use \"Number.isNaN(x)\" instead to test for NaN."}})
			return true
		}

	case *js_ast.EArray, *js_ast.EArrow, *js_ast.EClass,
		*js_ast.EFunction, *js_ast.EObject, *js_ast.ERegExp:
		// This warning only applies to strict equality because loose equality can
		// cause string conversions. For example, "x == []" is true if x is the
		// empty string. Here's an example of code with this problem:
		// https://github.com/aws/aws-sdk-js/issues/3325
		if len(op) > 2 {
			text := fmt.Sprintf("Comparison using the %q operator here is always %v", op, op[0] == '!')
			if op == "case" {
				text = "This case clause will never be evaluated because the comparison is always false"
			}
			r := p.source.RangeOfOperatorBefore(afterOpLoc, op)
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode {
				kind = logger.Debug
			}
			p.log.AddIDWithNotes(logger.MsgID_JS_EqualsNewObject, kind, &p.tracker, r, text,
				[]logger.MsgData{{Text: "Equality with a new object is always false in JavaScript because the equality operator tests object identity. " +
					"You need to write code to compare the contents of the object instead. " +
					"For example, use \"Array.isArray(x) && x.length === 0\" instead of \"x === []\" to test for an empty array."}})
			return true
		}
	}

	return false
}

// EDot nodes represent a property access. This function may return an
// expression to replace the property access with. It assumes that the
// target of the EDot expression has already been visited.
func (p *parser) maybeRewritePropertyAccess(
	loc logger.Loc,
	assignTarget js_ast.AssignTarget,
	isDeleteTarget bool,
	target js_ast.Expr,
	name string,
	nameLoc logger.Loc,
	isCallTarget bool,
	isTemplateTag bool,
	preferQuotedKey bool,
) (js_ast.Expr, bool) {
	if id, ok := target.Data.(*js_ast.EIdentifier); ok {
		// Rewrite property accesses on explicit namespace imports as an identifier.
		// This lets us replace them easily in the printer to rebind them to
		// something else without paying the cost of a whole-tree traversal during
		// module linking just to rewrite these EDot expressions.
		if p.options.mode == config.ModeBundle {
			if importItems, ok := p.importItemsForNamespace[id.Ref]; ok {
				// Cache translation so each property access resolves to the same import
				item, ok := importItems.entries[name]
				if !ok {
					// Replace non-default imports with "undefined" for JSON import assertions
					if record := &p.importRecords[importItems.importRecordIndex]; (record.Flags&ast.AssertTypeJSON) != 0 && name != "default" {
						kind := logger.Warning
						if p.suppressWarningsAboutWeirdCode {
							kind = logger.Debug
						}
						p.log.AddIDWithNotes(logger.MsgID_JS_AssertTypeJSON, kind, &p.tracker, js_lexer.RangeOfIdentifier(p.source, nameLoc),
							fmt.Sprintf("Non-default import %q is undefined with a JSON import assertion", name),
							p.notesForAssertTypeJSON(record, name))
						p.ignoreUsage(id.Ref)
						return js_ast.Expr{Loc: loc, Data: js_ast.EUndefinedShared}, true
					}

					// Generate a new import item symbol in the module scope
					item = ast.LocRef{Loc: nameLoc, Ref: p.newSymbol(ast.SymbolImport, name)}
					p.moduleScope.Generated = append(p.moduleScope.Generated, item.Ref)

					// Link the namespace import and the import item together
					importItems.entries[name] = item
					p.isImportItem[item.Ref] = true

					symbol := &p.symbols[item.Ref.InnerIndex]
					if p.options.mode == config.ModePassThrough {
						// Make sure the printer prints this as a property access
						symbol.NamespaceAlias = &ast.NamespaceAlias{
							NamespaceRef: id.Ref,
							Alias:        name,
						}
					} else {
						// Mark this as generated in case it's missing. We don't want to
						// generate errors for missing import items that are automatically
						// generated.
						symbol.ImportItemStatus = ast.ImportItemGenerated
					}
				}

				// Undo the usage count for the namespace itself. This is used later
				// to detect whether the namespace symbol has ever been "captured"
				// or whether it has just been used to read properties off of.
				//
				// The benefit of doing this is that if both this module and the
				// imported module end up in the same module group and the namespace
				// symbol has never been captured, then we don't need to generate
				// any code for the namespace at all.
				p.ignoreUsage(id.Ref)

				// Track how many times we've referenced this symbol
				p.recordUsage(item.Ref)
				return p.handleIdentifier(nameLoc, &js_ast.EIdentifier{Ref: item.Ref}, identifierOpts{
					assignTarget:    assignTarget,
					isCallTarget:    isCallTarget,
					isDeleteTarget:  isDeleteTarget,
					preferQuotedKey: preferQuotedKey,

					// If this expression is used as the target of a call expression, make
					// sure the value of "this" is preserved.
					wasOriginallyIdentifier: false,
				}), true
			}

			// Rewrite "module.require()" to "require()" for Webpack compatibility.
			// See https://github.com/webpack/webpack/pull/7750 for more info.
			if isCallTarget && id.Ref == p.moduleRef && name == "require" {
				p.ignoreUsage(p.moduleRef)

				// This uses "require" instead of a reference to our "__require"
				// function so that the code coming up that detects calls to
				// "require" will recognize it.
				p.recordUsage(p.requireRef)
				return js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: p.requireRef}}, true
			}
		}
	}

	// Attempt to simplify statically-determined object literal property accesses
	if !isCallTarget && !isTemplateTag && p.options.minifySyntax && assignTarget == js_ast.AssignTargetNone {
		if object, ok := target.Data.(*js_ast.EObject); ok {
			var replace js_ast.Expr
			hasProtoNull := false
			isUnsafe := false

			// Check that doing this is safe
			for _, prop := range object.Properties {
				// "{ ...a }.a" must be preserved
				// "new ({ a() {} }.a)" must throw
				// "{ get a() {} }.a" must be preserved
				// "{ set a(b) {} }.a = 1" must be preserved
				// "{ a: 1, [String.fromCharCode(97)]: 2 }.a" must be 2
				if prop.Kind == js_ast.PropertySpread || prop.Flags.Has(js_ast.PropertyIsComputed) || prop.Kind.IsMethodDefinition() {
					isUnsafe = true
					break
				}

				// Do not attempt to compare against numeric keys
				key, ok := prop.Key.Data.(*js_ast.EString)
				if !ok {
					isUnsafe = true
					break
				}

				// The "__proto__" key has special behavior
				if helpers.UTF16EqualsString(key.Value, "__proto__") {
					if _, ok := prop.ValueOrNil.Data.(*js_ast.ENull); ok {
						// Replacing "{__proto__: null}.a" with undefined should be safe
						hasProtoNull = true
					}
				}

				// This entire object literal must have no side effects
				if !p.astHelpers.ExprCanBeRemovedIfUnused(prop.ValueOrNil) {
					isUnsafe = true
					break
				}

				// Note that we need to take the last value if there are duplicate keys
				if helpers.UTF16EqualsString(key.Value, name) {
					replace = prop.ValueOrNil
				}
			}

			if !isUnsafe {
				// If the key was found, return the value for that key. Note
				// that "{__proto__: null}.__proto__" is undefined, not null.
				if replace.Data != nil && name != "__proto__" {
					return replace, true
				}

				// We can only return "undefined" when a key is missing if the prototype is null
				if hasProtoNull {
					return js_ast.Expr{Loc: target.Loc, Data: js_ast.EUndefinedShared}, true
				}
			}
		}
	}

	// Handle references to namespaces or namespace members
	if target.Data == p.tsNamespaceTarget && assignTarget == js_ast.AssignTargetNone && !isDeleteTarget {
		if ns, ok := p.tsNamespaceMemberData.(*js_ast.TSNamespaceMemberNamespace); ok {
			if member, ok := ns.ExportedMembers[name]; ok {
				switch m := member.Data.(type) {
				case *js_ast.TSNamespaceMemberEnumNumber:
					p.ignoreUsageOfIdentifierInDotChain(target)
					return p.wrapInlinedEnum(js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: m.Value}}, name), true

				case *js_ast.TSNamespaceMemberEnumString:
					p.ignoreUsageOfIdentifierInDotChain(target)
					return p.wrapInlinedEnum(js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: m.Value}}, name), true

				case *js_ast.TSNamespaceMemberNamespace:
					// If this isn't a constant, return a clone of this property access
					// but with the namespace member data associated with it so that
					// more property accesses off of this property access are recognized.
					if preferQuotedKey || !js_ast.IsIdentifier(name) {
						p.tsNamespaceTarget = &js_ast.EIndex{
							Target: target,
							Index:  js_ast.Expr{Loc: nameLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(name)}},
						}
					} else {
						p.tsNamespaceTarget = p.dotOrMangledPropVisit(target, name, nameLoc)
					}
					p.tsNamespaceMemberData = member.Data
					return js_ast.Expr{Loc: loc, Data: p.tsNamespaceTarget}, true
				}
			}
		}
	}

	// Symbol uses due to a property access off of an imported symbol are tracked
	// specially. This lets us do tree shaking for cross-file TypeScript enums.
	if p.options.mode == config.ModeBundle && !p.isControlFlowDead {
		if id, ok := target.Data.(*js_ast.EImportIdentifier); ok {
			// Remove the normal symbol use
			use := p.symbolUses[id.Ref]
			use.CountEstimate--
			if use.CountEstimate == 0 {
				delete(p.symbolUses, id.Ref)
			} else {
				p.symbolUses[id.Ref] = use
			}

			// Add a special symbol use instead
			if p.importSymbolPropertyUses == nil {
				p.importSymbolPropertyUses = make(map[ast.Ref]map[string]js_ast.SymbolUse)
			}
			properties := p.importSymbolPropertyUses[id.Ref]
			if properties == nil {
				properties = make(map[string]js_ast.SymbolUse)
				p.importSymbolPropertyUses[id.Ref] = properties
			}
			use = properties[name]
			use.CountEstimate++
			properties[name] = use
		}
	}

	// Minify "foo".length
	if p.options.minifySyntax && assignTarget == js_ast.AssignTargetNone {
		switch t := target.Data.(type) {
		case *js_ast.EString:
			if name == "length" {
				return js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: float64(len(t.Value))}}, true
			}
		case *js_ast.EInlinedEnum:
			if s, ok := t.Value.Data.(*js_ast.EString); ok && name == "length" {
				return js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: float64(len(s.Value))}}, true
			}
		}
	}

	return js_ast.Expr{}, false
}

type exprIn struct {
	isMethod               bool
	isLoweredPrivateMethod bool

	// This tells us if there are optional chain expressions (EDot, EIndex, or
	// ECall) that are chained on to this expression. Because of the way the AST
	// works, chaining expressions on to this expression means they are our
	// parent expressions.
	//
	// Some examples:
	//
	//   a?.b.c  // EDot
	//   a?.b[c] // EIndex
	//   a?.b()  // ECall
	//
	// Note that this is false if our parent is a node with a OptionalChain
	// value of OptionalChainStart. That means it's the start of a new chain, so
	// it's not considered part of this one.
	//
	// Some examples:
	//
	//   a?.b?.c   // EDot
	//   a?.b?.[c] // EIndex
	//   a?.b?.()  // ECall
	//
	// Also note that this is false if our parent is a node with a OptionalChain
	// value of OptionalChainNone. That means it's outside parentheses, which
	// means it's no longer part of the chain.
	//
	// Some examples:
	//
	//   (a?.b).c  // EDot
	//   (a?.b)[c] // EIndex
	//   (a?.b)()  // ECall
	//
	hasChainParent bool

	// If our parent is an ECall node with an OptionalChain value of
	// OptionalChainStart, then we will need to store the value for the "this" of
	// that call somewhere if the current expression is an optional chain that
	// ends in a property access. That's because the value for "this" will be
	// used twice: once for the inner optional chain and once for the outer
	// optional chain.
	//
	// Example:
	//
	//   // Original
	//   a?.b?.();
	//
	//   // Lowered
	//   var _a;
	//   (_a = a == null ? void 0 : a.b) == null ? void 0 : _a.call(a);
	//
	// In the example above we need to store "a" as the value for "this" so we
	// can substitute it back in when we call "_a" if "_a" is indeed present.
	// See also "thisArgFunc" and "thisArgWrapFunc" in "exprOut".
	storeThisArgForParentOptionalChain bool

	// If true, string literals that match the current property mangling pattern
	// should be turned into ENameOfSymbol expressions, which will cause us to
	// rename them in the linker.
	shouldMangleStringsAsProps bool

	// Certain substitutions of identifiers are disallowed for assignment targets.
	// For example, we shouldn't transform "undefined = 1" into "void 0 = 1". This
	// isn't something real-world code would do but it matters for conformance
	// tests.
	assignTarget js_ast.AssignTarget
}

type exprOut struct {
	// If our parent is an ECall node with an OptionalChain value of
	// OptionalChainContinue, then we may need to return the value for "this"
	// from this node or one of this node's children so that the parent that is
	// the end of the optional chain can use it.
	//
	// Example:
	//
	//   // Original
	//   a?.b?.().c();
	//
	//   // Lowered
	//   var _a;
	//   (_a = a == null ? void 0 : a.b) == null ? void 0 : _a.call(a).c();
	//
	// The value "_a" for "this" must be passed all the way up to the call to
	// ".c()" which is where the optional chain is lowered. From there it must
	// be substituted as the value for "this" in the call to ".b?.()". See also
	// "storeThisArgForParentOptionalChain" in "exprIn".
	thisArgFunc     func() js_ast.Expr
	thisArgWrapFunc func(js_ast.Expr) js_ast.Expr

	// True if the child node is an optional chain node (EDot, EIndex, or ECall
	// with an IsOptionalChain value of true)
	childContainsOptionalChain bool

	// If true and this is used as a call target, the whole call expression
	// must be replaced with undefined.
	callMustBeReplacedWithUndefined       bool
	methodCallMustBeReplacedWithUndefined bool
}

func (p *parser) visitExpr(expr js_ast.Expr) js_ast.Expr {
	expr, _ = p.visitExprInOut(expr, exprIn{})
	return expr
}

func (p *parser) valueForThis(
	loc logger.Loc,
	shouldLog bool,
	assignTarget js_ast.AssignTarget,
	isCallTarget bool,
	isDeleteTarget bool,
) (js_ast.Expr, bool) {
	// Substitute "this" if we're inside a static class context
	if p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef {
		p.recordUsage(*p.fnOnlyDataVisit.innerClassNameRef)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: *p.fnOnlyDataVisit.innerClassNameRef}}, true
	}

	// Is this a top-level use of "this"?
	if !p.fnOnlyDataVisit.isThisNested {
		// Substitute user-specified defines
		if data, ok := p.options.defines.IdentifierDefines["this"]; ok {
			if data.DefineExpr != nil {
				return p.instantiateDefineExpr(loc, *data.DefineExpr, identifierOpts{
					assignTarget:   assignTarget,
					isCallTarget:   isCallTarget,
					isDeleteTarget: isDeleteTarget,
				}), true
			}
		}

		// Otherwise, replace top-level "this" with either "undefined" or "exports"
		if p.isFileConsideredToHaveESMExports {
			// Warn about "this" becoming undefined, but only once per file
			if shouldLog && !p.messageAboutThisIsUndefined && !p.fnOnlyDataVisit.silenceMessageAboutThisBeingUndefined {
				p.messageAboutThisIsUndefined = true
				kind := logger.Debug
				data := p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, loc),
					"Top-level \"this\" will be replaced with undefined since this file is an ECMAScript module")
				data.Location.Suggestion = "undefined"
				_, notes := p.whyESModule()
				p.log.AddMsgID(logger.MsgID_JS_ThisIsUndefinedInESM, logger.Msg{Kind: kind, Data: data, Notes: notes})
			}

			// In an ES6 module, "this" is supposed to be undefined. Instead of
			// doing this at runtime using "fn.call(undefined)", we do it at
			// compile time using expression substitution here.
			return js_ast.Expr{Loc: loc, Data: js_ast.EUndefinedShared}, true
		} else if p.options.mode != config.ModePassThrough {
			// In a CommonJS module, "this" is supposed to be the same as "exports".
			// Instead of doing this at runtime using "fn.call(module.exports)", we
			// do it at compile time using expression substitution here.
			p.recordUsage(p.exportsRef)
			return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.exportsRef}}, true
		}
	}

	return js_ast.Expr{}, false
}

func (p *parser) valueForImportMeta(loc logger.Loc) (js_ast.Expr, bool) {
	if p.options.unsupportedJSFeatures.Has(compat.ImportMeta) ||
		(p.options.mode != config.ModePassThrough && !p.options.outputFormat.KeepESMImportExportSyntax()) {
		// Generate the variable if it doesn't exist yet
		if p.importMetaRef == ast.InvalidRef {
			p.importMetaRef = p.newSymbol(ast.SymbolOther, "import_meta")
			p.moduleScope.Generated = append(p.moduleScope.Generated, p.importMetaRef)
		}

		// Replace "import.meta" with a reference to the symbol
		p.recordUsage(p.importMetaRef)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.importMetaRef}}, true
	}

	return js_ast.Expr{}, false
}

func locAfterOp(e *js_ast.EBinary) logger.Loc {
	if e.Left.Loc.Start < e.Right.Loc.Start {
		return e.Right.Loc
	} else {
		// Handle the case when we have transposed the operands
		return e.Left.Loc
	}
}

// This function exists to tie all of these checks together in one place
func isEvalOrArguments(name string) bool {
	return name == "eval" || name == "arguments"
}

func (p *parser) reportPrivateNameUsage(name string) {
	if p.parseExperimentalDecoratorNesting > 0 {
		if p.lowerAllOfThesePrivateNames == nil {
			p.lowerAllOfThesePrivateNames = make(map[string]bool)
		}
		p.lowerAllOfThesePrivateNames[name] = true
	}
}

func (p *parser) isValidAssignmentTarget(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EIdentifier:
		if p.isStrictMode() {
			if name := p.loadNameFromRef(e.Ref); isEvalOrArguments(name) {
				return false
			}
		}
		return true
	case *js_ast.EDot:
		return e.OptionalChain == js_ast.OptionalChainNone
	case *js_ast.EIndex:
		return e.OptionalChain == js_ast.OptionalChainNone

	// Don't worry about recursive checking for objects and arrays. This will
	// already be handled naturally by passing down the assign target flag.
	case *js_ast.EObject:
		return !e.IsParenthesized
	case *js_ast.EArray:
		return !e.IsParenthesized
	}
	return false
}

func containsClosingScriptTag(text string) bool {
	for {
		i := strings.Index(text, "</")
		if i < 0 {
			break
		}
		text = text[i+2:]
		if len(text) >= 6 && strings.EqualFold(text[:6], "script") {
			return true
		}
	}
	return false
}

func (p *parser) isUnsupportedRegularExpression(loc logger.Loc, value string) (pattern string, flags string, isUnsupported bool) {
	var what string
	var r logger.Range

	end := strings.LastIndexByte(value, '/')
	pattern = value[1:end]
	flags = value[end+1:]
	isUnicode := strings.IndexByte(flags, 'u') >= 0
	parenDepth := 0
	i := 0

	// Do a simple scan for unsupported features assuming the regular expression
	// is valid. This doesn't do a full validation of the regular expression
	// because regular expression grammar is complicated. If it contains a syntax
	// error that we don't catch, then we will just generate output code with a
	// syntax error. Garbage in, garbage out.
pattern:
	for i < len(pattern) {
		c := pattern[i]
		i++

		switch c {
		case '[':
		class:
			for i < len(pattern) {
				c := pattern[i]
				i++

				switch c {
				case ']':
					break class

				case '\\':
					i++ // Skip the escaped character
				}
			}

		case '(':
			tail := pattern[i:]

			if strings.HasPrefix(tail, "?<=") || strings.HasPrefix(tail, "?<!") {
				if p.options.unsupportedJSFeatures.Has(compat.RegexpLookbehindAssertions) {
					what = "Lookbehind assertions in regular expressions are not available"
					r = logger.Range{Loc: logger.Loc{Start: loc.Start + int32(i) + 1}, Len: 3}
					isUnsupported = true
					break pattern
				}
			} else if strings.HasPrefix(tail, "?<") {
				if p.options.unsupportedJSFeatures.Has(compat.RegexpNamedCaptureGroups) {
					if end := strings.IndexByte(tail, '>'); end >= 0 {
						what = "Named capture groups in regular expressions are not available"
						r = logger.Range{Loc: logger.Loc{Start: loc.Start + int32(i) + 1}, Len: int32(end) + 1}
						isUnsupported = true
						break pattern
					}
				}
			}

			parenDepth++

		case ')':
			if parenDepth == 0 {
				r := logger.Range{Loc: logger.Loc{Start: loc.Start + int32(i)}, Len: 1}
				p.log.AddError(&p.tracker, r, "Unexpected \")\" in regular expression")
				return
			}

			parenDepth--

		case '\\':
			tail := pattern[i:]

			if isUnicode && (strings.HasPrefix(tail, "p{") || strings.HasPrefix(tail, "P{")) {
				if p.options.unsupportedJSFeatures.Has(compat.RegexpUnicodePropertyEscapes) {
					if end := strings.IndexByte(tail, '}'); end >= 0 {
						what = "Unicode property escapes in regular expressions are not available"
						r = logger.Range{Loc: logger.Loc{Start: loc.Start + int32(i)}, Len: int32(end) + 2}
						isUnsupported = true
						break pattern
					}
				}
			}

			i++ // Skip the escaped character
		}
	}

	if !isUnsupported {
		for i, c := range flags {
			switch c {
			case 'g', 'i', 'm':
				continue // These are part of ES5 and are always supported

			case 's':
				if !p.options.unsupportedJSFeatures.Has(compat.RegexpDotAllFlag) {
					continue // This is part of ES2018
				}

			case 'y', 'u':
				if !p.options.unsupportedJSFeatures.Has(compat.RegexpStickyAndUnicodeFlags) {
					continue // These are part of ES2018
				}

			case 'd':
				if !p.options.unsupportedJSFeatures.Has(compat.RegexpMatchIndices) {
					continue // This is part of ES2022
				}

			case 'v':
				if !p.options.unsupportedJSFeatures.Has(compat.RegexpSetNotation) {
					continue // This is from a proposal: https://github.com/tc39/proposal-regexp-v-flag
				}

			default:
				// Unknown flags are never supported
			}

			r = logger.Range{Loc: logger.Loc{Start: loc.Start + int32(end+1) + int32(i)}, Len: 1}
			what = fmt.Sprintf("The regular expression flag \"%c\" is not available", c)
			isUnsupported = true
			break
		}
	}

	if isUnsupported {
		where := config.PrettyPrintTargetEnvironment(p.options.originalTargetEnv, p.options.unsupportedJSFeatureOverridesMask)
		p.log.AddIDWithNotes(logger.MsgID_JS_UnsupportedRegExp, logger.Debug, &p.tracker, r, fmt.Sprintf("%s in %s", what, where), []logger.MsgData{{
			Text: "This regular expression literal has been converted to a \"new RegExp()\" constructor " +
				"to avoid generating code with a syntax error. However, you will need to include a " +
				"polyfill for \"RegExp\" for your code to have the correct behavior at run-time."}})
	}

	return
}

// This function takes "exprIn" as input from the caller and produces "exprOut"
// for the caller to pass along extra data. This is mostly for optional chaining.
func (p *parser) visitExprInOut(expr js_ast.Expr, in exprIn) (js_ast.Expr, exprOut) {
	if in.assignTarget != js_ast.AssignTargetNone && !p.isValidAssignmentTarget(expr) {
		p.log.AddError(&p.tracker, logger.Range{Loc: expr.Loc}, "Invalid assignment target")
	}

	// Note: Anything added before or after this switch statement will be bypassed
	// when visiting nested "EBinary" nodes due to stack overflow mitigations for
	// deeply-nested ASTs. If anything like that is added, care must be taken that
	// it doesn't affect these mitigations by ensuring that the mitigations are not
	// applied in those cases (e.g. by adding an additional conditional check).
	switch e := expr.Data.(type) {
	case *js_ast.ENull, *js_ast.ESuper, *js_ast.EBoolean, *js_ast.EUndefined, *js_ast.EJSXText:

	case *js_ast.EBigInt:
		if p.options.unsupportedJSFeatures.Has(compat.Bigint) {
			// For ease of implementation, the actual reference of the "BigInt"
			// symbol is deferred to print time. That means we don't have to
			// special-case the "BigInt" constructor in side-effect computations
			// and future big integer constant folding (of which there isn't any
			// at the moment).
			p.markSyntaxFeature(compat.Bigint, p.source.RangeOfNumber(expr.Loc))
			p.recordUsage(p.makeBigIntRef())
		}

	case *js_ast.ENameOfSymbol:
		e.Ref = p.symbolForMangledProp(p.loadNameFromRef(e.Ref))

	case *js_ast.ERegExp:
		// "/pattern/flags" => "new RegExp('pattern', 'flags')"
		if pattern, flags, ok := p.isUnsupportedRegularExpression(expr.Loc, e.Value); ok {
			args := []js_ast.Expr{{
				Loc:  logger.Loc{Start: expr.Loc.Start + 1},
				Data: &js_ast.EString{Value: helpers.StringToUTF16(pattern)},
			}}
			if flags != "" {
				args = append(args, js_ast.Expr{
					Loc:  logger.Loc{Start: expr.Loc.Start + int32(len(pattern)) + 2},
					Data: &js_ast.EString{Value: helpers.StringToUTF16(flags)},
				})
			}
			regExpRef := p.makeRegExpRef()
			p.recordUsage(regExpRef)
			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENew{
				Target:        js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: regExpRef}},
				Args:          args,
				CloseParenLoc: logger.Loc{Start: expr.Loc.Start + int32(len(e.Value))},
			}}, exprOut{}
		}

	case *js_ast.ENewTarget:
		if !p.fnOnlyDataVisit.isNewTargetAllowed {
			p.log.AddError(&p.tracker, e.Range, "Cannot use \"new.target\" here:")
		}

	case *js_ast.EString:
		if e.LegacyOctalLoc.Start > 0 {
			if e.PreferTemplate {
				p.log.AddError(&p.tracker, p.source.RangeOfLegacyOctalEscape(e.LegacyOctalLoc),
					"Legacy octal escape sequences cannot be used in template literals")
			} else if p.isStrictMode() {
				p.markStrictModeFeature(legacyOctalEscape, p.source.RangeOfLegacyOctalEscape(e.LegacyOctalLoc), "")
			}
		}

		if in.shouldMangleStringsAsProps && p.options.mangleQuoted && !e.PreferTemplate {
			if name := helpers.UTF16ToString(e.Value); p.isMangledProp(name) {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENameOfSymbol{
					Ref:                   p.symbolForMangledProp(name),
					HasPropertyKeyComment: e.HasPropertyKeyComment,
				}}, exprOut{}
			}
		}

	case *js_ast.ENumber:
		if p.legacyOctalLiterals != nil && p.isStrictMode() {
			if r, ok := p.legacyOctalLiterals[expr.Data]; ok {
				p.markStrictModeFeature(legacyOctalLiteral, r, "")
			}
		}

	case *js_ast.EThis:
		isDeleteTarget := e == p.deleteTarget
		isCallTarget := e == p.callTarget

		if value, ok := p.valueForThis(expr.Loc, true /* shouldLog */, in.assignTarget, isDeleteTarget, isCallTarget); ok {
			return value, exprOut{}
		}

		// Capture "this" inside arrow functions that will be lowered into normal
		// function expressions for older language environments
		if p.fnOrArrowDataVisit.isArrow && p.options.unsupportedJSFeatures.Has(compat.Arrow) && p.fnOnlyDataVisit.isThisNested {
			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: p.captureThis()}}, exprOut{}
		}

	case *js_ast.EImportMeta:
		isDeleteTarget := e == p.deleteTarget
		isCallTarget := e == p.callTarget

		// Check both user-specified defines and known globals
		if defines, ok := p.options.defines.DotDefines["meta"]; ok {
			for _, define := range defines {
				if p.isDotOrIndexDefineMatch(expr, define.KeyParts) {
					// Substitute user-specified defines
					if define.DefineExpr != nil {
						return p.instantiateDefineExpr(expr.Loc, *define.DefineExpr, identifierOpts{
							assignTarget:   in.assignTarget,
							isCallTarget:   isCallTarget,
							isDeleteTarget: isDeleteTarget,
						}), exprOut{}
					}
				}
			}
		}

		// Check injected dot names
		if names, ok := p.injectedDotNames["meta"]; ok {
			for _, name := range names {
				if p.isDotOrIndexDefineMatch(expr, name.parts) {
					// Note: We don't need to "ignoreRef" on the underlying identifier
					// because we have only parsed it but not visited it yet
					return p.instantiateInjectDotName(expr.Loc, name, in.assignTarget), exprOut{}
				}
			}
		}

		// Warn about "import.meta" if it's not replaced by a define
		if p.options.unsupportedJSFeatures.Has(compat.ImportMeta) {
			r := logger.Range{Loc: expr.Loc, Len: e.RangeLen}
			p.markSyntaxFeature(compat.ImportMeta, r)
		} else if p.options.mode != config.ModePassThrough && !p.options.outputFormat.KeepESMImportExportSyntax() {
			r := logger.Range{Loc: expr.Loc, Len: e.RangeLen}
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode || p.fnOrArrowDataVisit.tryBodyCount > 0 {
				kind = logger.Debug
			}
			p.log.AddIDWithNotes(logger.MsgID_JS_EmptyImportMeta, kind, &p.tracker, r, fmt.Sprintf(
				"\"import.meta\" is not available with the %q output format and will be empty", p.options.outputFormat.String()),
				[]logger.MsgData{{Text: "You need to set the output format to \"esm\" for \"import.meta\" to work correctly."}})
		}

		// Convert "import.meta" to a variable if it's not supported in the output format
		if importMeta, ok := p.valueForImportMeta(expr.Loc); ok {
			return importMeta, exprOut{}
		}

	case *js_ast.ESpread:
		e.Value = p.visitExpr(e.Value)

	case *js_ast.EIdentifier:
		isCallTarget := e == p.callTarget
		isDeleteTarget := e == p.deleteTarget
		name := p.loadNameFromRef(e.Ref)
		if p.isStrictMode() && js_lexer.StrictModeReservedWords[name] {
			p.markStrictModeFeature(reservedWord, js_lexer.RangeOfIdentifier(p.source, expr.Loc), name)
		}
		result := p.findSymbol(expr.Loc, name)
		e.MustKeepDueToWithStmt = result.isInsideWithScope
		e.Ref = result.ref

		// Handle referencing a class name within that class's computed property
		// key. This is not allowed, and must fail at run-time:
		//
		//   class Foo {
		//     static foo = 'bar'
		//     static [Foo.foo] = 'foo'
		//   }
		//
		if p.symbols[result.ref.InnerIndex].Kind == ast.SymbolClassInComputedPropertyKey {
			p.log.AddID(logger.MsgID_JS_ClassNameWillThrow, logger.Warning, &p.tracker, js_lexer.RangeOfIdentifier(p.source, expr.Loc),
				fmt.Sprintf("Accessing class %q before initialization will throw", name))
			return p.callRuntime(expr.Loc, "__earlyAccess", []js_ast.Expr{{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(name)}}}), exprOut{}
		}

		// Handle assigning to a constant
		if in.assignTarget != js_ast.AssignTargetNone {
			switch p.symbols[result.ref.InnerIndex].Kind {
			case ast.SymbolConst:
				r := js_lexer.RangeOfIdentifier(p.source, expr.Loc)
				notes := []logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, result.declareLoc),
					fmt.Sprintf("The symbol %q was declared a constant here:", name))}

				// Make this an error when bundling because we may need to convert this
				// "const" into a "var" during bundling. Also make this an error when
				// the constant is inlined because we will otherwise generate code with
				// a syntax error.
				if _, isInlinedConstant := p.constValues[result.ref]; isInlinedConstant || p.options.mode == config.ModeBundle ||
					(p.currentScope.Parent == nil && p.willWrapModuleInTryCatchForUsing) {
					p.log.AddErrorWithNotes(&p.tracker, r,
						fmt.Sprintf("Cannot assign to %q because it is a constant", name), notes)
				} else {
					p.log.AddIDWithNotes(logger.MsgID_JS_AssignToConstant, logger.Warning, &p.tracker, r,
						fmt.Sprintf("This assignment will throw because %q is a constant", name), notes)
				}

			case ast.SymbolInjected:
				if where, ok := p.injectedSymbolSources[result.ref]; ok {
					r := js_lexer.RangeOfIdentifier(p.source, expr.Loc)
					tracker := logger.MakeLineColumnTracker(&where.source)
					p.log.AddErrorWithNotes(&p.tracker, r,
						fmt.Sprintf("Cannot assign to %q because it's an import from an injected file", name),
						[]logger.MsgData{tracker.MsgData(js_lexer.RangeOfIdentifier(where.source, where.loc),
							fmt.Sprintf("The symbol %q was exported from %q here:", name, where.source.PrettyPath))})
				}
			}
		}

		// Substitute user-specified defines for unbound or injected symbols
		methodCallMustBeReplacedWithUndefined := false
		if p.symbols[e.Ref.InnerIndex].Kind.IsUnboundOrInjected() && !result.isInsideWithScope && e != p.deleteTarget {
			if data, ok := p.options.defines.IdentifierDefines[name]; ok {
				if data.DefineExpr != nil {
					new := p.instantiateDefineExpr(expr.Loc, *data.DefineExpr, identifierOpts{
						assignTarget:   in.assignTarget,
						isCallTarget:   isCallTarget,
						isDeleteTarget: isDeleteTarget,
					})
					if in.assignTarget == js_ast.AssignTargetNone || defineValueCanBeUsedInAssignTarget(new.Data) {
						p.ignoreUsage(e.Ref)
						return new, exprOut{}
					} else {
						p.logAssignToDefine(js_lexer.RangeOfIdentifier(p.source, expr.Loc), name, js_ast.Expr{})
					}
				}

				// Copy the side effect flags over in case this expression is unused
				if data.Flags.Has(config.CanBeRemovedIfUnused) {
					e.CanBeRemovedIfUnused = true
				}
				if data.Flags.Has(config.CallCanBeUnwrappedIfUnused) && !p.options.ignoreDCEAnnotations {
					e.CallCanBeUnwrappedIfUnused = true
				}
				if data.Flags.Has(config.MethodCallsMustBeReplacedWithUndefined) {
					methodCallMustBeReplacedWithUndefined = true
				}
			}
		}

		return p.handleIdentifier(expr.Loc, e, identifierOpts{
				assignTarget:            in.assignTarget,
				isCallTarget:            isCallTarget,
				isDeleteTarget:          isDeleteTarget,
				wasOriginallyIdentifier: true,
			}), exprOut{
				methodCallMustBeReplacedWithUndefined: methodCallMustBeReplacedWithUndefined,
			}

	case *js_ast.EJSXElement:
		propsLoc := expr.Loc

		// Resolving the location index to a specific line and column in
		// development mode is not too expensive because we seek from the
		// previous JSX element. It amounts to at most a single additional
		// scan over the source code. Note that this has to happen before
		// we visit anything about this JSX element to make sure that we
		// only ever need to scan forward, not backward.
		var jsxSourceLine int
		var jsxSourceColumn int
		if p.options.jsx.Development && p.options.jsx.AutomaticRuntime {
			for p.jsxSourceLoc < int(propsLoc.Start) {
				r, size := utf8.DecodeRuneInString(p.source.Contents[p.jsxSourceLoc:])
				p.jsxSourceLoc += size
				if r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
					if r == '\r' && p.jsxSourceLoc < len(p.source.Contents) && p.source.Contents[p.jsxSourceLoc] == '\n' {
						p.jsxSourceLoc++ // Handle Windows-style CRLF newlines
					}
					p.jsxSourceLine++
					p.jsxSourceColumn = 0
				} else {
					// Babel and TypeScript count columns in UTF-16 code units
					if r < 0xFFFF {
						p.jsxSourceColumn++
					} else {
						p.jsxSourceColumn += 2
					}
				}
			}
			jsxSourceLine = p.jsxSourceLine
			jsxSourceColumn = p.jsxSourceColumn
		}

		if e.TagOrNil.Data != nil {
			propsLoc = e.TagOrNil.Loc
			e.TagOrNil = p.visitExpr(e.TagOrNil)
			p.warnAboutImportNamespaceCall(e.TagOrNil, exprKindJSXTag)
		}

		// Visit properties
		hasSpread := false
		for i, property := range e.Properties {
			if property.Kind == js_ast.PropertySpread {
				hasSpread = true
			} else {
				if mangled, ok := property.Key.Data.(*js_ast.ENameOfSymbol); ok {
					mangled.Ref = p.symbolForMangledProp(p.loadNameFromRef(mangled.Ref))
				} else {
					property.Key = p.visitExpr(property.Key)
				}
			}
			if property.ValueOrNil.Data != nil {
				property.ValueOrNil = p.visitExpr(property.ValueOrNil)
			}
			if property.InitializerOrNil.Data != nil {
				property.InitializerOrNil = p.visitExpr(property.InitializerOrNil)
			}
			e.Properties[i] = property
		}

		// "{a, ...{b, c}, d}" => "{a, b, c, d}"
		if p.options.minifySyntax && hasSpread {
			e.Properties = js_ast.MangleObjectSpread(e.Properties)
		}

		// Visit children
		if len(e.NullableChildren) > 0 {
			for i, childOrNil := range e.NullableChildren {
				if childOrNil.Data != nil {
					e.NullableChildren[i] = p.visitExpr(childOrNil)
				}
			}
		}

		if p.options.jsx.Preserve {
			// If the tag is an identifier, mark it as needing to be upper-case
			switch tag := e.TagOrNil.Data.(type) {
			case *js_ast.EIdentifier:
				p.symbols[tag.Ref.InnerIndex].Flags |= ast.MustStartWithCapitalLetterForJSX

			case *js_ast.EImportIdentifier:
				p.symbols[tag.Ref.InnerIndex].Flags |= ast.MustStartWithCapitalLetterForJSX
			}
		} else {
			// Remove any nil children in the array (in place) before iterating over it
			children := e.NullableChildren
			{
				end := 0
				for _, childOrNil := range children {
					if childOrNil.Data != nil {
						children[end] = childOrNil
						end++
					}
				}
				children = children[:end]
			}

			// A missing tag is a fragment
			if e.TagOrNil.Data == nil {
				if p.options.jsx.AutomaticRuntime {
					e.TagOrNil = p.importJSXSymbol(expr.Loc, JSXImportFragment)
				} else {
					e.TagOrNil = p.instantiateDefineExpr(expr.Loc, p.options.jsx.Fragment, identifierOpts{
						wasOriginallyIdentifier: true,
						matchAgainstDefines:     true, // Allow defines to rewrite the JSX fragment factory
					})
				}
			}

			shouldUseCreateElement := !p.options.jsx.AutomaticRuntime
			if !shouldUseCreateElement {
				// Even for runtime="automatic", <div {...props} key={key} /> is special cased to createElement
				// See https://github.com/babel/babel/blob/e482c763466ba3f44cb9e3467583b78b7f030b4a/packages/babel-plugin-transform-react-jsx/src/create-plugin.ts#L352
				seenPropsSpread := false
				for _, property := range e.Properties {
					if seenPropsSpread && property.Kind == js_ast.PropertyField {
						if str, ok := property.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(str.Value, "key") {
							shouldUseCreateElement = true
							break
						}
					} else if property.Kind == js_ast.PropertySpread {
						seenPropsSpread = true
					}
				}
			}

			if shouldUseCreateElement {
				// Arguments to createElement()
				args := []js_ast.Expr{e.TagOrNil}
				if len(e.Properties) > 0 {
					args = append(args, p.lowerObjectSpread(propsLoc, &js_ast.EObject{
						Properties:   e.Properties,
						IsSingleLine: e.IsTagSingleLine,
					}))
				} else {
					args = append(args, js_ast.Expr{Loc: propsLoc, Data: js_ast.ENullShared})
				}
				if len(children) > 0 {
					args = append(args, children...)
				}

				// Call createElement()
				var target js_ast.Expr
				kind := js_ast.NormalCall
				if p.options.jsx.AutomaticRuntime {
					target = p.importJSXSymbol(expr.Loc, JSXImportCreateElement)
				} else {
					target = p.instantiateDefineExpr(expr.Loc, p.options.jsx.Factory, identifierOpts{
						wasOriginallyIdentifier: true,
						matchAgainstDefines:     true, // Allow defines to rewrite the JSX factory
					})
					if js_ast.IsPropertyAccess(target) {
						kind = js_ast.TargetWasOriginallyPropertyAccess
					}
					p.warnAboutImportNamespaceCall(target, exprKindCall)
				}
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
					Target:        target,
					Args:          args,
					CloseParenLoc: e.CloseLoc,
					IsMultiLine:   !e.IsTagSingleLine,
					Kind:          kind,

					// Enable tree shaking
					CanBeUnwrappedIfUnused: !p.options.ignoreDCEAnnotations && !p.options.jsx.SideEffects,
				}}, exprOut{}
			} else {
				// Arguments to jsx()
				args := []js_ast.Expr{e.TagOrNil}

				// Props argument
				properties := make([]js_ast.Property, 0, len(e.Properties)+1)

				// For jsx(), "key" is passed in as a separate argument, so filter it out
				// from the props here. Also, check for __source and __self, which might have
				// been added by some upstream plugin. Their presence here would represent a
				// configuration error.
				hasKey := false
				keyProperty := js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared}
				for _, property := range e.Properties {
					if str, ok := property.Key.Data.(*js_ast.EString); ok {
						propName := helpers.UTF16ToString(str.Value)
						switch propName {
						case "key":
							if boolean, ok := property.ValueOrNil.Data.(*js_ast.EBoolean); ok && boolean.Value && property.Flags.Has(js_ast.PropertyWasShorthand) {
								r := js_lexer.RangeOfIdentifier(p.source, property.Loc)
								msg := logger.Msg{
									Kind:  logger.Error,
									Data:  p.tracker.MsgData(r, "Please provide an explicit value for \"key\":"),
									Notes: []logger.MsgData{{Text: "Using \"key\" as a shorthand for \"key={true}\" is not allowed when using React's \"automatic\" JSX transform."}},
								}
								msg.Data.Location.Suggestion = "key={true}"
								p.log.AddMsg(msg)
							} else {
								keyProperty = property.ValueOrNil
								hasKey = true
							}
							continue

						case "__source", "__self":
							r := js_lexer.RangeOfIdentifier(p.source, property.Loc)
							p.log.AddErrorWithNotes(&p.tracker, r,
								fmt.Sprintf("Duplicate \"%s\" prop found:", propName),
								[]logger.MsgData{{Text: "Both \"__source\" and \"__self\" are set automatically by esbuild when using React's \"automatic\" JSX transform. " +
									"This duplicate prop may have come from a plugin."}})
							continue
						}
					}
					properties = append(properties, property)
				}

				isStaticChildren := len(children) > 1

				// Children are passed in as an explicit prop
				if len(children) > 0 {
					childrenValue := children[0]

					if len(children) > 1 {
						childrenValue.Data = &js_ast.EArray{Items: children}
					} else if _, ok := childrenValue.Data.(*js_ast.ESpread); ok {
						// TypeScript considers spread children to be static, but Babel considers
						// it to be an error ("Spread children are not supported in React.").
						// We'll follow TypeScript's behavior here because spread children may be
						// valid with non-React source runtimes.
						childrenValue.Data = &js_ast.EArray{Items: []js_ast.Expr{childrenValue}}
						isStaticChildren = true
					}

					properties = append(properties, js_ast.Property{
						Key: js_ast.Expr{
							Data: &js_ast.EString{Value: helpers.StringToUTF16("children")},
							Loc:  childrenValue.Loc,
						},
						ValueOrNil: childrenValue,
						Kind:       js_ast.PropertyField,
						Loc:        childrenValue.Loc,
					})
				}

				args = append(args, p.lowerObjectSpread(propsLoc, &js_ast.EObject{
					Properties:   properties,
					IsSingleLine: e.IsTagSingleLine,
				}))

				// "key"
				if hasKey || p.options.jsx.Development {
					args = append(args, keyProperty)
				}

				if p.options.jsx.Development {
					// "isStaticChildren"
					args = append(args, js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: isStaticChildren}})

					// "__source"
					args = append(args, js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EObject{
						Properties: []js_ast.Property{
							{
								Kind:       js_ast.PropertyField,
								Key:        js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16("fileName")}},
								ValueOrNil: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(p.source.PrettyPath)}},
							},
							{
								Kind:       js_ast.PropertyField,
								Key:        js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16("lineNumber")}},
								ValueOrNil: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(jsxSourceLine + 1)}}, // 1-based lines
							},
							{
								Kind:       js_ast.PropertyField,
								Key:        js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16("columnNumber")}},
								ValueOrNil: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(jsxSourceColumn + 1)}}, // 1-based columns
							},
						},
					}})

					// "__self"
					__self := js_ast.Expr{Loc: expr.Loc, Data: js_ast.EThisShared}
					{
						if p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef {
							// Substitute "this" if we're inside a static class context
							p.recordUsage(*p.fnOnlyDataVisit.innerClassNameRef)
							__self.Data = &js_ast.EIdentifier{Ref: *p.fnOnlyDataVisit.innerClassNameRef}
						} else if !p.fnOnlyDataVisit.isThisNested && p.options.mode != config.ModePassThrough {
							// Replace top-level "this" with "undefined" if there's an output format
							__self.Data = js_ast.EUndefinedShared
						} else if p.fnOrArrowDataVisit.isDerivedClassCtor {
							// We can't use "this" here in case it comes before "super()"
							__self.Data = js_ast.EUndefinedShared
						}
					}
					if _, ok := __self.Data.(*js_ast.EUndefined); !ok {
						// Omit "__self" entirely if it's undefined
						args = append(args, __self)
					}
				}

				jsx := JSXImportJSX
				if isStaticChildren {
					jsx = JSXImportJSXS
				}

				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
					Target:        p.importJSXSymbol(expr.Loc, jsx),
					Args:          args,
					CloseParenLoc: e.CloseLoc,
					IsMultiLine:   !e.IsTagSingleLine,

					// Enable tree shaking
					CanBeUnwrappedIfUnused: !p.options.ignoreDCEAnnotations && !p.options.jsx.SideEffects,
				}}, exprOut{}
			}
		}

	case *js_ast.ETemplate:
		if e.LegacyOctalLoc.Start > 0 {
			p.log.AddError(&p.tracker, p.source.RangeOfLegacyOctalEscape(e.LegacyOctalLoc),
				"Legacy octal escape sequences cannot be used in template literals")
		}

		var tagThisFunc func() js_ast.Expr
		var tagWrapFunc func(js_ast.Expr) js_ast.Expr

		if e.TagOrNil.Data != nil {
			// Capture the value for "this" if the tag is a lowered optional chain.
			// We'll need to manually apply this value later to preserve semantics.
			tagIsLoweredOptionalChain := false
			if p.options.unsupportedJSFeatures.Has(compat.OptionalChain) {
				switch target := e.TagOrNil.Data.(type) {
				case *js_ast.EDot:
					tagIsLoweredOptionalChain = target.OptionalChain != js_ast.OptionalChainNone
				case *js_ast.EIndex:
					tagIsLoweredOptionalChain = target.OptionalChain != js_ast.OptionalChainNone
				}
			}

			p.templateTag = e.TagOrNil.Data
			tag, tagOut := p.visitExprInOut(e.TagOrNil, exprIn{storeThisArgForParentOptionalChain: tagIsLoweredOptionalChain})
			e.TagOrNil = tag
			tagThisFunc = tagOut.thisArgFunc
			tagWrapFunc = tagOut.thisArgWrapFunc

			// Copy the call side effect flag over if this is a known target
			if id, ok := tag.Data.(*js_ast.EIdentifier); ok && p.symbols[id.Ref.InnerIndex].Flags.Has(ast.CallCanBeUnwrappedIfUnused) {
				e.CanBeUnwrappedIfUnused = true
			}

			// The value of "this" must be manually preserved for private member
			// accesses inside template tag expressions such as "this.#foo``".
			// The private member "this.#foo" must see the value of "this".
			if target, loc, private := p.extractPrivateIndex(e.TagOrNil); private != nil {
				// "foo.#bar`123`" => "__privateGet(_a = foo, #bar).bind(_a)`123`"
				targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(target.Loc, 2, target, valueCouldBeMutated)
				e.TagOrNil = targetWrapFunc(js_ast.Expr{Loc: target.Loc, Data: &js_ast.ECall{
					Target: js_ast.Expr{Loc: target.Loc, Data: &js_ast.EDot{
						Target:  p.lowerPrivateGet(targetFunc(), loc, private),
						Name:    "bind",
						NameLoc: target.Loc,
					}},
					Args: []js_ast.Expr{targetFunc()},
					Kind: js_ast.TargetWasOriginallyPropertyAccess,
				}})
			}
		}

		for i, part := range e.Parts {
			e.Parts[i].Value = p.visitExpr(part.Value)
		}

		// When mangling, inline string values into the template literal. Note that
		// it may no longer be a template literal after this point (it may turn into
		// a plain string literal instead).
		if p.shouldFoldTypeScriptConstantExpressions || p.options.minifySyntax {
			expr = js_ast.InlinePrimitivesIntoTemplate(expr.Loc, e)
		}

		shouldLowerTemplateLiteral := p.options.unsupportedJSFeatures.Has(compat.TemplateLiteral)

		// If the tag was originally an optional chaining property access, then
		// we'll need to lower this template literal as well to preserve the value
		// for "this".
		if tagThisFunc != nil {
			shouldLowerTemplateLiteral = true
		}

		// Lower tagged template literals that include "</script"
		// since we won't be able to escape it without lowering it
		if !shouldLowerTemplateLiteral && !p.options.unsupportedJSFeatures.Has(compat.InlineScript) && e.TagOrNil.Data != nil {
			if containsClosingScriptTag(e.HeadRaw) {
				shouldLowerTemplateLiteral = true
			} else {
				for _, part := range e.Parts {
					if containsClosingScriptTag(part.TailRaw) {
						shouldLowerTemplateLiteral = true
						break
					}
				}
			}
		}

		// Convert template literals to older syntax if this is still a template literal
		if shouldLowerTemplateLiteral {
			if e, ok := expr.Data.(*js_ast.ETemplate); ok {
				return p.lowerTemplateLiteral(expr.Loc, e, tagThisFunc, tagWrapFunc), exprOut{}
			}
		}

	case *js_ast.EBinary:
		// The handling of binary expressions is convoluted because we're using
		// iteration on the heap instead of recursion on the call stack to avoid
		// stack overflow for deeply-nested ASTs. See the comment before the
		// definition of "binaryExprVisitor" for details.
		v := binaryExprVisitor{
			e:   e,
			loc: expr.Loc,
			in:  in,
		}

		// Everything uses a single stack to reduce allocation overhead. This stack
		// should almost always be very small, and almost all visits should reuse
		// existing memory without allocating anything.
		stackBottom := len(p.binaryExprStack)

		// Iterate down into the AST along the left node of the binary operation.
		// Continue iterating until we encounter something that's not a binary node.
		for {
			// Check whether this node is a special case. If it is, a result will be
			// provided which ends our iteration. Otherwise, the visitor object will
			// be prepared for visiting.
			if result := v.checkAndPrepare(p); result.Data != nil {
				expr = result
				break
			}

			// Grab the arguments to our nested "visitExprInOut" call for the left
			// node. We only care about deeply-nested left nodes because most binary
			// operators in JavaScript are left-associative and the problematic edge
			// cases we're trying to avoid crashing on have lots of left-associative
			// binary operators chained together without parentheses (e.g. "1+2+...").
			left := v.e.Left
			leftIn := v.leftIn
			leftBinary, ok := left.Data.(*js_ast.EBinary)

			// Stop iterating if iteration doesn't apply to the left node. This checks
			// the assignment target because "visitExprInOut" has additional behavior
			// in that case that we don't want to miss (before the top-level "switch"
			// statement).
			if !ok || leftIn.assignTarget != js_ast.AssignTargetNone {
				v.e.Left, _ = p.visitExprInOut(left, leftIn)
				expr = v.visitRightAndFinish(p)
				break
			}

			// Note that we only append to the stack (and therefore allocate memory
			// on the heap) when there are nested binary expressions. A single binary
			// expression doesn't add anything to the stack.
			p.binaryExprStack = append(p.binaryExprStack, v)
			v = binaryExprVisitor{
				e:   leftBinary,
				loc: left.Loc,
				in:  leftIn,
			}
		}

		// Process all binary operations from the deepest-visited node back toward
		// our original top-level binary operation.
		for {
			n := len(p.binaryExprStack) - 1
			if n < stackBottom {
				break
			}
			v := p.binaryExprStack[n]
			p.binaryExprStack = p.binaryExprStack[:n]
			v.e.Left = expr
			expr = v.visitRightAndFinish(p)
		}

		return expr, exprOut{}

	case *js_ast.EDot:
		isDeleteTarget := e == p.deleteTarget
		isCallTarget := e == p.callTarget
		isTemplateTag := e == p.templateTag

		// Check both user-specified defines and known globals
		if defines, ok := p.options.defines.DotDefines[e.Name]; ok {
			for _, define := range defines {
				if p.isDotOrIndexDefineMatch(expr, define.KeyParts) {
					// Substitute user-specified defines
					if define.DefineExpr != nil {
						new := p.instantiateDefineExpr(expr.Loc, *define.DefineExpr, identifierOpts{
							assignTarget:   in.assignTarget,
							isCallTarget:   isCallTarget,
							isDeleteTarget: isDeleteTarget,
						})
						if in.assignTarget == js_ast.AssignTargetNone || defineValueCanBeUsedInAssignTarget(new.Data) {
							// Note: We don't need to "ignoreRef" on the underlying identifier
							// because we have only parsed it but not visited it yet
							return new, exprOut{}
						} else {
							r := logger.Range{Loc: expr.Loc, Len: js_lexer.RangeOfIdentifier(p.source, e.NameLoc).End() - expr.Loc.Start}
							p.logAssignToDefine(r, "", expr)
						}
					}

					// Copy the side effect flags over in case this expression is unused
					if define.Flags.Has(config.CanBeRemovedIfUnused) {
						e.CanBeRemovedIfUnused = true
					}
					if define.Flags.Has(config.CallCanBeUnwrappedIfUnused) && !p.options.ignoreDCEAnnotations {
						e.CallCanBeUnwrappedIfUnused = true
					}
					if define.Flags.Has(config.IsSymbolInstance) {
						e.IsSymbolInstance = true
					}
					break
				}
			}
		}

		// Check injected dot names
		if names, ok := p.injectedDotNames[e.Name]; ok {
			for _, name := range names {
				if p.isDotOrIndexDefineMatch(expr, name.parts) {
					// Note: We don't need to "ignoreRef" on the underlying identifier
					// because we have only parsed it but not visited it yet
					return p.instantiateInjectDotName(expr.Loc, name, in.assignTarget), exprOut{}
				}
			}
		}

		// Track ".then().catch()" chains
		if isCallTarget && p.thenCatchChain.nextTarget == e {
			if e.Name == "catch" {
				p.thenCatchChain = thenCatchChain{
					nextTarget: e.Target.Data,
					hasCatch:   true,
					catchLoc:   e.NameLoc,
				}
			} else if e.Name == "then" {
				p.thenCatchChain = thenCatchChain{
					nextTarget: e.Target.Data,
					hasCatch:   p.thenCatchChain.hasCatch || p.thenCatchChain.hasMultipleArgs,
					catchLoc:   p.thenCatchChain.catchLoc,
				}
			}
		}

		p.dotOrIndexTarget = e.Target.Data
		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == js_ast.OptionalChainContinue,
		})
		e.Target = target

		// Lower "super.prop" if necessary
		if e.OptionalChain == js_ast.OptionalChainNone && in.assignTarget == js_ast.AssignTargetNone &&
			!isCallTarget && p.shouldLowerSuperPropertyAccess(e.Target) {
			// "super.foo" => "__superGet('foo')"
			key := js_ast.Expr{Loc: e.NameLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(e.Name)}}
			value := p.lowerSuperPropertyGet(expr.Loc, key)
			if isTemplateTag {
				value.Data = &js_ast.ECall{
					Target: js_ast.Expr{Loc: value.Loc, Data: &js_ast.EDot{
						Target:  value,
						Name:    "bind",
						NameLoc: value.Loc,
					}},
					Args: []js_ast.Expr{{Loc: value.Loc, Data: js_ast.EThisShared}},
					Kind: js_ast.TargetWasOriginallyPropertyAccess,
				}
			}
			return value, exprOut{}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain == js_ast.OptionalChainStart ||
			(e.OptionalChain == js_ast.OptionalChainContinue && out.childContainsOptionalChain)
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out)
		}

		// Also erase "console.log.call(console, 123)" and "console.log.bind(console)"
		if out.callMustBeReplacedWithUndefined {
			if e.Name == "call" || e.Name == "apply" {
				out.methodCallMustBeReplacedWithUndefined = true
			} else if p.options.unsupportedJSFeatures.Has(compat.Arrow) {
				e.Target.Data = &js_ast.EFunction{}
			} else {
				e.Target.Data = &js_ast.EArrow{}
			}
		}

		// Potentially rewrite this property access
		out = exprOut{
			childContainsOptionalChain:      containsOptionalChain,
			callMustBeReplacedWithUndefined: out.methodCallMustBeReplacedWithUndefined,
			thisArgFunc:                     out.thisArgFunc,
			thisArgWrapFunc:                 out.thisArgWrapFunc,
		}
		if !in.hasChainParent {
			out.thisArgFunc = nil
			out.thisArgWrapFunc = nil
		}
		if e.OptionalChain == js_ast.OptionalChainNone {
			if value, ok := p.maybeRewritePropertyAccess(expr.Loc, in.assignTarget,
				isDeleteTarget, e.Target, e.Name, e.NameLoc, isCallTarget, isTemplateTag, false); ok {
				return value, out
			}
		}
		return js_ast.Expr{Loc: expr.Loc, Data: e}, out

	case *js_ast.EIndex:
		isCallTarget := e == p.callTarget
		isTemplateTag := e == p.templateTag
		isDeleteTarget := e == p.deleteTarget

		// Check both user-specified defines and known globals
		if str, ok := e.Index.Data.(*js_ast.EString); ok {
			if defines, ok := p.options.defines.DotDefines[helpers.UTF16ToString(str.Value)]; ok {
				for _, define := range defines {
					if p.isDotOrIndexDefineMatch(expr, define.KeyParts) {
						// Substitute user-specified defines
						if define.DefineExpr != nil {
							new := p.instantiateDefineExpr(expr.Loc, *define.DefineExpr, identifierOpts{
								assignTarget:   in.assignTarget,
								isCallTarget:   isCallTarget,
								isDeleteTarget: isDeleteTarget,
							})
							if in.assignTarget == js_ast.AssignTargetNone || defineValueCanBeUsedInAssignTarget(new.Data) {
								// Note: We don't need to "ignoreRef" on the underlying identifier
								// because we have only parsed it but not visited it yet
								return new, exprOut{}
							} else {
								r := logger.Range{Loc: expr.Loc}
								afterIndex := logger.Loc{Start: p.source.RangeOfString(e.Index.Loc).End()}
								if closeBracket := p.source.RangeOfOperatorAfter(afterIndex, "]"); closeBracket.Len > 0 {
									r.Len = closeBracket.End() - r.Loc.Start
								}
								p.logAssignToDefine(r, "", expr)
							}
						}

						// Copy the side effect flags over in case this expression is unused
						if define.Flags.Has(config.CanBeRemovedIfUnused) {
							e.CanBeRemovedIfUnused = true
						}
						if define.Flags.Has(config.CallCanBeUnwrappedIfUnused) && !p.options.ignoreDCEAnnotations {
							e.CallCanBeUnwrappedIfUnused = true
						}
						if define.Flags.Has(config.IsSymbolInstance) {
							e.IsSymbolInstance = true
						}
						break
					}
				}
			}
		}

		// "a['b']" => "a.b"
		if p.options.minifySyntax {
			if str, ok := e.Index.Data.(*js_ast.EString); ok && js_ast.IsIdentifierUTF16(str.Value) {
				dot := p.dotOrMangledPropParse(e.Target, js_lexer.MaybeSubstring{String: helpers.UTF16ToString(str.Value)}, e.Index.Loc, e.OptionalChain, wasOriginallyIndex)
				if isCallTarget {
					p.callTarget = dot
				}
				if isTemplateTag {
					p.templateTag = dot
				}
				if isDeleteTarget {
					p.deleteTarget = dot
				}
				return p.visitExprInOut(js_ast.Expr{Loc: expr.Loc, Data: dot}, in)
			}
		}

		p.dotOrIndexTarget = e.Target.Data
		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == js_ast.OptionalChainContinue,
		})
		e.Target = target

		// Special-case private identifiers
		if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok {
			name := p.loadNameFromRef(private.Ref)
			result := p.findSymbol(e.Index.Loc, name)
			private.Ref = result.ref

			// Unlike regular identifiers, there are no unbound private identifiers
			kind := p.symbols[result.ref.InnerIndex].Kind
			if !kind.IsPrivate() {
				r := logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
				p.log.AddError(&p.tracker, r, fmt.Sprintf("Private name %q must be declared in an enclosing class", name))
			} else {
				var r logger.Range
				var text string
				if in.assignTarget != js_ast.AssignTargetNone && (kind == ast.SymbolPrivateMethod || kind == ast.SymbolPrivateStaticMethod) {
					r = logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
					text = fmt.Sprintf("Writing to read-only method %q will throw", name)
				} else if in.assignTarget != js_ast.AssignTargetNone && (kind == ast.SymbolPrivateGet || kind == ast.SymbolPrivateStaticGet) {
					r = logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
					text = fmt.Sprintf("Writing to getter-only property %q will throw", name)
				} else if in.assignTarget != js_ast.AssignTargetReplace && (kind == ast.SymbolPrivateSet || kind == ast.SymbolPrivateStaticSet) {
					r = logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
					text = fmt.Sprintf("Reading from setter-only property %q will throw", name)
				}
				if text != "" {
					kind := logger.Warning
					if p.suppressWarningsAboutWeirdCode {
						kind = logger.Debug
					}
					p.log.AddID(logger.MsgID_JS_PrivateNameWillThrow, kind, &p.tracker, r, text)
				}
			}

			// Lower private member access only if we're sure the target isn't needed
			// for the value of "this" for a call expression. All other cases will be
			// taken care of by the enclosing call expression.
			if p.privateSymbolNeedsToBeLowered(private) && e.OptionalChain == js_ast.OptionalChainNone &&
				in.assignTarget == js_ast.AssignTargetNone && !isCallTarget && !isTemplateTag {
				// "foo.#bar" => "__privateGet(foo, #bar)"
				return p.lowerPrivateGet(e.Target, e.Index.Loc, private), exprOut{}
			}
		} else {
			e.Index, _ = p.visitExprInOut(e.Index, exprIn{
				shouldMangleStringsAsProps: true,
			})
		}

		// Lower "super[prop]" if necessary
		if e.OptionalChain == js_ast.OptionalChainNone && in.assignTarget == js_ast.AssignTargetNone &&
			!isCallTarget && p.shouldLowerSuperPropertyAccess(e.Target) {
			// "super[foo]" => "__superGet(foo)"
			value := p.lowerSuperPropertyGet(expr.Loc, e.Index)
			if isTemplateTag {
				value.Data = &js_ast.ECall{
					Target: js_ast.Expr{Loc: value.Loc, Data: &js_ast.EDot{
						Target:  value,
						Name:    "bind",
						NameLoc: value.Loc,
					}},
					Args: []js_ast.Expr{{Loc: value.Loc, Data: js_ast.EThisShared}},
					Kind: js_ast.TargetWasOriginallyPropertyAccess,
				}
			}
			return value, exprOut{}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain == js_ast.OptionalChainStart ||
			(e.OptionalChain == js_ast.OptionalChainContinue && out.childContainsOptionalChain)
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out)
		}

		// Potentially rewrite this property access
		out = exprOut{
			childContainsOptionalChain:      containsOptionalChain,
			callMustBeReplacedWithUndefined: out.methodCallMustBeReplacedWithUndefined,
			thisArgFunc:                     out.thisArgFunc,
			thisArgWrapFunc:                 out.thisArgWrapFunc,
		}
		if !in.hasChainParent {
			out.thisArgFunc = nil
			out.thisArgWrapFunc = nil
		}
		if str, ok := e.Index.Data.(*js_ast.EString); ok && e.OptionalChain == js_ast.OptionalChainNone {
			preferQuotedKey := !p.options.minifySyntax
			if value, ok := p.maybeRewritePropertyAccess(expr.Loc, in.assignTarget, isDeleteTarget,
				e.Target, helpers.UTF16ToString(str.Value), e.Index.Loc, isCallTarget, isTemplateTag, preferQuotedKey); ok {
				return value, out
			}
		}

		// Create an error for assigning to an import namespace when bundling. Even
		// though this is a run-time error, we make it a compile-time error when
		// bundling because scope hoisting means these will no longer be run-time
		// errors.
		if p.options.mode == config.ModeBundle && (in.assignTarget != js_ast.AssignTargetNone || isDeleteTarget) {
			if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok && p.symbols[id.Ref.InnerIndex].Kind == ast.SymbolImport {
				r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
				p.log.AddErrorWithNotes(&p.tracker, r,
					fmt.Sprintf("Cannot assign to property on import %q", p.symbols[id.Ref.InnerIndex].OriginalName),
					[]logger.MsgData{{Text: "Imports are immutable in JavaScript. " +
						"To modify the value of this import, you must export a setter function in the " +
						"imported file and then import and call that function here instead."}})

			}
		}

		if p.options.minifySyntax {
			switch index := e.Index.Data.(type) {
			case *js_ast.EString:
				// "a['x' + 'y']" => "a.xy" (this is done late to allow for constant folding)
				if js_ast.IsIdentifierUTF16(index.Value) {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EDot{
						Target:                     e.Target,
						Name:                       helpers.UTF16ToString(index.Value),
						NameLoc:                    e.Index.Loc,
						OptionalChain:              e.OptionalChain,
						CanBeRemovedIfUnused:       e.CanBeRemovedIfUnused,
						CallCanBeUnwrappedIfUnused: e.CallCanBeUnwrappedIfUnused,
					}}, out
				}

				// "a['123']" => "a[123]" (this is done late to allow "'123'" to be mangled)
				if numberValue, ok := js_ast.StringToEquivalentNumberValue(index.Value); ok {
					e.Index.Data = &js_ast.ENumber{Value: numberValue}
				}

			case *js_ast.ENumber:
				// "'abc'[1]" => "'b'"
				if target, ok := e.Target.Data.(*js_ast.EString); ok {
					if intValue := math.Floor(index.Value); index.Value == intValue && intValue >= 0 && intValue < float64(len(target.Value)) {
						return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: []uint16{target.Value[int(intValue)]}}}, out
					}
				}
			}
		}

		return js_ast.Expr{Loc: expr.Loc, Data: e}, out

	case *js_ast.EUnary:
		switch e.Op {
		case js_ast.UnOpTypeof:
			e.Value, _ = p.visitExprInOut(e.Value, exprIn{assignTarget: e.Op.UnaryAssignTarget()})

			// Compile-time "typeof" evaluation
			if typeof, ok := js_ast.TypeofWithoutSideEffects(e.Value.Data); ok {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(typeof)}}, exprOut{}
			}

		case js_ast.UnOpDelete:
			// Warn about code that tries to do "delete super.foo"
			var superPropLoc logger.Loc
			switch e2 := e.Value.Data.(type) {
			case *js_ast.EDot:
				if _, ok := e2.Target.Data.(*js_ast.ESuper); ok {
					superPropLoc = e2.Target.Loc
				}
			case *js_ast.EIndex:
				if _, ok := e2.Target.Data.(*js_ast.ESuper); ok {
					superPropLoc = e2.Target.Loc
				}
			case *js_ast.EIdentifier:
				p.markStrictModeFeature(deleteBareName, js_lexer.RangeOfIdentifier(p.source, e.Value.Loc), "")
			}
			if superPropLoc.Start != 0 {
				r := js_lexer.RangeOfIdentifier(p.source, superPropLoc)
				text := "Attempting to delete a property of \"super\" will throw a ReferenceError"
				kind := logger.Warning
				if p.suppressWarningsAboutWeirdCode {
					kind = logger.Debug
				}
				p.log.AddID(logger.MsgID_JS_DeleteSuperProperty, kind, &p.tracker, r, text)
			}

			p.deleteTarget = e.Value.Data
			value, out := p.visitExprInOut(e.Value, exprIn{hasChainParent: true})
			e.Value = value

			// Lower optional chaining if present since we're guaranteed to be the
			// end of the chain
			if out.childContainsOptionalChain {
				return p.lowerOptionalChain(expr, in, out)
			}

		default:
			e.Value, _ = p.visitExprInOut(e.Value, exprIn{assignTarget: e.Op.UnaryAssignTarget()})

			// Post-process the unary expression
			switch e.Op {
			case js_ast.UnOpNot:
				if p.options.minifySyntax {
					e.Value = p.astHelpers.SimplifyBooleanExpr(e.Value)
				}

				if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(e.Value.Data); ok && sideEffects == js_ast.NoSideEffects {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: !boolean}}, exprOut{}
				}

				if p.options.minifySyntax {
					if result, ok := js_ast.MaybeSimplifyNot(e.Value); ok {
						return result, exprOut{}
					}
				}

			case js_ast.UnOpVoid:
				var shouldRemove bool
				if p.options.minifySyntax {
					shouldRemove = p.astHelpers.ExprCanBeRemovedIfUnused(e.Value)
				} else {
					// This special case was added for a very obscure reason. There's a
					// custom dialect of JavaScript called Svelte that uses JavaScript
					// syntax with different semantics. Specifically variable accesses
					// have side effects (!). And someone wants to use "void x" instead
					// of just "x" to trigger the side effect for some reason.
					//
					// Arguably this should not be supported, because you shouldn't be
					// running esbuild on weird kinda-JavaScript-but-not languages and
					// expecting it to work correctly. But this one special case seems
					// harmless enough. This is definitely not fully supported though.
					//
					// More info: https://github.com/evanw/esbuild/issues/4041
					shouldRemove = isUnsightlyPrimitive(e.Value.Data)
				}
				if shouldRemove {
					return js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared}, exprOut{}
				}

			case js_ast.UnOpPos:
				if number, ok := js_ast.ToNumberWithoutSideEffects(e.Value.Data); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: number}}, exprOut{}
				}

			case js_ast.UnOpNeg:
				if number, ok := js_ast.ToNumberWithoutSideEffects(e.Value.Data); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: -number}}, exprOut{}
				}

			case js_ast.UnOpCpl:
				if p.shouldFoldTypeScriptConstantExpressions || p.options.minifySyntax {
					// Minification folds complement operations since they are unlikely to result in larger output
					if number, ok := js_ast.ToNumberWithoutSideEffects(e.Value.Data); ok {
						return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(^js_ast.ToInt32(number))}}, exprOut{}
					}
				}

				////////////////////////////////////////////////////////////////////////////////
				// All assignment operators below here

			case js_ast.UnOpPreDec, js_ast.UnOpPreInc, js_ast.UnOpPostDec, js_ast.UnOpPostInc:
				if target, loc, private := p.extractPrivateIndex(e.Value); private != nil {
					return p.lowerPrivateSetUnOp(target, loc, private, e.Op), exprOut{}
				}
				if property := p.extractSuperProperty(e.Value); property.Data != nil {
					e.Value = p.callSuperPropertyWrapper(expr.Loc, property)
				}
			}
		}

		// "-(a, b)" => "a, -b"
		if p.options.minifySyntax && e.Op != js_ast.UnOpDelete && e.Op != js_ast.UnOpTypeof {
			if comma, ok := e.Value.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
				return js_ast.JoinWithComma(comma.Left, js_ast.Expr{
					Loc: comma.Right.Loc,
					Data: &js_ast.EUnary{
						Op:    e.Op,
						Value: comma.Right,
					},
				}), exprOut{}
			}
		}

	case *js_ast.EIf:
		e.Test = p.visitExpr(e.Test)

		if p.options.minifySyntax {
			e.Test = p.astHelpers.SimplifyBooleanExpr(e.Test)
		}

		// Propagate these flags into the branches
		childIn := exprIn{
			shouldMangleStringsAsProps: in.shouldMangleStringsAsProps,
		}

		// Fold constants
		if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(e.Test.Data); !ok {
			e.Yes, _ = p.visitExprInOut(e.Yes, childIn)
			e.No, _ = p.visitExprInOut(e.No, childIn)
		} else {
			// Mark the control flow as dead if the branch is never taken
			if boolean {
				// "true ? live : dead"
				e.Yes, _ = p.visitExprInOut(e.Yes, childIn)
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.No, _ = p.visitExprInOut(e.No, childIn)
				p.isControlFlowDead = old

				if p.options.minifySyntax {
					// "(a, true) ? b : c" => "a, b"
					if sideEffects == js_ast.CouldHaveSideEffects {
						return js_ast.JoinWithComma(p.astHelpers.SimplifyUnusedExpr(e.Test, p.options.unsupportedJSFeatures), e.Yes), exprOut{}
					}

					return e.Yes, exprOut{}
				}
			} else {
				// "false ? dead : live"
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.Yes, _ = p.visitExprInOut(e.Yes, childIn)
				p.isControlFlowDead = old
				e.No, _ = p.visitExprInOut(e.No, childIn)

				if p.options.minifySyntax {
					// "(a, false) ? b : c" => "a, c"
					if sideEffects == js_ast.CouldHaveSideEffects {
						return js_ast.JoinWithComma(p.astHelpers.SimplifyUnusedExpr(e.Test, p.options.unsupportedJSFeatures), e.No), exprOut{}
					}

					return e.No, exprOut{}
				}
			}
		}

		if p.options.minifySyntax {
			return p.astHelpers.MangleIfExpr(expr.Loc, e, p.options.unsupportedJSFeatures), exprOut{}
		}

	case *js_ast.EAwait:
		// Silently remove unsupported top-level "await" in dead code branches
		if p.fnOrArrowDataVisit.isOutsideFnOrArrow {
			if p.isControlFlowDead && (p.options.unsupportedJSFeatures.Has(compat.TopLevelAwait) || !p.options.outputFormat.KeepESMImportExportSyntax()) {
				return p.visitExprInOut(e.Value, in)
			} else {
				p.liveTopLevelAwaitKeyword = logger.Range{Loc: expr.Loc, Len: 5}
				p.markSyntaxFeature(compat.TopLevelAwait, logger.Range{Loc: expr.Loc, Len: 5})
			}
		}

		p.awaitTarget = e.Value.Data
		e.Value = p.visitExpr(e.Value)

		// "await" expressions turn into "yield" expressions when lowering
		return p.maybeLowerAwait(expr.Loc, e), exprOut{}

	case *js_ast.EYield:
		if e.ValueOrNil.Data != nil {
			e.ValueOrNil = p.visitExpr(e.ValueOrNil)
		}

		// "yield* x" turns into "yield* __yieldStar(x)" when lowering async generator functions
		if e.IsStar && p.options.unsupportedJSFeatures.Has(compat.AsyncGenerator) && p.fnOrArrowDataVisit.isGenerator {
			e.ValueOrNil = p.callRuntime(expr.Loc, "__yieldStar", []js_ast.Expr{e.ValueOrNil})
		}

	case *js_ast.EArray:
		if in.assignTarget != js_ast.AssignTargetNone {
			if e.CommaAfterSpread.Start != 0 {
				p.log.AddError(&p.tracker, logger.Range{Loc: e.CommaAfterSpread, Len: 1}, "Unexpected \",\" after rest pattern")
			}
			p.markSyntaxFeature(compat.Destructuring, logger.Range{Loc: expr.Loc, Len: 1})
		}
		hasSpread := false
		for i, item := range e.Items {
			switch e2 := item.Data.(type) {
			case *js_ast.EMissing:
			case *js_ast.ESpread:
				e2.Value, _ = p.visitExprInOut(e2.Value, exprIn{assignTarget: in.assignTarget})
				hasSpread = true
			case *js_ast.EBinary:
				if in.assignTarget != js_ast.AssignTargetNone && e2.Op == js_ast.BinOpAssign {
					e2.Left, _ = p.visitExprInOut(e2.Left, exprIn{assignTarget: js_ast.AssignTargetReplace})

					// Propagate the name to keep from the binding into the initializer
					if id, ok := e2.Left.Data.(*js_ast.EIdentifier); ok {
						p.nameToKeep = p.symbols[id.Ref.InnerIndex].OriginalName
						p.nameToKeepIsFor = e2.Right.Data
					}

					e2.Right = p.visitExpr(e2.Right)
				} else {
					item, _ = p.visitExprInOut(item, exprIn{assignTarget: in.assignTarget})
				}
			default:
				item, _ = p.visitExprInOut(item, exprIn{assignTarget: in.assignTarget})
			}
			e.Items[i] = item
		}

		// "[1, ...[2, 3], 4]" => "[1, 2, 3, 4]"
		if p.options.minifySyntax && hasSpread && in.assignTarget == js_ast.AssignTargetNone {
			e.Items = js_ast.InlineSpreadsOfArrayLiterals(e.Items)
		}

	case *js_ast.EObject:
		if in.assignTarget != js_ast.AssignTargetNone {
			if e.CommaAfterSpread.Start != 0 {
				p.log.AddError(&p.tracker, logger.Range{Loc: e.CommaAfterSpread, Len: 1}, "Unexpected \",\" after rest pattern")
			}
			p.markSyntaxFeature(compat.Destructuring, logger.Range{Loc: expr.Loc, Len: 1})
		}

		hasSpread := false
		protoRange := logger.Range{}
		innerClassNameRef := ast.InvalidRef

		for i := range e.Properties {
			property := &e.Properties[i]

			if property.Kind != js_ast.PropertySpread {
				key := property.Key
				if mangled, ok := key.Data.(*js_ast.ENameOfSymbol); ok {
					mangled.Ref = p.symbolForMangledProp(p.loadNameFromRef(mangled.Ref))
				} else {
					key, _ = p.visitExprInOut(property.Key, exprIn{
						shouldMangleStringsAsProps: true,
					})
					property.Key = key
				}

				// Forbid duplicate "__proto__" properties according to the specification
				if !property.Flags.Has(js_ast.PropertyIsComputed) && !property.Flags.Has(js_ast.PropertyWasShorthand) &&
					property.Kind == js_ast.PropertyField && in.assignTarget == js_ast.AssignTargetNone {
					if str, ok := key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(str.Value, "__proto__") {
						r := js_lexer.RangeOfIdentifier(p.source, key.Loc)
						if protoRange.Len > 0 {
							p.log.AddErrorWithNotes(&p.tracker, r,
								"Cannot specify the \"__proto__\" property more than once per object",
								[]logger.MsgData{p.tracker.MsgData(protoRange, "The earlier \"__proto__\" property is here:")})
						} else {
							protoRange = r
						}
					}
				}

				// "{['x']: y}" => "{x: y}"
				if p.options.minifySyntax && property.Flags.Has(js_ast.PropertyIsComputed) {
					if inlined, ok := key.Data.(*js_ast.EInlinedEnum); ok {
						switch inlined.Value.Data.(type) {
						case *js_ast.EString, *js_ast.ENumber:
							key.Data = inlined.Value.Data
							property.Key.Data = key.Data
						}
					}
					switch k := key.Data.(type) {
					case *js_ast.ENumber, *js_ast.ENameOfSymbol:
						property.Flags &= ^js_ast.PropertyIsComputed
					case *js_ast.EString:
						if !helpers.UTF16EqualsString(k.Value, "__proto__") {
							property.Flags &= ^js_ast.PropertyIsComputed
						}
					}
				}
			} else {
				hasSpread = true
			}

			// Extract the initializer for expressions like "({ a: b = c } = d)"
			if in.assignTarget != js_ast.AssignTargetNone && property.InitializerOrNil.Data == nil && property.ValueOrNil.Data != nil {
				if binary, ok := property.ValueOrNil.Data.(*js_ast.EBinary); ok && binary.Op == js_ast.BinOpAssign {
					property.InitializerOrNil = binary.Right
					property.ValueOrNil = binary.Left
				}
			}

			if property.ValueOrNil.Data != nil {
				oldIsInStaticClassContext := p.fnOnlyDataVisit.isInStaticClassContext
				oldInnerClassNameRef := p.fnOnlyDataVisit.innerClassNameRef

				// If this is an async method and async methods are unsupported,
				// generate a temporary variable in case this async method contains a
				// "super" property reference. If that happens, the "super" expression
				// must be lowered which will need a reference to this object literal.
				if property.Kind == js_ast.PropertyMethod && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) {
					if fn, ok := property.ValueOrNil.Data.(*js_ast.EFunction); ok && fn.Fn.IsAsync {
						if innerClassNameRef == ast.InvalidRef {
							innerClassNameRef = p.generateTempRef(tempRefNeedsDeclareMayBeCapturedInsideLoop, "")
						}
						p.fnOnlyDataVisit.isInStaticClassContext = true
						p.fnOnlyDataVisit.innerClassNameRef = &innerClassNameRef
					}
				}

				// Propagate the name to keep from the property into the value
				if str, ok := property.Key.Data.(*js_ast.EString); ok {
					p.nameToKeep = helpers.UTF16ToString(str.Value)
					p.nameToKeepIsFor = property.ValueOrNil.Data
				}

				property.ValueOrNil, _ = p.visitExprInOut(property.ValueOrNil, exprIn{
					isMethod:     property.Kind.IsMethodDefinition(),
					assignTarget: in.assignTarget,
				})

				p.fnOnlyDataVisit.innerClassNameRef = oldInnerClassNameRef
				p.fnOnlyDataVisit.isInStaticClassContext = oldIsInStaticClassContext
			}

			if property.InitializerOrNil.Data != nil {
				// Propagate the name to keep from the binding into the initializer
				if id, ok := property.ValueOrNil.Data.(*js_ast.EIdentifier); ok {
					p.nameToKeep = p.symbols[id.Ref.InnerIndex].OriginalName
					p.nameToKeepIsFor = property.InitializerOrNil.Data
				}

				property.InitializerOrNil = p.visitExpr(property.InitializerOrNil)
			}

			// "{ '123': 4 }" => "{ 123: 4 }" (this is done late to allow "'123'" to be mangled)
			if p.options.minifySyntax {
				if str, ok := property.Key.Data.(*js_ast.EString); ok {
					if numberValue, ok := js_ast.StringToEquivalentNumberValue(str.Value); ok && numberValue >= 0 {
						property.Key.Data = &js_ast.ENumber{Value: numberValue}
					}
				}
			}
		}

		// Check for and warn about duplicate keys in object literals
		if !p.suppressWarningsAboutWeirdCode {
			p.warnAboutDuplicateProperties(e.Properties, duplicatePropertiesInObject)
		}

		if in.assignTarget == js_ast.AssignTargetNone {
			// "{a, ...{b, c}, d}" => "{a, b, c, d}"
			if p.options.minifySyntax && hasSpread {
				e.Properties = js_ast.MangleObjectSpread(e.Properties)
			}

			// Object expressions represent both object literals and binding patterns.
			// Only lower object spread if we're an object literal, not a binding pattern.
			value := p.lowerObjectSpread(expr.Loc, e)

			// If we generated and used the temporary variable for a lowered "super"
			// property reference inside a lowered "async" method, then initialize
			// the temporary with this object literal.
			if innerClassNameRef != ast.InvalidRef && p.symbols[innerClassNameRef.InnerIndex].UseCountEstimate > 0 {
				p.recordUsage(innerClassNameRef)
				value = js_ast.Assign(js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: innerClassNameRef}}, value)
			}

			return value, exprOut{}
		}

	case *js_ast.EImportCall:
		isAwaitTarget := e == p.awaitTarget
		isThenCatchTarget := e == p.thenCatchChain.nextTarget && p.thenCatchChain.hasCatch
		e.Expr = p.visitExpr(e.Expr)

		var assertOrWith *ast.ImportAssertOrWith
		var flags ast.ImportRecordFlags
		if e.OptionsOrNil.Data != nil {
			e.OptionsOrNil = p.visitExpr(e.OptionsOrNil)

			// If there's an additional argument, this can't be split because the
			// additional argument requires evaluation and our AST nodes can't be
			// reused in different places in the AST (e.g. function scopes must be
			// unique). Also the additional argument may have side effects and we
			// don't currently account for that.
			why := "the second argument was not an object literal"
			whyLoc := e.OptionsOrNil.Loc

			// However, make a special case for an additional argument that contains
			// only an "assert" or a "with" clause. In that case we can split this
			// AST node.
			if object, ok := e.OptionsOrNil.Data.(*js_ast.EObject); ok {
				if len(object.Properties) == 1 {
					if prop := object.Properties[0]; prop.Kind == js_ast.PropertyField && !prop.Flags.Has(js_ast.PropertyIsComputed) {
						if str, ok := prop.Key.Data.(*js_ast.EString); ok && (helpers.UTF16EqualsString(str.Value, "assert") || helpers.UTF16EqualsString(str.Value, "with")) {
							keyword := ast.WithKeyword
							if helpers.UTF16EqualsString(str.Value, "assert") {
								keyword = ast.AssertKeyword
							}
							if value, ok := prop.ValueOrNil.Data.(*js_ast.EObject); ok {
								entries := []ast.AssertOrWithEntry{}
								for _, p := range value.Properties {
									if p.Kind == js_ast.PropertyField && !p.Flags.Has(js_ast.PropertyIsComputed) {
										if key, ok := p.Key.Data.(*js_ast.EString); ok {
											if value, ok := p.ValueOrNil.Data.(*js_ast.EString); ok {
												entries = append(entries, ast.AssertOrWithEntry{
													Key:             key.Value,
													KeyLoc:          p.Key.Loc,
													Value:           value.Value,
													ValueLoc:        p.ValueOrNil.Loc,
													PreferQuotedKey: p.Flags.Has(js_ast.PropertyPreferQuotedKey),
												})
												if keyword == ast.AssertKeyword && helpers.UTF16EqualsString(key.Value, "type") && helpers.UTF16EqualsString(value.Value, "json") {
													flags |= ast.AssertTypeJSON
												}
												continue
											} else {
												why = fmt.Sprintf("the value for the property %q was not a string literal",
													helpers.UTF16ToString(key.Value))
												whyLoc = p.ValueOrNil.Loc
											}
										} else {
											why = "this property was not a string literal"
											whyLoc = p.Key.Loc
										}
									} else {
										why = "this property was invalid"
										whyLoc = p.Key.Loc
									}
									entries = nil
									break
								}
								if entries != nil {
									if keyword == ast.AssertKeyword {
										p.maybeWarnAboutAssertKeyword(prop.Key.Loc)
									}
									assertOrWith = &ast.ImportAssertOrWith{
										Entries:            entries,
										Keyword:            keyword,
										KeywordLoc:         prop.Key.Loc,
										InnerOpenBraceLoc:  prop.ValueOrNil.Loc,
										InnerCloseBraceLoc: value.CloseBraceLoc,
										OuterOpenBraceLoc:  e.OptionsOrNil.Loc,
										OuterCloseBraceLoc: object.CloseBraceLoc,
									}
									why = ""
								}
							} else {
								why = "the value for \"assert\" was not an object literal"
								whyLoc = prop.ValueOrNil.Loc
							}
						} else {
							why = "this property was not called \"assert\" or \"with\""
							whyLoc = prop.Key.Loc
						}
					} else {
						why = "this property was invalid"
						whyLoc = prop.Key.Loc
					}
				} else {
					why = "the second argument was not an object literal with a single property called \"assert\" or \"with\""
					whyLoc = e.OptionsOrNil.Loc
				}
			}

			// Handle the case that isn't just an import assertion or attribute clause
			if why != "" {
				// Only warn when bundling
				if p.options.mode == config.ModeBundle {
					text := "This \"import()\" was not recognized because " + why
					kind := logger.Warning
					if p.suppressWarningsAboutWeirdCode {
						kind = logger.Debug
					}
					p.log.AddID(logger.MsgID_JS_UnsupportedDynamicImport, kind, &p.tracker, logger.Range{Loc: whyLoc}, text)
				}

				// If import assertions and/attributes are both not supported in the
				// target platform, then "import()" cannot accept a second argument
				// and keeping them would be a syntax error, so we need to get rid of
				// them. We can't just not print them because they may have important
				// side effects. Attempt to discard them without changing side effects
				// and generate an error if that isn't possible.
				if p.options.unsupportedJSFeatures.Has(compat.ImportAssertions) && p.options.unsupportedJSFeatures.Has(compat.ImportAttributes) {
					if p.astHelpers.ExprCanBeRemovedIfUnused(e.OptionsOrNil) {
						e.OptionsOrNil = js_ast.Expr{}
					} else {
						p.markSyntaxFeature(compat.ImportAttributes, logger.Range{Loc: e.OptionsOrNil.Loc})
					}
				}

				// Stop now so we don't try to split "?:" expressions below and
				// potentially end up with an AST node reused multiple times
				break
			}
		}

		return p.maybeTransposeIfExprChain(e.Expr, func(arg js_ast.Expr) js_ast.Expr {
			// The argument must be a string
			if str, ok := arg.Data.(*js_ast.EString); ok {
				// Ignore calls to import() if the control flow is provably dead here.
				// We don't want to spend time scanning the required files if they will
				// never be used.
				if p.isControlFlowDead {
					return js_ast.Expr{Loc: arg.Loc, Data: js_ast.ENullShared}
				}

				importRecordIndex := p.addImportRecord(ast.ImportDynamic, p.source.RangeOfString(arg.Loc), helpers.UTF16ToString(str.Value), assertOrWith, flags)
				if isAwaitTarget && p.fnOrArrowDataVisit.tryBodyCount != 0 {
					record := &p.importRecords[importRecordIndex]
					record.Flags |= ast.HandlesImportErrors
					record.ErrorHandlerLoc = p.fnOrArrowDataVisit.tryCatchLoc
				} else if isThenCatchTarget {
					record := &p.importRecords[importRecordIndex]
					record.Flags |= ast.HandlesImportErrors
					record.ErrorHandlerLoc = p.thenCatchChain.catchLoc
				}
				p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EImportString{
					ImportRecordIndex: importRecordIndex,
					CloseParenLoc:     e.CloseParenLoc,
				}}
			}

			// Handle glob patterns
			if p.options.mode == config.ModeBundle {
				if value := p.handleGlobPattern(arg, ast.ImportDynamic, "globImport", assertOrWith); value.Data != nil {
					return value
				}
			}

			// Use a debug log so people can see this if they want to
			r := js_lexer.RangeOfIdentifier(p.source, expr.Loc)
			p.log.AddID(logger.MsgID_JS_UnsupportedDynamicImport, logger.Debug, &p.tracker, r,
				"This \"import\" expression will not be bundled because the argument is not a string literal")

			// We need to convert this into a call to "require()" if ES6 syntax is
			// not supported in the current output format. The full conversion:
			//
			//   Before:
			//     import(foo)
			//
			//   After:
			//     Promise.resolve().then(() => __toESM(require(foo)))
			//
			// This is normally done by the printer since we don't know during the
			// parsing stage whether this module is external or not. However, it's
			// guaranteed to be external if the argument isn't a string. We handle
			// this case here instead of in the printer because both the printer
			// and the linker currently need an import record to handle this case
			// correctly, and you need a string literal to get an import record.
			if p.options.unsupportedJSFeatures.Has(compat.DynamicImport) {
				var then js_ast.Expr
				value := p.callRuntime(arg.Loc, "__toESM", []js_ast.Expr{{Loc: expr.Loc, Data: &js_ast.ECall{
					Target:        p.valueToSubstituteForRequire(expr.Loc),
					Args:          []js_ast.Expr{arg},
					CloseParenLoc: e.CloseParenLoc,
				}}})
				body := js_ast.FnBody{Loc: expr.Loc, Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: expr.Loc, Data: &js_ast.SReturn{ValueOrNil: value}}}}}
				if p.options.unsupportedJSFeatures.Has(compat.Arrow) {
					then = js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: body}}}
				} else {
					then = js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EArrow{Body: body, PreferExpr: true}}
				}
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
					Target: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EDot{
						Target: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
							Target: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EDot{
								Target:  js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: p.makePromiseRef()}},
								Name:    "resolve",
								NameLoc: expr.Loc,
							}},
							Kind: js_ast.TargetWasOriginallyPropertyAccess,
						}},
						Name:    "then",
						NameLoc: expr.Loc,
					}},
					Args: []js_ast.Expr{then},
					Kind: js_ast.TargetWasOriginallyPropertyAccess,
				}}
			}

			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EImportCall{
				Expr:          arg,
				OptionsOrNil:  e.OptionsOrNil,
				CloseParenLoc: e.CloseParenLoc,
			}}
		}), exprOut{}

	case *js_ast.ECall:
		p.callTarget = e.Target.Data

		// Track ".then().catch()" chains
		p.thenCatchChain = thenCatchChain{
			nextTarget:      e.Target.Data,
			hasMultipleArgs: len(e.Args) >= 2,
			hasCatch:        p.thenCatchChain.nextTarget == e && p.thenCatchChain.hasCatch,
			catchLoc:        p.thenCatchChain.catchLoc,
		}
		if p.thenCatchChain.hasMultipleArgs {
			p.thenCatchChain.catchLoc = e.Args[1].Loc
		}

		// Prepare to recognize "require.resolve()" and "Object.create" calls
		couldBeRequireResolve := false
		couldBeObjectCreate := false
		if len(e.Args) == 1 {
			if dot, ok := e.Target.Data.(*js_ast.EDot); ok && dot.OptionalChain == js_ast.OptionalChainNone {
				if p.options.mode != config.ModePassThrough && dot.Name == "resolve" {
					couldBeRequireResolve = true
				} else if dot.Name == "create" {
					couldBeObjectCreate = true
				}
			}
		}

		wasIdentifierBeforeVisit := false
		isParenthesizedOptionalChain := false
		switch e2 := e.Target.Data.(type) {
		case *js_ast.EIdentifier:
			wasIdentifierBeforeVisit = true
		case *js_ast.EDot:
			isParenthesizedOptionalChain = e.OptionalChain == js_ast.OptionalChainNone && e2.OptionalChain != js_ast.OptionalChainNone
		case *js_ast.EIndex:
			isParenthesizedOptionalChain = e.OptionalChain == js_ast.OptionalChainNone && e2.OptionalChain != js_ast.OptionalChainNone
		}
		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == js_ast.OptionalChainContinue,

			// Signal to our child if this is an ECall at the start of an optional
			// chain. If so, the child will need to stash the "this" context for us
			// that we need for the ".call(this, ...args)".
			storeThisArgForParentOptionalChain: e.OptionalChain == js_ast.OptionalChainStart || isParenthesizedOptionalChain,
		})
		e.Target = target
		p.warnAboutImportNamespaceCall(e.Target, exprKindCall)

		hasSpread := false
		oldIsControlFlowDead := p.isControlFlowDead

		// If we're removing this call, don't count any arguments as symbol uses
		if out.callMustBeReplacedWithUndefined {
			if js_ast.IsPropertyAccess(e.Target) {
				p.isControlFlowDead = true
			} else {
				out.callMustBeReplacedWithUndefined = false
			}
		}

		// Visit the arguments
		for i, arg := range e.Args {
			arg = p.visitExpr(arg)
			if _, ok := arg.Data.(*js_ast.ESpread); ok {
				hasSpread = true
			}
			e.Args[i] = arg
		}

		// Mark side-effect free IIFEs with "/* @__PURE__ */"
		if !e.CanBeUnwrappedIfUnused {
			switch target := e.Target.Data.(type) {
			case *js_ast.EArrow:
				if !target.IsAsync && p.iifeCanBeRemovedIfUnused(target.Args, target.Body) {
					e.CanBeUnwrappedIfUnused = true
				}
			case *js_ast.EFunction:
				if !target.Fn.IsAsync && !target.Fn.IsGenerator && p.iifeCanBeRemovedIfUnused(target.Fn.Args, target.Fn.Body) {
					e.CanBeUnwrappedIfUnused = true
				}
			}
		}

		// Our hack for reading Yarn PnP files is implemented here:
		if p.options.decodeHydrateRuntimeStateYarnPnP {
			if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok && p.symbols[id.Ref.InnerIndex].OriginalName == "hydrateRuntimeState" && len(e.Args) >= 1 {
				switch arg := e.Args[0].Data.(type) {
				case *js_ast.EObject:
					// "hydrateRuntimeState(<object literal>)"
					if arg := e.Args[0]; isValidJSON(arg) {
						p.manifestForYarnPnP = arg
					}

				case *js_ast.ECall:
					// "hydrateRuntimeState(JSON.parse(<something>))"
					if len(arg.Args) == 1 {
						if dot, ok := arg.Target.Data.(*js_ast.EDot); ok && dot.Name == "parse" {
							if id, ok := dot.Target.Data.(*js_ast.EIdentifier); ok {
								if symbol := &p.symbols[id.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound && symbol.OriginalName == "JSON" {
									arg := arg.Args[0]
									switch a := arg.Data.(type) {
									case *js_ast.EString:
										// "hydrateRuntimeState(JSON.parse(<string literal>))"
										source := logger.Source{KeyPath: p.source.KeyPath, Contents: helpers.UTF16ToString(a.Value)}
										stringInJSTable := logger.GenerateStringInJSTable(p.source.Contents, arg.Loc, source.Contents)
										log := logger.NewStringInJSLog(p.log, &p.tracker, stringInJSTable)
										p.manifestForYarnPnP, _ = ParseJSON(log, source, JSONOptions{})
										remapExprLocsInJSON(&p.manifestForYarnPnP, stringInJSTable)

									case *js_ast.EIdentifier:
										// "hydrateRuntimeState(JSON.parse(<identifier>))"
										if data, ok := p.stringLocalsForYarnPnP[a.Ref]; ok {
											source := logger.Source{KeyPath: p.source.KeyPath, Contents: helpers.UTF16ToString(data.value)}
											stringInJSTable := logger.GenerateStringInJSTable(p.source.Contents, data.loc, source.Contents)
											log := logger.NewStringInJSLog(p.log, &p.tracker, stringInJSTable)
											p.manifestForYarnPnP, _ = ParseJSON(log, source, JSONOptions{})
											remapExprLocsInJSON(&p.manifestForYarnPnP, stringInJSTable)
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// Stop now if this call must be removed
		if out.callMustBeReplacedWithUndefined {
			p.isControlFlowDead = oldIsControlFlowDead
			return js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared}, exprOut{}
		}

		// "foo(1, ...[2, 3], 4)" => "foo(1, 2, 3, 4)"
		if p.options.minifySyntax && hasSpread {
			e.Args = js_ast.InlineSpreadsOfArrayLiterals(e.Args)
		}

		switch t := target.Data.(type) {
		case *js_ast.EImportIdentifier:
			// If this function is inlined, allow it to be tree-shaken
			if p.options.minifySyntax && !p.isControlFlowDead {
				p.convertSymbolUseToCall(t.Ref, len(e.Args) == 1 && !hasSpread)
			}

		case *js_ast.EIdentifier:
			// Detect if this is a direct eval. Note that "(1 ? eval : 0)(x)" will
			// become "eval(x)" after we visit the target due to dead code elimination,
			// but that doesn't mean it should become a direct eval.
			//
			// Note that "eval?.(x)" is considered an indirect eval. There was debate
			// about this after everyone implemented it as a direct eval, but the
			// language committee said it was indirect and everyone had to change it:
			// https://github.com/tc39/ecma262/issues/2062.
			if e.OptionalChain == js_ast.OptionalChainNone {
				symbol := p.symbols[t.Ref.InnerIndex]
				if wasIdentifierBeforeVisit && symbol.OriginalName == "eval" {
					e.Kind = js_ast.DirectEval

					// Pessimistically assume that if this looks like a CommonJS module
					// (e.g. no "export" keywords), a direct call to "eval" means that
					// code could potentially access "module" or "exports".
					if p.options.mode == config.ModeBundle && !p.isFileConsideredToHaveESMExports {
						p.recordUsage(p.moduleRef)
						p.recordUsage(p.exportsRef)
					}

					// Mark this scope and all parent scopes as containing a direct eval.
					// This will prevent us from renaming any symbols.
					for s := p.currentScope; s != nil; s = s.Parent {
						s.ContainsDirectEval = true
					}

					// Warn when direct eval is used in an ESM file. There is no way we
					// can guarantee that this will work correctly for top-level imported
					// and exported symbols due to scope hoisting. Except don't warn when
					// this code is in a 3rd-party library because there's nothing people
					// will be able to do about the warning.
					text := "Using direct eval with a bundler is not recommended and may cause problems"
					kind := logger.Debug
					if p.options.mode == config.ModeBundle && p.isFileConsideredESM && !p.suppressWarningsAboutWeirdCode {
						kind = logger.Warning
					}
					p.log.AddIDWithNotes(logger.MsgID_JS_DirectEval, kind, &p.tracker, js_lexer.RangeOfIdentifier(p.source, e.Target.Loc), text,
						[]logger.MsgData{{Text: "You can read more about direct eval and bundling here: https://esbuild.github.io/link/direct-eval"}})
				} else if symbol.Flags.Has(ast.CallCanBeUnwrappedIfUnused) {
					// Automatically add a "/* @__PURE__ */" comment to file-local calls
					// of functions declared with a "/* @__NO_SIDE_EFFECTS__ */" comment
					t.CallCanBeUnwrappedIfUnused = true
				}
			}

			// Optimize references to global constructors
			if p.options.minifySyntax && t.CanBeRemovedIfUnused && len(e.Args) <= 1 && !hasSpread {
				if symbol := &p.symbols[t.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound {
					// Note: We construct expressions by assigning to "expr.Data" so
					// that the source map position for the constructor is preserved
					switch symbol.OriginalName {
					case "Boolean":
						if len(e.Args) == 0 {
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: false}}, exprOut{}
						} else {
							expr.Data = &js_ast.EUnary{Value: p.astHelpers.SimplifyBooleanExpr(e.Args[0]), Op: js_ast.UnOpNot}
							return js_ast.Not(expr), exprOut{}
						}

					case "Number":
						if len(e.Args) == 0 {
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: 0}}, exprOut{}
						} else {
							arg := e.Args[0]

							switch js_ast.KnownPrimitiveType(arg.Data) {
							case js_ast.PrimitiveNumber:
								return arg, exprOut{}

							case
								js_ast.PrimitiveUndefined, // NaN
								js_ast.PrimitiveNull,      // 0
								js_ast.PrimitiveBoolean,   // 0 or 1
								js_ast.PrimitiveString:    // StringToNumber
								if number, ok := js_ast.ToNumberWithoutSideEffects(arg.Data); ok {
									expr.Data = &js_ast.ENumber{Value: number}
								} else {
									expr.Data = &js_ast.EUnary{Value: arg, Op: js_ast.UnOpPos}
								}
								return expr, exprOut{}
							}
						}

					case "String":
						if len(e.Args) == 0 {
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: nil}}, exprOut{}
						} else {
							arg := e.Args[0]

							switch js_ast.KnownPrimitiveType(arg.Data) {
							case js_ast.PrimitiveString:
								return arg, exprOut{}
							}
						}

					case "BigInt":
						if len(e.Args) == 1 {
							arg := e.Args[0]

							switch js_ast.KnownPrimitiveType(arg.Data) {
							case js_ast.PrimitiveBigInt:
								return arg, exprOut{}
							}
						}
					}
				}
			}

			// Copy the call side effect flag over if this is a known target
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}

			// If this function is inlined, allow it to be tree-shaken
			if p.options.minifySyntax && !p.isControlFlowDead {
				p.convertSymbolUseToCall(t.Ref, len(e.Args) == 1 && !hasSpread)
			}

		case *js_ast.EDot:
			// Recognize "require.resolve()" calls
			if couldBeRequireResolve && t.Name == "resolve" {
				if id, ok := t.Target.Data.(*js_ast.EIdentifier); ok && id.Ref == p.requireRef {
					p.ignoreUsage(p.requireRef)
					return p.maybeTransposeIfExprChain(e.Args[0], func(arg js_ast.Expr) js_ast.Expr {
						if str, ok := e.Args[0].Data.(*js_ast.EString); ok {
							// Ignore calls to require.resolve() if the control flow is provably
							// dead here. We don't want to spend time scanning the required files
							// if they will never be used.
							if p.isControlFlowDead {
								return js_ast.Expr{Loc: expr.Loc, Data: js_ast.ENullShared}
							}

							importRecordIndex := p.addImportRecord(ast.ImportRequireResolve, p.source.RangeOfString(e.Args[0].Loc), helpers.UTF16ToString(str.Value), nil, 0)
							if p.fnOrArrowDataVisit.tryBodyCount != 0 {
								record := &p.importRecords[importRecordIndex]
								record.Flags |= ast.HandlesImportErrors
								record.ErrorHandlerLoc = p.fnOrArrowDataVisit.tryCatchLoc
							}
							p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)

							// Create a new expression to represent the operation
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ERequireResolveString{
								ImportRecordIndex: importRecordIndex,
								CloseParenLoc:     e.CloseParenLoc,
							}}
						}

						// Otherwise just return a clone of the "require.resolve()" call
						return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
							Target: js_ast.Expr{Loc: e.Target.Loc, Data: &js_ast.EDot{
								Target:  p.valueToSubstituteForRequire(t.Target.Loc),
								Name:    t.Name,
								NameLoc: t.NameLoc,
							}},
							Args:          []js_ast.Expr{arg},
							Kind:          e.Kind,
							CloseParenLoc: e.CloseParenLoc,
						}}
					}), exprOut{}
				}
			}

			// Recognize "Object.create()" calls
			if couldBeObjectCreate && t.Name == "create" {
				if id, ok := t.Target.Data.(*js_ast.EIdentifier); ok {
					if symbol := &p.symbols[id.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound && symbol.OriginalName == "Object" {
						switch e.Args[0].Data.(type) {
						case *js_ast.ENull, *js_ast.EObject:
							// Mark "Object.create(null)" and "Object.create({})" as pure
							e.CanBeUnwrappedIfUnused = true
						}
					}
				}
			}

			if p.options.minifySyntax {
				switch t.Name {
				case "charCodeAt":
					// Recognize "charCodeAt()" calls
					if str, ok := t.Target.Data.(*js_ast.EString); ok && len(e.Args) <= 1 {
						index := 0
						hasIndex := false
						if len(e.Args) == 0 {
							hasIndex = true
						} else if num, ok := e.Args[0].Data.(*js_ast.ENumber); ok && num.Value == math.Trunc(num.Value) && math.Abs(num.Value) <= 0x7FFF_FFFF {
							index = int(num.Value)
							hasIndex = true
						}
						if hasIndex {
							if index >= 0 && index < len(str.Value) {
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(str.Value[index])}}, exprOut{}
							} else {
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: math.NaN()}}, exprOut{}
							}
						}
					}

				case "fromCharCode":
					// Recognize "fromCharCode()" calls
					if id, ok := t.Target.Data.(*js_ast.EIdentifier); ok {
						if symbol := &p.symbols[id.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound && symbol.OriginalName == "String" {
							charCodes := make([]uint16, 0, len(e.Args))
							for _, arg := range e.Args {
								arg, ok := js_ast.ToNumberWithoutSideEffects(arg.Data)
								if !ok {
									break
								}
								charCodes = append(charCodes, uint16(js_ast.ToInt32(arg)))
							}
							if len(charCodes) == len(e.Args) {
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: charCodes}}, exprOut{}
							}
						}
					}

				case "toString":
					switch target := t.Target.Data.(type) {
					case *js_ast.ENumber:
						radix := 0
						if len(e.Args) == 0 {
							radix = 10
						} else if len(e.Args) == 1 {
							if num, ok := e.Args[0].Data.(*js_ast.ENumber); ok && num.Value == math.Trunc(num.Value) && num.Value >= 2 && num.Value <= 36 {
								radix = int(num.Value)
							}
						}
						if radix != 0 {
							if str, ok := js_ast.TryToStringOnNumberSafely(target.Value, radix); ok {
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(str)}}, exprOut{}
							}
						}

					case *js_ast.ERegExp:
						if len(e.Args) == 0 {
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(target.Value)}}, exprOut{}
						}

					case *js_ast.EBoolean:
						if len(e.Args) == 0 {
							if target.Value {
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16("true")}}, exprOut{}
							} else {
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16("false")}}, exprOut{}
							}
						}

					case *js_ast.EString:
						if len(e.Args) == 0 {
							return t.Target, exprOut{}
						}
					}
				}
			}

			// Copy the call side effect flag over if this is a known target
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}

		case *js_ast.EIndex:
			// Copy the call side effect flag over if this is a known target
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}

		case *js_ast.ESuper:
			// If we're shimming "super()" calls, replace this call with "__super()"
			if p.superCtorRef != ast.InvalidRef {
				p.recordUsage(p.superCtorRef)
				target.Data = &js_ast.EIdentifier{Ref: p.superCtorRef}
				e.Target.Data = target.Data
			}
		}

		// Handle parenthesized optional chains
		if isParenthesizedOptionalChain && out.thisArgFunc != nil && out.thisArgWrapFunc != nil {
			return p.lowerParenthesizedOptionalChain(expr.Loc, e, out), exprOut{}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain == js_ast.OptionalChainStart ||
			(e.OptionalChain == js_ast.OptionalChainContinue && out.childContainsOptionalChain)
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out)
		}

		// If this is a plain call expression (instead of an optional chain), lower
		// private member access in the call target now if there is one
		if !containsOptionalChain {
			if target, loc, private := p.extractPrivateIndex(e.Target); private != nil {
				// "foo.#bar(123)" => "__privateGet(_a = foo, #bar).call(_a, 123)"
				targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(target.Loc, 2, target, valueCouldBeMutated)
				return targetWrapFunc(js_ast.Expr{Loc: target.Loc, Data: &js_ast.ECall{
					Target: js_ast.Expr{Loc: target.Loc, Data: &js_ast.EDot{
						Target:  p.lowerPrivateGet(targetFunc(), loc, private),
						Name:    "call",
						NameLoc: target.Loc,
					}},
					Args:                   append([]js_ast.Expr{targetFunc()}, e.Args...),
					CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
					Kind:                   js_ast.TargetWasOriginallyPropertyAccess,
				}}), exprOut{}
			}
			p.maybeLowerSuperPropertyGetInsideCall(e)
		}

		// Track calls to require() so we can use them while bundling
		if p.options.mode != config.ModePassThrough && e.OptionalChain == js_ast.OptionalChainNone {
			if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok && id.Ref == p.requireRef {
				// Heuristic: omit warnings inside try/catch blocks because presumably
				// the try/catch statement is there to handle the potential run-time
				// error from the unbundled require() call failing.
				omitWarnings := p.fnOrArrowDataVisit.tryBodyCount != 0

				if p.options.mode != config.ModePassThrough {
					// There must be one argument
					if len(e.Args) == 1 {
						p.ignoreUsage(p.requireRef)
						return p.maybeTransposeIfExprChain(e.Args[0], func(arg js_ast.Expr) js_ast.Expr {
							// The argument must be a string
							if str, ok := arg.Data.(*js_ast.EString); ok {
								// Ignore calls to require() if the control flow is provably dead here.
								// We don't want to spend time scanning the required files if they will
								// never be used.
								if p.isControlFlowDead {
									return js_ast.Expr{Loc: expr.Loc, Data: js_ast.ENullShared}
								}

								importRecordIndex := p.addImportRecord(ast.ImportRequire, p.source.RangeOfString(arg.Loc), helpers.UTF16ToString(str.Value), nil, 0)
								if p.fnOrArrowDataVisit.tryBodyCount != 0 {
									record := &p.importRecords[importRecordIndex]
									record.Flags |= ast.HandlesImportErrors
									record.ErrorHandlerLoc = p.fnOrArrowDataVisit.tryCatchLoc
								}
								p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)

								// Currently "require" is not converted into "import" for ESM
								if p.options.mode != config.ModeBundle && p.options.outputFormat == config.FormatESModule && !omitWarnings {
									r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
									p.log.AddID(logger.MsgID_JS_UnsupportedRequireCall, logger.Warning, &p.tracker, r, "Converting \"require\" to \"esm\" is currently not supported")
								}

								// Create a new expression to represent the operation
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ERequireString{
									ImportRecordIndex: importRecordIndex,
									CloseParenLoc:     e.CloseParenLoc,
								}}
							}

							// Handle glob patterns
							if p.options.mode == config.ModeBundle {
								if value := p.handleGlobPattern(arg, ast.ImportRequire, "globRequire", nil); value.Data != nil {
									return value
								}
							}

							// Use a debug log so people can see this if they want to
							r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
							p.log.AddID(logger.MsgID_JS_UnsupportedRequireCall, logger.Debug, &p.tracker, r,
								"This call to \"require\" will not be bundled because the argument is not a string literal")

							// Otherwise just return a clone of the "require()" call
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
								Target:        p.valueToSubstituteForRequire(e.Target.Loc),
								Args:          []js_ast.Expr{arg},
								CloseParenLoc: e.CloseParenLoc,
							}}
						}), exprOut{}
					} else {
						// Use a debug log so people can see this if they want to
						r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
						p.log.AddIDWithNotes(logger.MsgID_JS_UnsupportedRequireCall, logger.Debug, &p.tracker, r,
							fmt.Sprintf("This call to \"require\" will not be bundled because it has %d arguments", len(e.Args)),
							[]logger.MsgData{{Text: "To be bundled by esbuild, a \"require\" call must have exactly 1 argument."}})
					}

					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
						Target:        p.valueToSubstituteForRequire(e.Target.Loc),
						Args:          e.Args,
						CloseParenLoc: e.CloseParenLoc,
					}}, exprOut{}
				}
			}
		}

		out = exprOut{
			childContainsOptionalChain: containsOptionalChain,
			thisArgFunc:                out.thisArgFunc,
			thisArgWrapFunc:            out.thisArgWrapFunc,
		}
		if !in.hasChainParent {
			out.thisArgFunc = nil
			out.thisArgWrapFunc = nil
		}
		return expr, out

	case *js_ast.ENew:
		hasSpread := false

		e.Target = p.visitExpr(e.Target)
		p.warnAboutImportNamespaceCall(e.Target, exprKindNew)

		for i, arg := range e.Args {
			arg = p.visitExpr(arg)
			if _, ok := arg.Data.(*js_ast.ESpread); ok {
				hasSpread = true
			}
			e.Args[i] = arg
		}

		// "new foo(1, ...[2, 3], 4)" => "new foo(1, 2, 3, 4)"
		if p.options.minifySyntax && hasSpread {
			e.Args = js_ast.InlineSpreadsOfArrayLiterals(e.Args)
		}

		p.maybeMarkKnownGlobalConstructorAsPure(e)

	case *js_ast.EArrow:
		// Check for a propagated name to keep from the parent context
		var nameToKeep string
		if p.nameToKeepIsFor == e {
			nameToKeep = p.nameToKeep
		}

		// Prepare for suspicious logical operator checking
		if e.PreferExpr && len(e.Args) == 1 && e.Args[0].DefaultOrNil.Data == nil && len(e.Body.Block.Stmts) == 1 {
			if _, ok := e.Args[0].Binding.Data.(*js_ast.BIdentifier); ok {
				if stmt, ok := e.Body.Block.Stmts[0].Data.(*js_ast.SReturn); ok {
					if binary, ok := stmt.ValueOrNil.Data.(*js_ast.EBinary); ok && (binary.Op == js_ast.BinOpLogicalAnd || binary.Op == js_ast.BinOpLogicalOr) {
						p.suspiciousLogicalOperatorInsideArrow = binary
					}
				}
			}
		}

		asyncArrowNeedsToBeLowered := e.IsAsync && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait)
		oldFnOrArrowData := p.fnOrArrowDataVisit
		p.fnOrArrowDataVisit = fnOrArrowDataVisit{
			isArrow:                        true,
			isAsync:                        e.IsAsync,
			shouldLowerSuperPropertyAccess: oldFnOrArrowData.shouldLowerSuperPropertyAccess || asyncArrowNeedsToBeLowered,
		}

		// Mark if we're inside an async arrow function. This value should be true
		// even if we're inside multiple arrow functions and the closest inclosing
		// arrow function isn't async, as long as at least one enclosing arrow
		// function within the current enclosing function is async.
		oldInsideAsyncArrowFn := p.fnOnlyDataVisit.isInsideAsyncArrowFn
		if e.IsAsync {
			p.fnOnlyDataVisit.isInsideAsyncArrowFn = true
		}

		p.pushScopeForVisitPass(js_ast.ScopeFunctionArgs, expr.Loc)
		p.visitArgs(e.Args, visitArgsOpts{
			hasRestArg:               e.HasRestArg,
			body:                     e.Body.Block.Stmts,
			isUniqueFormalParameters: true,
		})
		p.pushScopeForVisitPass(js_ast.ScopeFunctionBody, e.Body.Loc)
		e.Body.Block.Stmts = p.visitStmtsAndPrependTempRefs(e.Body.Block.Stmts, prependTempRefsOpts{kind: stmtsFnBody})
		p.popScope()
		p.lowerFunction(&e.IsAsync, nil, &e.Args, e.Body.Loc, &e.Body.Block, &e.PreferExpr, &e.HasRestArg, true /* isArrow */)
		p.popScope()

		if p.options.minifySyntax && len(e.Body.Block.Stmts) == 1 {
			if s, ok := e.Body.Block.Stmts[0].Data.(*js_ast.SReturn); ok {
				if s.ValueOrNil.Data == nil {
					// "() => { return }" => "() => {}"
					e.Body.Block.Stmts = []js_ast.Stmt{}
				} else {
					// "() => { return x }" => "() => x"
					e.PreferExpr = true
				}
			}
		}

		p.fnOnlyDataVisit.isInsideAsyncArrowFn = oldInsideAsyncArrowFn
		p.fnOrArrowDataVisit = oldFnOrArrowData

		// Convert arrow functions to function expressions when lowering
		if p.options.unsupportedJSFeatures.Has(compat.Arrow) {
			expr.Data = &js_ast.EFunction{Fn: js_ast.Fn{
				Args:         e.Args,
				Body:         e.Body,
				ArgumentsRef: ast.InvalidRef,
				IsAsync:      e.IsAsync,
				HasRestArg:   e.HasRestArg,
			}}
		}

		// Optionally preserve the name
		if p.options.keepNames && nameToKeep != "" {
			expr = p.keepExprSymbolName(expr, nameToKeep)
		}

	case *js_ast.EFunction:
		// Check for a propagated name to keep from the parent context
		var nameToKeep string
		if p.nameToKeepIsFor == e {
			nameToKeep = p.nameToKeep
		}

		p.visitFn(&e.Fn, expr.Loc, visitFnOpts{
			isMethod:               in.isMethod,
			isDerivedClassCtor:     e == p.propDerivedCtorValue,
			isLoweredPrivateMethod: in.isLoweredPrivateMethod,
		})
		name := e.Fn.Name

		// Remove unused function names when minifying
		if p.options.minifySyntax && !p.currentScope.ContainsDirectEval &&
			name != nil && p.symbols[name.Ref.InnerIndex].UseCountEstimate == 0 {
			e.Fn.Name = nil
		}

		// Optionally preserve the name for functions, but not for methods
		if p.options.keepNames && (!in.isMethod || in.isLoweredPrivateMethod) {
			if name != nil {
				expr = p.keepExprSymbolName(expr, p.symbols[name.Ref.InnerIndex].OriginalName)
			} else if nameToKeep != "" {
				expr = p.keepExprSymbolName(expr, nameToKeep)
			}
		}

	case *js_ast.EClass:
		// Check for a propagated name to keep from the parent context
		var nameToKeep string
		if p.nameToKeepIsFor == e {
			nameToKeep = p.nameToKeep
		}

		result := p.visitClass(expr.Loc, &e.Class, ast.InvalidRef, nameToKeep)

		// Lower class field syntax for browsers that don't support it
		_, expr = p.lowerClass(js_ast.Stmt{}, expr, result, nameToKeep)

		// We may be able to determine that a class is side-effect before lowering
		// but not after lowering (e.g. due to "--keep-names" mutating the object).
		// If that's the case, add a special annotation so this doesn't prevent
		// tree-shaking from happening.
		if result.canBeRemovedIfUnused {
			expr.Data = &js_ast.EAnnotation{
				Value: expr,
				Flags: js_ast.CanBeRemovedIfUnusedFlag,
			}
		}

	default:
		// Note: EPrivateIdentifier should have already been handled
		panic(fmt.Sprintf("Unexpected expression of type %T", expr.Data))
	}

	return expr, exprOut{}
}

// This exists to handle very deeply-nested ASTs. For example, the "grapheme-splitter"
// package contains this monstrosity:
//
//	if (
//	  (0x0300 <= code && code <= 0x036F) ||
//	  (0x0483 <= code && code <= 0x0487) ||
//	  (0x0488 <= code && code <= 0x0489) ||
//	  (0x0591 <= code && code <= 0x05BD) ||
//	  ... many hundreds of lines later ...
//	) {
//	  return;
//	}
//
// If "checkAndPrepare" returns non-nil, then the return value is the final
// expression. Otherwise, the final expression can be obtained by manually
// visiting the left child and then calling "visitRightAndFinish":
//
//	if result := v.checkAndPrepare(p); result.Data != nil {
//	  return result
//	}
//	v.e.Left, _ = p.visitExprInOut(v.e.Left, v.leftIn)
//	return v.visitRightAndFinish(p)
//
// This code is convoluted this way so that we can use our own stack on the
// heap instead of the call stack when there are additional levels of nesting.
// Before this transformation, the code previously looked something like this:
//
//	... The code in "checkAndPrepare" ...
//	e.Left, _ = p.visitExprInOut(e.Left, in)
//	... The code in "visitRightAndFinish" ...
//
// If this code is still confusing, it may be helpful to look back in git
// history at the commit that introduced this transformation.
//
// Go normally has growable call stacks so this code transformation normally
// doesn't do anything, but WebAssembly doesn't allow stack pointer manipulation
// so Go's WebAssembly implementation doesn't support growable call stacks and
// is therefore vulnerable to stack overflow. So this code transformation is
// only really relevant for esbuild's WebAssembly-based API.
type binaryExprVisitor struct {
	// Inputs
	e   *js_ast.EBinary
	loc logger.Loc
	in  exprIn

	// Input for visiting the left child
	leftIn exprIn

	// "Local variables" passed from "checkAndPrepare" to "visitRightAndFinish"
	isStmtExpr                               bool
	oldSilenceWarningAboutThisBeingUndefined bool
}

func (v *binaryExprVisitor) checkAndPrepare(p *parser) js_ast.Expr {
	e := v.e

	// Special-case EPrivateIdentifier to allow it here
	if private, ok := e.Left.Data.(*js_ast.EPrivateIdentifier); ok && e.Op == js_ast.BinOpIn {
		name := p.loadNameFromRef(private.Ref)
		result := p.findSymbol(e.Left.Loc, name)
		private.Ref = result.ref

		// Unlike regular identifiers, there are no unbound private identifiers
		symbol := &p.symbols[result.ref.InnerIndex]
		if !symbol.Kind.IsPrivate() {
			r := logger.Range{Loc: e.Left.Loc, Len: int32(len(name))}
			p.log.AddError(&p.tracker, r, fmt.Sprintf("Private name %q must be declared in an enclosing class", name))
		}

		e.Right = p.visitExpr(e.Right)

		if p.privateSymbolNeedsToBeLowered(private) {
			return p.lowerPrivateBrandCheck(e.Right, v.loc, private)
		}
		return js_ast.Expr{Loc: v.loc, Data: e}
	}

	v.isStmtExpr = e == p.stmtExprValue
	v.oldSilenceWarningAboutThisBeingUndefined = p.fnOnlyDataVisit.silenceMessageAboutThisBeingUndefined

	if _, ok := e.Left.Data.(*js_ast.EThis); ok && e.Op == js_ast.BinOpLogicalAnd {
		p.fnOnlyDataVisit.silenceMessageAboutThisBeingUndefined = true
	}
	v.leftIn = exprIn{
		assignTarget:               e.Op.BinaryAssignTarget(),
		shouldMangleStringsAsProps: e.Op == js_ast.BinOpIn,
	}
	return js_ast.Expr{}
}

func (v *binaryExprVisitor) visitRightAndFinish(p *parser) js_ast.Expr {
	e := v.e

	// Mark the control flow as dead if the branch is never taken
	switch e.Op {
	case js_ast.BinOpLogicalOr:
		if boolean, _, ok := js_ast.ToBooleanWithSideEffects(e.Left.Data); ok && boolean {
			// "true || dead"
			old := p.isControlFlowDead
			p.isControlFlowDead = true
			e.Right = p.visitExpr(e.Right)
			p.isControlFlowDead = old
		} else {
			e.Right = p.visitExpr(e.Right)
		}

	case js_ast.BinOpLogicalAnd:
		if boolean, _, ok := js_ast.ToBooleanWithSideEffects(e.Left.Data); ok && !boolean {
			// "false && dead"
			old := p.isControlFlowDead
			p.isControlFlowDead = true
			e.Right = p.visitExpr(e.Right)
			p.isControlFlowDead = old
		} else {
			e.Right = p.visitExpr(e.Right)
		}

	case js_ast.BinOpNullishCoalescing:
		if isNullOrUndefined, _, ok := js_ast.ToNullOrUndefinedWithSideEffects(e.Left.Data); ok && !isNullOrUndefined {
			// "notNullOrUndefined ?? dead"
			old := p.isControlFlowDead
			p.isControlFlowDead = true
			e.Right = p.visitExpr(e.Right)
			p.isControlFlowDead = old
		} else {
			e.Right = p.visitExpr(e.Right)
		}

	case js_ast.BinOpComma:
		e.Right, _ = p.visitExprInOut(e.Right, exprIn{
			shouldMangleStringsAsProps: v.in.shouldMangleStringsAsProps,
		})

	case js_ast.BinOpAssign, js_ast.BinOpLogicalOrAssign, js_ast.BinOpLogicalAndAssign, js_ast.BinOpNullishCoalescingAssign:
		// Check for a propagated name to keep from the parent context
		if id, ok := e.Left.Data.(*js_ast.EIdentifier); ok {
			p.nameToKeep = p.symbols[id.Ref.InnerIndex].OriginalName
			p.nameToKeepIsFor = e.Right.Data
		}

		e.Right = p.visitExpr(e.Right)

	default:
		e.Right = p.visitExpr(e.Right)
	}
	p.fnOnlyDataVisit.silenceMessageAboutThisBeingUndefined = v.oldSilenceWarningAboutThisBeingUndefined

	// Always put constants consistently on the same side for equality
	// comparisons to help improve compression. In theory, dictionary-based
	// compression methods may already have a dictionary entry for code that
	// is similar to previous code. Note that we can only reorder expressions
	// that do not have any side effects.
	//
	// Constants are currently ordered on the right instead of the left because
	// it results in slightly smalller gzip size on our primary benchmark
	// (although slightly larger uncompressed size). The size difference is
	// less than 0.1% so it really isn't that important an optimization.
	if p.options.minifySyntax {
		switch e.Op {
		case js_ast.BinOpLooseEq, js_ast.BinOpLooseNe, js_ast.BinOpStrictEq, js_ast.BinOpStrictNe:
			// "1 === x" => "x === 1"
			if js_ast.IsPrimitiveLiteral(e.Left.Data) && !js_ast.IsPrimitiveLiteral(e.Right.Data) {
				e.Left, e.Right = e.Right, e.Left
			}
		}
	}

	if p.shouldFoldTypeScriptConstantExpressions || (p.options.minifySyntax && js_ast.ShouldFoldBinaryOperatorWhenMinifying(e)) {
		if result := js_ast.FoldBinaryOperator(v.loc, e); result.Data != nil {
			return result
		}
	}

	// Post-process the binary expression
	switch e.Op {
	case js_ast.BinOpComma:
		// "(1, 2)" => "2"
		// "(sideEffects(), 2)" => "(sideEffects(), 2)"
		if p.options.minifySyntax {
			e.Left = p.astHelpers.SimplifyUnusedExpr(e.Left, p.options.unsupportedJSFeatures)
			if e.Left.Data == nil {
				return e.Right
			}
		}

	case js_ast.BinOpLooseEq:
		if result, ok := js_ast.CheckEqualityIfNoSideEffects(e.Left.Data, e.Right.Data, js_ast.LooseEquality); ok {
			return js_ast.Expr{Loc: v.loc, Data: &js_ast.EBoolean{Value: result}}
		}
		afterOpLoc := locAfterOp(e)
		if !p.warnAboutEqualityCheck("==", e.Left, afterOpLoc) {
			p.warnAboutEqualityCheck("==", e.Right, afterOpLoc)
		}
		p.warnAboutTypeofAndString(e.Left, e.Right, checkBothOrders)

		if p.options.minifySyntax {
			// "x == void 0" => "x == null"
			if _, ok := e.Left.Data.(*js_ast.EUndefined); ok {
				e.Left.Data = js_ast.ENullShared
			} else if _, ok := e.Right.Data.(*js_ast.EUndefined); ok {
				e.Right.Data = js_ast.ENullShared
			}

			if result, ok := js_ast.MaybeSimplifyEqualityComparison(v.loc, e, p.options.unsupportedJSFeatures); ok {
				return result
			}
		}

	case js_ast.BinOpStrictEq:
		if result, ok := js_ast.CheckEqualityIfNoSideEffects(e.Left.Data, e.Right.Data, js_ast.StrictEquality); ok {
			return js_ast.Expr{Loc: v.loc, Data: &js_ast.EBoolean{Value: result}}
		}
		afterOpLoc := locAfterOp(e)
		if !p.warnAboutEqualityCheck("===", e.Left, afterOpLoc) {
			p.warnAboutEqualityCheck("===", e.Right, afterOpLoc)
		}
		p.warnAboutTypeofAndString(e.Left, e.Right, checkBothOrders)

		if p.options.minifySyntax {
			// "typeof x === 'undefined'" => "typeof x == 'undefined'"
			if js_ast.CanChangeStrictToLoose(e.Left, e.Right) {
				e.Op = js_ast.BinOpLooseEq
			}

			if result, ok := js_ast.MaybeSimplifyEqualityComparison(v.loc, e, p.options.unsupportedJSFeatures); ok {
				return result
			}
		}

	case js_ast.BinOpLooseNe:
		if result, ok := js_ast.CheckEqualityIfNoSideEffects(e.Left.Data, e.Right.Data, js_ast.LooseEquality); ok {
			return js_ast.Expr{Loc: v.loc, Data: &js_ast.EBoolean{Value: !result}}
		}
		afterOpLoc := locAfterOp(e)
		if !p.warnAboutEqualityCheck("!=", e.Left, afterOpLoc) {
			p.warnAboutEqualityCheck("!=", e.Right, afterOpLoc)
		}
		p.warnAboutTypeofAndString(e.Left, e.Right, checkBothOrders)

		if p.options.minifySyntax {
			// "x != void 0" => "x != null"
			if _, ok := e.Left.Data.(*js_ast.EUndefined); ok {
				e.Left.Data = js_ast.ENullShared
			} else if _, ok := e.Right.Data.(*js_ast.EUndefined); ok {
				e.Right.Data = js_ast.ENullShared
			}

			if result, ok := js_ast.MaybeSimplifyEqualityComparison(v.loc, e, p.options.unsupportedJSFeatures); ok {
				return result
			}
		}

	case js_ast.BinOpStrictNe:
		if result, ok := js_ast.CheckEqualityIfNoSideEffects(e.Left.Data, e.Right.Data, js_ast.StrictEquality); ok {
			return js_ast.Expr{Loc: v.loc, Data: &js_ast.EBoolean{Value: !result}}
		}
		afterOpLoc := locAfterOp(e)
		if !p.warnAboutEqualityCheck("!==", e.Left, afterOpLoc) {
			p.warnAboutEqualityCheck("!==", e.Right, afterOpLoc)
		}
		p.warnAboutTypeofAndString(e.Left, e.Right, checkBothOrders)

		if p.options.minifySyntax {
			// "typeof x !== 'undefined'" => "typeof x != 'undefined'"
			if js_ast.CanChangeStrictToLoose(e.Left, e.Right) {
				e.Op = js_ast.BinOpLooseNe
			}

			if result, ok := js_ast.MaybeSimplifyEqualityComparison(v.loc, e, p.options.unsupportedJSFeatures); ok {
				return result
			}
		}

	case js_ast.BinOpNullishCoalescing:
		if isNullOrUndefined, sideEffects, ok := js_ast.ToNullOrUndefinedWithSideEffects(e.Left.Data); ok {
			// Warn about potential bugs
			if !js_ast.IsPrimitiveLiteral(e.Left.Data) {
				// "return props.flag === flag ?? true" is "return (props.flag === flag) ?? true" not "return props.flag === (flag ?? true)"
				var which string
				var leftIsNullOrUndefined string
				var leftIsReturned string
				if !isNullOrUndefined {
					which = "left"
					leftIsNullOrUndefined = "never"
					leftIsReturned = "always"
				} else {
					which = "right"
					leftIsNullOrUndefined = "always"
					leftIsReturned = "never"
				}
				kind := logger.Warning
				if p.suppressWarningsAboutWeirdCode {
					kind = logger.Debug
				}
				rOp := p.source.RangeOfOperatorBefore(e.Right.Loc, "??")
				rLeft := logger.Range{Loc: e.Left.Loc, Len: p.source.LocBeforeWhitespace(rOp.Loc).Start - e.Left.Loc.Start}
				p.log.AddIDWithNotes(logger.MsgID_JS_SuspiciousNullishCoalescing, kind, &p.tracker, rOp,
					fmt.Sprintf("The \"??\" operator here will always return the %s operand", which), []logger.MsgData{
						p.tracker.MsgData(rLeft, fmt.Sprintf(
							"The left operand of the \"??\" operator here will %s be null or undefined, so it will %s be returned. This usually indicates a bug in your code:",
							leftIsNullOrUndefined, leftIsReturned))})
			}

			if !isNullOrUndefined {
				return e.Left
			} else if sideEffects == js_ast.NoSideEffects {
				return e.Right
			}
		}

		if p.options.minifySyntax {
			// "a ?? (b ?? c)" => "a ?? b ?? c"
			if right, ok := e.Right.Data.(*js_ast.EBinary); ok && right.Op == js_ast.BinOpNullishCoalescing {
				e.Left = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpNullishCoalescing, e.Left, right.Left)
				e.Right = right.Right
			}
		}

		if p.options.unsupportedJSFeatures.Has(compat.NullishCoalescing) {
			return p.lowerNullishCoalescing(v.loc, e.Left, e.Right)
		}

	case js_ast.BinOpLogicalOr:
		if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(e.Left.Data); ok {
			// Warn about potential bugs
			if e == p.suspiciousLogicalOperatorInsideArrow {
				if arrowLoc := p.source.RangeOfOperatorBefore(v.loc, "=>"); arrowLoc.Loc.Start+2 == p.source.LocBeforeWhitespace(v.loc).Start {
					// "return foo => 1 || foo <= 0"
					var which string
					if boolean {
						which = "left"
					} else {
						which = "right"
					}
					kind := logger.Warning
					if p.suppressWarningsAboutWeirdCode {
						kind = logger.Debug
					}
					note := p.tracker.MsgData(arrowLoc,
						"The \"=>\" symbol creates an arrow function expression in JavaScript. Did you mean to use the greater-than-or-equal-to operator \">=\" here instead?")
					note.Location.Suggestion = ">="
					rOp := p.source.RangeOfOperatorBefore(e.Right.Loc, "||")
					p.log.AddIDWithNotes(logger.MsgID_JS_SuspiciousLogicalOperator, kind, &p.tracker, rOp,
						fmt.Sprintf("The \"||\" operator here will always return the %s operand", which), []logger.MsgData{note})
				}
			}

			if boolean {
				return e.Left
			} else if sideEffects == js_ast.NoSideEffects {
				return e.Right
			}
		}

		if p.options.minifySyntax {
			// "a || (b || c)" => "a || b || c"
			if right, ok := e.Right.Data.(*js_ast.EBinary); ok && right.Op == js_ast.BinOpLogicalOr {
				e.Left = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Left, right.Left)
				e.Right = right.Right
			}

			// "a === null || a === undefined" => "a == null"
			if left, right, ok := js_ast.IsBinaryNullAndUndefined(e.Left, e.Right, js_ast.BinOpStrictEq); ok {
				e.Op = js_ast.BinOpLooseEq
				e.Left = left
				e.Right = right
			}
		}

	case js_ast.BinOpLogicalAnd:
		if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(e.Left.Data); ok {
			// Warn about potential bugs
			if e == p.suspiciousLogicalOperatorInsideArrow {
				if arrowLoc := p.source.RangeOfOperatorBefore(v.loc, "=>"); arrowLoc.Loc.Start+2 == p.source.LocBeforeWhitespace(v.loc).Start {
					// "return foo => 0 && foo <= 1"
					var which string
					if !boolean {
						which = "left"
					} else {
						which = "right"
					}
					kind := logger.Warning
					if p.suppressWarningsAboutWeirdCode {
						kind = logger.Debug
					}
					note := p.tracker.MsgData(arrowLoc,
						"The \"=>\" symbol creates an arrow function expression in JavaScript. Did you mean to use the greater-than-or-equal-to operator \">=\" here instead?")
					note.Location.Suggestion = ">="
					rOp := p.source.RangeOfOperatorBefore(e.Right.Loc, "&&")
					p.log.AddIDWithNotes(logger.MsgID_JS_SuspiciousLogicalOperator, kind, &p.tracker, rOp,
						fmt.Sprintf("The \"&&\" operator here will always return the %s operand", which), []logger.MsgData{note})
				}
			}

			if !boolean {
				return e.Left
			} else if sideEffects == js_ast.NoSideEffects {
				return e.Right
			}
		}

		if p.options.minifySyntax {
			// "a && (b && c)" => "a && b && c"
			if right, ok := e.Right.Data.(*js_ast.EBinary); ok && right.Op == js_ast.BinOpLogicalAnd {
				e.Left = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Left, right.Left)
				e.Right = right.Right
			}

			// "a !== null && a !== undefined" => "a != null"
			if left, right, ok := js_ast.IsBinaryNullAndUndefined(e.Left, e.Right, js_ast.BinOpStrictNe); ok {
				e.Op = js_ast.BinOpLooseNe
				e.Left = left
				e.Right = right
			}
		}

	case js_ast.BinOpAdd:
		// "'abc' + 'xyz'" => "'abcxyz'"
		if result := js_ast.FoldStringAddition(e.Left, e.Right, js_ast.StringAdditionNormal); result.Data != nil {
			return result
		}

		if left, ok := e.Left.Data.(*js_ast.EBinary); ok && left.Op == js_ast.BinOpAdd {
			// "x + 'abc' + 'xyz'" => "x + 'abcxyz'"
			if result := js_ast.FoldStringAddition(left.Right, e.Right, js_ast.StringAdditionWithNestedLeft); result.Data != nil {
				return js_ast.Expr{Loc: v.loc, Data: &js_ast.EBinary{Op: left.Op, Left: left.Left, Right: result}}
			}
		}

	case js_ast.BinOpPow:
		// Lower the exponentiation operator for browsers that don't support it
		if p.options.unsupportedJSFeatures.Has(compat.ExponentOperator) {
			return p.callRuntime(v.loc, "__pow", []js_ast.Expr{e.Left, e.Right})
		}

		////////////////////////////////////////////////////////////////////////////////
		// All assignment operators below here

	case js_ast.BinOpAssign:
		if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
			return p.lowerPrivateSet(target, loc, private, e.Right)
		}

		if property := p.extractSuperProperty(e.Left); property.Data != nil {
			return p.lowerSuperPropertySet(e.Left.Loc, property, e.Right)
		}

		// Lower assignment destructuring patterns for browsers that don't
		// support them. Note that assignment expressions are used to represent
		// initializers in binding patterns, so only do this if we're not
		// ourselves the target of an assignment. Example: "[a = b] = c"
		if v.in.assignTarget == js_ast.AssignTargetNone {
			mode := objRestMustReturnInitExpr
			if v.isStmtExpr {
				mode = objRestReturnValueIsUnused
			}
			if result, ok := p.lowerAssign(e.Left, e.Right, mode); ok {
				return result
			}

			// If CommonJS-style exports are disabled, then references to them are
			// treated as global variable references. This is consistent with how
			// they work in node and the browser, so it's the correct interpretation.
			//
			// However, people sometimes try to use both types of exports within the
			// same module and expect it to work. We warn about this when module
			// format conversion is enabled.
			//
			// Only warn about this for uses in assignment position since there are
			// some legitimate other uses. For example, some people do "typeof module"
			// to check for a CommonJS environment, and we shouldn't warn on that.
			if p.options.mode != config.ModePassThrough && p.isFileConsideredToHaveESMExports && !p.isControlFlowDead {
				if dot, ok := e.Left.Data.(*js_ast.EDot); ok {
					var name string
					var loc logger.Loc

					switch target := dot.Target.Data.(type) {
					case *js_ast.EIdentifier:
						if symbol := &p.symbols[target.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound &&
							((symbol.OriginalName == "module" && dot.Name == "exports") || symbol.OriginalName == "exports") &&
							!symbol.Flags.Has(ast.DidWarnAboutCommonJSInESM) {
							// "module.exports = ..."
							// "exports.something = ..."
							name = symbol.OriginalName
							loc = dot.Target.Loc
							symbol.Flags |= ast.DidWarnAboutCommonJSInESM
						}

					case *js_ast.EDot:
						if target.Name == "exports" {
							if id, ok := target.Target.Data.(*js_ast.EIdentifier); ok {
								if symbol := &p.symbols[id.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound &&
									symbol.OriginalName == "module" && !symbol.Flags.Has(ast.DidWarnAboutCommonJSInESM) {
									// "module.exports.foo = ..."
									name = symbol.OriginalName
									loc = target.Target.Loc
									symbol.Flags |= ast.DidWarnAboutCommonJSInESM
								}
							}
						}
					}

					if name != "" {
						kind := logger.Warning
						if p.suppressWarningsAboutWeirdCode {
							kind = logger.Debug
						}
						why, notes := p.whyESModule()
						if why == whyESMTypeModulePackageJSON {
							text := "Node's package format requires that CommonJS files in a \"type\": \"module\" package use the \".cjs\" file extension."
							if p.options.ts.Parse {
								text += " If you are using TypeScript, you can use the \".cts\" file extension with esbuild instead."
							}
							notes = append(notes, logger.MsgData{Text: text})
						}
						p.log.AddIDWithNotes(logger.MsgID_JS_CommonJSVariableInESM, kind, &p.tracker, js_lexer.RangeOfIdentifier(p.source, loc),
							fmt.Sprintf("The CommonJS %q variable is treated as a global variable in an ECMAScript module and may not work as expected", name),
							notes)
					}
				}
			}
		}

	case js_ast.BinOpAddAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpAdd, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpSubAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpSub, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpMulAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpMul, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpDivAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpDiv, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpRemAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpRem, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpPowAssign:
		// Lower the exponentiation operator for browsers that don't support it
		if p.options.unsupportedJSFeatures.Has(compat.ExponentOperator) {
			return p.lowerExponentiationAssignmentOperator(v.loc, e)
		}

		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpPow, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpShlAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpShl, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpShrAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpShr, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpUShrAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpUShr, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpBitwiseOrAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpBitwiseOr, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpBitwiseAndAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpBitwiseAnd, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpBitwiseXorAssign:
		if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpBitwiseXor, e.Right); result.Data != nil {
			return result
		}

	case js_ast.BinOpNullishCoalescingAssign:
		if value, ok := p.lowerNullishCoalescingAssignmentOperator(v.loc, e); ok {
			return value
		}

	case js_ast.BinOpLogicalAndAssign:
		if value, ok := p.lowerLogicalAssignmentOperator(v.loc, e, js_ast.BinOpLogicalAnd); ok {
			return value
		}

	case js_ast.BinOpLogicalOrAssign:
		if value, ok := p.lowerLogicalAssignmentOperator(v.loc, e, js_ast.BinOpLogicalOr); ok {
			return value
		}
	}

	// "(a, b) + c" => "a, b + c"
	if p.options.minifySyntax && e.Op != js_ast.BinOpComma {
		if comma, ok := e.Left.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
			return js_ast.JoinWithComma(comma.Left, js_ast.Expr{
				Loc: comma.Right.Loc,
				Data: &js_ast.EBinary{
					Op:    e.Op,
					Left:  comma.Right,
					Right: e.Right,
				},
			})
		}
	}

	return js_ast.Expr{Loc: v.loc, Data: e}
}

func remapExprLocsInJSON(expr *js_ast.Expr, table []logger.StringInJSTableEntry) {
	expr.Loc = logger.RemapStringInJSLoc(table, expr.Loc)

	switch e := expr.Data.(type) {
	case *js_ast.EArray:
		e.CloseBracketLoc = logger.RemapStringInJSLoc(table, e.CloseBracketLoc)
		for i := range e.Items {
			remapExprLocsInJSON(&e.Items[i], table)
		}

	case *js_ast.EObject:
		e.CloseBraceLoc = logger.RemapStringInJSLoc(table, e.CloseBraceLoc)
		for i := range e.Properties {
			remapExprLocsInJSON(&e.Properties[i].Key, table)
			remapExprLocsInJSON(&e.Properties[i].ValueOrNil, table)
		}
	}
}

func (p *parser) handleGlobPattern(expr js_ast.Expr, kind ast.ImportKind, prefix string, assertOrWith *ast.ImportAssertOrWith) js_ast.Expr {
	pattern, approximateRange := p.globPatternFromExpr(expr)
	if pattern == nil {
		return js_ast.Expr{}
	}

	var last helpers.GlobPart
	var parts []helpers.GlobPart

	for _, part := range pattern {
		if part.isWildcard {
			if last.Wildcard == helpers.GlobNone {
				if !strings.HasSuffix(last.Prefix, "/") {
					// "`a${b}c`" => "a*c"
					last.Wildcard = helpers.GlobAllExceptSlash
				} else {
					// "`a/${b}c`" => "a/**/*c"
					last.Wildcard = helpers.GlobAllIncludingSlash
					parts = append(parts, last)
					last = helpers.GlobPart{Prefix: "/", Wildcard: helpers.GlobAllExceptSlash}
				}
			}
		} else if part.text != "" {
			if last.Wildcard != helpers.GlobNone {
				parts = append(parts, last)
				last = helpers.GlobPart{}
			}
			last.Prefix += part.text
		}
	}

	parts = append(parts, last)

	// Don't handle this if it's a string constant
	if len(parts) == 1 && parts[0].Wildcard == helpers.GlobNone {
		return js_ast.Expr{}
	}

	// We currently only support relative globs
	if prefix := parts[0].Prefix; !strings.HasPrefix(prefix, "./") && !strings.HasPrefix(prefix, "../") {
		return js_ast.Expr{}
	}

	ref := ast.InvalidRef

	// Don't generate duplicate glob imports
outer:
	for _, globPattern := range p.globPatternImports {
		// Check the kind
		if globPattern.kind != kind {
			continue
		}

		// Check the parts
		if len(globPattern.parts) != len(parts) {
			continue
		}
		for i := range parts {
			if globPattern.parts[i] != parts[i] {
				continue outer
			}
		}

		// Check the import assertions/attributes
		if assertOrWith == nil {
			if globPattern.assertOrWith != nil {
				continue
			}
		} else {
			if globPattern.assertOrWith == nil {
				continue
			}
			if assertOrWith.Keyword != globPattern.assertOrWith.Keyword {
				continue
			}
			a := assertOrWith.Entries
			b := globPattern.assertOrWith.Entries
			if len(a) != len(b) {
				continue
			}
			for i := range a {
				ai := a[i]
				bi := b[i]
				if !helpers.UTF16EqualsUTF16(ai.Key, bi.Key) || !helpers.UTF16EqualsUTF16(ai.Value, bi.Value) {
					continue outer
				}
			}
		}

		// If we get here, then these are the same glob pattern
		ref = globPattern.ref
		break
	}

	// If there's no duplicate glob import, then generate a new glob import
	if ref == ast.InvalidRef && prefix != "" {
		sb := strings.Builder{}
		sb.WriteString(prefix)

		for _, part := range parts {
			gap := true
			for _, c := range part.Prefix {
				if !js_ast.IsIdentifierContinue(c) {
					gap = true
				} else {
					if gap {
						sb.WriteByte('_')
						gap = false
					}
					sb.WriteRune(c)
				}
			}
		}

		name := sb.String()
		ref = p.newSymbol(ast.SymbolOther, name)
		p.moduleScope.Generated = append(p.moduleScope.Generated, ref)

		p.globPatternImports = append(p.globPatternImports, globPatternImport{
			assertOrWith:     assertOrWith,
			parts:            parts,
			name:             name,
			approximateRange: approximateRange,
			ref:              ref,
			kind:             kind,
		})
	}

	p.recordUsage(ref)
	return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: ref}},
		Args:   []js_ast.Expr{expr},
	}}
}

type globPart struct {
	text       string
	isWildcard bool
}

func (p *parser) globPatternFromExpr(expr js_ast.Expr) ([]globPart, logger.Range) {
	switch e := expr.Data.(type) {
	case *js_ast.EString:
		return []globPart{{text: helpers.UTF16ToString(e.Value)}}, p.source.RangeOfString(expr.Loc)

	case *js_ast.ETemplate:
		if e.TagOrNil.Data != nil {
			break
		}

		pattern := make([]globPart, 0, 1+2*len(e.Parts))
		pattern = append(pattern, globPart{text: helpers.UTF16ToString(e.HeadCooked)})

		for _, part := range e.Parts {
			if partPattern, _ := p.globPatternFromExpr(part.Value); partPattern != nil {
				pattern = append(pattern, partPattern...)
			} else {
				pattern = append(pattern, globPart{isWildcard: true})
			}
			pattern = append(pattern, globPart{text: helpers.UTF16ToString(part.TailCooked)})
		}

		if len(e.Parts) == 0 {
			return pattern, p.source.RangeOfString(expr.Loc)
		}

		text := p.source.Contents
		templateRange := logger.Range{Loc: e.HeadLoc}

		for i := e.Parts[len(e.Parts)-1].TailLoc.Start; i < int32(len(text)); i++ {
			c := text[i]
			if c == '`' {
				templateRange.Len = i + 1 - templateRange.Loc.Start
				break
			} else if c == '\\' {
				i += 1
			}
		}

		return pattern, templateRange

	case *js_ast.EBinary:
		if e.Op != js_ast.BinOpAdd {
			break
		}

		pattern, leftRange := p.globPatternFromExpr(e.Left)
		if pattern == nil {
			break
		}

		if rightPattern, rightRange := p.globPatternFromExpr(e.Right); rightPattern != nil {
			pattern = append(pattern, rightPattern...)
			leftRange.Len = rightRange.End() - leftRange.Loc.Start
			return pattern, leftRange
		}

		pattern = append(pattern, globPart{isWildcard: true})

		// Try to extend the left range by the right operand in some common cases
		switch right := e.Right.Data.(type) {
		case *js_ast.EIdentifier:
			leftRange.Len = js_lexer.RangeOfIdentifier(p.source, e.Right.Loc).End() - leftRange.Loc.Start

		case *js_ast.ECall:
			if right.CloseParenLoc.Start > 0 {
				leftRange.Len = right.CloseParenLoc.Start + 1 - leftRange.Loc.Start
			}
		}

		return pattern, leftRange
	}

	return nil, logger.Range{}
}

func (p *parser) convertSymbolUseToCall(ref ast.Ref, isSingleNonSpreadArgCall bool) {
	// Remove the normal symbol use
	use := p.symbolUses[ref]
	use.CountEstimate--
	if use.CountEstimate == 0 {
		delete(p.symbolUses, ref)
	} else {
		p.symbolUses[ref] = use
	}

	// Add a special symbol use instead
	if p.symbolCallUses == nil {
		p.symbolCallUses = make(map[ast.Ref]js_ast.SymbolCallUse)
	}
	callUse := p.symbolCallUses[ref]
	callUse.CallCountEstimate++
	if isSingleNonSpreadArgCall {
		callUse.SingleArgNonSpreadCallCountEstimate++
	}
	p.symbolCallUses[ref] = callUse
}

func (p *parser) warnAboutImportNamespaceCall(target js_ast.Expr, kind importNamespaceCallKind) {
	if p.options.outputFormat != config.FormatPreserve {
		if id, ok := target.Data.(*js_ast.EIdentifier); ok && p.importItemsForNamespace[id.Ref].entries != nil {
			key := importNamespaceCall{
				ref:  id.Ref,
				kind: kind,
			}
			if p.importNamespaceCCMap == nil {
				p.importNamespaceCCMap = make(map[importNamespaceCall]bool)
			}

			// Don't log a warning for the same identifier more than once
			if _, ok := p.importNamespaceCCMap[key]; ok {
				return
			}

			p.importNamespaceCCMap[key] = true
			r := js_lexer.RangeOfIdentifier(p.source, target.Loc)

			var notes []logger.MsgData
			name := p.symbols[id.Ref.InnerIndex].OriginalName
			if member, ok := p.moduleScope.Members[name]; ok && member.Ref == id.Ref {
				if star := p.source.RangeOfOperatorBefore(member.Loc, "*"); star.Len > 0 {
					if as := p.source.RangeOfOperatorBefore(member.Loc, "as"); as.Len > 0 && as.Loc.Start > star.Loc.Start {
						note := p.tracker.MsgData(
							logger.Range{Loc: star.Loc, Len: js_lexer.RangeOfIdentifier(p.source, member.Loc).End() - star.Loc.Start},
							fmt.Sprintf("Consider changing %q to a default import instead:", name))
						note.Location.Suggestion = name
						notes = append(notes, note)
					}
				}
			}

			if p.options.ts.Parse {
				notes = append(notes, logger.MsgData{
					Text: "Make sure to enable TypeScript's \"esModuleInterop\" setting so that TypeScript's type checker generates an error when you try to do this. " +
						"You can read more about this setting here: https://www.typescriptlang.org/tsconfig#esModuleInterop",
				})
			}

			var verb string
			var where string
			var noun string

			switch kind {
			case exprKindCall:
				verb = "Calling"
				noun = "function"

			case exprKindNew:
				verb = "Constructing"
				noun = "constructor"

			case exprKindJSXTag:
				verb = "Using"
				where = " in a JSX expression"
				noun = "component"
			}

			p.log.AddIDWithNotes(logger.MsgID_JS_CallImportNamespace, logger.Warning, &p.tracker, r, fmt.Sprintf(
				"%s %q%s will crash at run-time because it's an import namespace object, not a %s",
				verb,
				p.symbols[id.Ref.InnerIndex].OriginalName,
				where,
				noun,
			), notes)
		}
	}
}

func (p *parser) maybeMarkKnownGlobalConstructorAsPure(e *js_ast.ENew) {
	if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok {
		if symbol := p.symbols[id.Ref.InnerIndex]; symbol.Kind == ast.SymbolUnbound {
			switch symbol.OriginalName {
			case "WeakSet", "WeakMap":
				n := len(e.Args)

				if n == 0 {
					// "new WeakSet()" is pure
					e.CanBeUnwrappedIfUnused = true
					break
				}

				if n == 1 {
					switch arg := e.Args[0].Data.(type) {
					case *js_ast.ENull, *js_ast.EUndefined:
						// "new WeakSet(null)" is pure
						// "new WeakSet(void 0)" is pure
						e.CanBeUnwrappedIfUnused = true

					case *js_ast.EArray:
						if len(arg.Items) == 0 {
							// "new WeakSet([])" is pure
							e.CanBeUnwrappedIfUnused = true
						} else {
							// "new WeakSet([x])" is impure because an exception is thrown if "x" is not an object
						}

					default:
						// "new WeakSet(x)" is impure because the iterator for "x" could have side effects
					}
				}

			case "Date":
				n := len(e.Args)

				if n == 0 {
					// "new Date()" is pure
					e.CanBeUnwrappedIfUnused = true
					break
				}

				if n == 1 {
					switch js_ast.KnownPrimitiveType(e.Args[0].Data) {
					case js_ast.PrimitiveNull, js_ast.PrimitiveUndefined, js_ast.PrimitiveBoolean, js_ast.PrimitiveNumber, js_ast.PrimitiveString:
						// "new Date('')" is pure
						// "new Date(0)" is pure
						// "new Date(null)" is pure
						// "new Date(true)" is pure
						// "new Date(false)" is pure
						// "new Date(undefined)" is pure
						e.CanBeUnwrappedIfUnused = true

					default:
						// "new Date(x)" is impure because converting "x" to a string could have side effects
					}
				}

			case "Set":
				n := len(e.Args)

				if n == 0 {
					// "new Set()" is pure
					e.CanBeUnwrappedIfUnused = true
					break
				}

				if n == 1 {
					switch e.Args[0].Data.(type) {
					case *js_ast.EArray, *js_ast.ENull, *js_ast.EUndefined:
						// "new Set([a, b, c])" is pure
						// "new Set(null)" is pure
						// "new Set(void 0)" is pure
						e.CanBeUnwrappedIfUnused = true

					default:
						// "new Set(x)" is impure because the iterator for "x" could have side effects
					}
				}

			case "Map":
				n := len(e.Args)

				if n == 0 {
					// "new Map()" is pure
					e.CanBeUnwrappedIfUnused = true
					break
				}

				if n == 1 {
					switch arg := e.Args[0].Data.(type) {
					case *js_ast.ENull, *js_ast.EUndefined:
						// "new Map(null)" is pure
						// "new Map(void 0)" is pure
						e.CanBeUnwrappedIfUnused = true

					case *js_ast.EArray:
						allEntriesAreArrays := true
						for _, item := range arg.Items {
							if _, ok := item.Data.(*js_ast.EArray); !ok {
								// "new Map([x])" is impure because "x[0]" could have side effects
								allEntriesAreArrays = false
								break
							}
						}

						// "new Map([[a, b], [c, d]])" is pure
						if allEntriesAreArrays {
							e.CanBeUnwrappedIfUnused = true
						}

					default:
						// "new Map(x)" is impure because the iterator for "x" could have side effects
					}
				}
			}
		}
	}
}

type identifierOpts struct {
	assignTarget            js_ast.AssignTarget
	isCallTarget            bool
	isDeleteTarget          bool
	preferQuotedKey         bool
	wasOriginallyIdentifier bool
	matchAgainstDefines     bool
}

func (p *parser) handleIdentifier(loc logger.Loc, e *js_ast.EIdentifier, opts identifierOpts) js_ast.Expr {
	ref := e.Ref

	// Substitute inlined constants
	if p.options.minifySyntax && !p.currentScope.ContainsDirectEval {
		if value, ok := p.constValues[ref]; ok {
			p.ignoreUsage(ref)
			return js_ast.ConstValueToExpr(loc, value)
		}
	}

	// Capture the "arguments" variable if necessary
	if p.fnOnlyDataVisit.argumentsRef != nil && ref == *p.fnOnlyDataVisit.argumentsRef {
		isInsideUnsupportedArrow := p.fnOrArrowDataVisit.isArrow && p.options.unsupportedJSFeatures.Has(compat.Arrow)
		isInsideUnsupportedAsyncArrow := p.fnOnlyDataVisit.isInsideAsyncArrowFn && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait)
		if isInsideUnsupportedArrow || isInsideUnsupportedAsyncArrow {
			return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.captureArguments()}}
		}
	}

	// Create an error for assigning to an import namespace
	if (opts.assignTarget != js_ast.AssignTargetNone ||
		(opts.isDeleteTarget && p.symbols[ref.InnerIndex].ImportItemStatus == ast.ImportItemGenerated)) &&
		p.symbols[ref.InnerIndex].Kind == ast.SymbolImport {
		r := js_lexer.RangeOfIdentifier(p.source, loc)

		// Try to come up with a setter name to try to make this message more understandable
		var setterHint string
		originalName := p.symbols[ref.InnerIndex].OriginalName
		if js_ast.IsIdentifier(originalName) && originalName != "_" {
			if len(originalName) == 1 || (len(originalName) > 1 && originalName[0] < utf8.RuneSelf) {
				setterHint = fmt.Sprintf(" (e.g. \"set%s%s\")", strings.ToUpper(originalName[:1]), originalName[1:])
			} else {
				setterHint = fmt.Sprintf(" (e.g. \"set_%s\")", originalName)
			}
		}

		notes := []logger.MsgData{{Text: "Imports are immutable in JavaScript. " +
			fmt.Sprintf("To modify the value of this import, you must export a setter function in the "+
				"imported file%s and then import and call that function here instead.", setterHint)}}

		if p.options.mode == config.ModeBundle {
			p.log.AddErrorWithNotes(&p.tracker, r, fmt.Sprintf("Cannot assign to import %q", originalName), notes)
		} else {
			kind := logger.Warning
			if p.suppressWarningsAboutWeirdCode {
				kind = logger.Debug
			}
			p.log.AddIDWithNotes(logger.MsgID_JS_AssignToImport, kind, &p.tracker, r,
				fmt.Sprintf("This assignment will throw because %q is an import", originalName), notes)
		}
	}

	// Substitute an EImportIdentifier now if this has a namespace alias
	if opts.assignTarget == js_ast.AssignTargetNone && !opts.isDeleteTarget {
		symbol := &p.symbols[ref.InnerIndex]
		if nsAlias := symbol.NamespaceAlias; nsAlias != nil {
			data := p.dotOrMangledPropVisit(
				js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: nsAlias.NamespaceRef}},
				symbol.OriginalName, loc)

			// Handle references to namespaces or namespace members
			if tsMemberData, ok := p.refToTSNamespaceMemberData[nsAlias.NamespaceRef]; ok {
				if ns, ok := tsMemberData.(*js_ast.TSNamespaceMemberNamespace); ok {
					if member, ok := ns.ExportedMembers[nsAlias.Alias]; ok {
						switch m := member.Data.(type) {
						case *js_ast.TSNamespaceMemberEnumNumber:
							return p.wrapInlinedEnum(js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: m.Value}}, nsAlias.Alias)

						case *js_ast.TSNamespaceMemberEnumString:
							return p.wrapInlinedEnum(js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: m.Value}}, nsAlias.Alias)

						case *js_ast.TSNamespaceMemberNamespace:
							p.tsNamespaceTarget = data
							p.tsNamespaceMemberData = member.Data
						}
					}
				}
			}

			return js_ast.Expr{Loc: loc, Data: data}
		}
	}

	// Substitute an EImportIdentifier now if this is an import item
	if p.isImportItem[ref] {
		return js_ast.Expr{Loc: loc, Data: &js_ast.EImportIdentifier{
			Ref:                     ref,
			PreferQuotedKey:         opts.preferQuotedKey,
			WasOriginallyIdentifier: opts.wasOriginallyIdentifier,
		}}
	}

	// Handle references to namespaces or namespace members
	if tsMemberData, ok := p.refToTSNamespaceMemberData[ref]; ok {
		switch m := tsMemberData.(type) {
		case *js_ast.TSNamespaceMemberEnumNumber:
			return p.wrapInlinedEnum(js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: m.Value}}, p.symbols[ref.InnerIndex].OriginalName)

		case *js_ast.TSNamespaceMemberEnumString:
			return p.wrapInlinedEnum(js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: m.Value}}, p.symbols[ref.InnerIndex].OriginalName)

		case *js_ast.TSNamespaceMemberNamespace:
			p.tsNamespaceTarget = e
			p.tsNamespaceMemberData = tsMemberData
		}
	}

	// Substitute a namespace export reference now if appropriate
	if p.options.ts.Parse {
		if nsRef, ok := p.isExportedInsideNamespace[ref]; ok {
			name := p.symbols[ref.InnerIndex].OriginalName

			// Otherwise, create a property access on the namespace
			p.recordUsage(nsRef)
			propertyAccess := p.dotOrMangledPropVisit(js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: nsRef}}, name, loc)
			if p.tsNamespaceTarget == e {
				p.tsNamespaceTarget = propertyAccess
			}
			return js_ast.Expr{Loc: loc, Data: propertyAccess}
		}
	}

	// Swap references to the global "require" function with our "__require" stub
	if ref == p.requireRef && !opts.isCallTarget {
		if p.options.mode == config.ModeBundle && p.source.Index != runtime.SourceIndex && e != p.dotOrIndexTarget {
			p.log.AddID(logger.MsgID_JS_IndirectRequire, logger.Debug, &p.tracker, js_lexer.RangeOfIdentifier(p.source, loc),
				"Indirect calls to \"require\" will not be bundled")
		}

		return p.valueToSubstituteForRequire(loc)
	}

	// Mark any mutated symbols as mutable
	if opts.assignTarget != js_ast.AssignTargetNone {
		p.symbols[e.Ref.InnerIndex].Flags |= ast.CouldPotentiallyBeMutated
	}

	return js_ast.Expr{Loc: loc, Data: e}
}

type visitFnOpts struct {
	isMethod               bool
	isDerivedClassCtor     bool
	isLoweredPrivateMethod bool
}

func (p *parser) visitFn(fn *js_ast.Fn, scopeLoc logger.Loc, opts visitFnOpts) {
	var decoratorScope *js_ast.Scope
	oldFnOrArrowData := p.fnOrArrowDataVisit
	oldFnOnlyData := p.fnOnlyDataVisit
	p.fnOrArrowDataVisit = fnOrArrowDataVisit{
		isAsync:                        fn.IsAsync,
		isGenerator:                    fn.IsGenerator,
		isDerivedClassCtor:             opts.isDerivedClassCtor,
		shouldLowerSuperPropertyAccess: (fn.IsAsync && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait)) || opts.isLoweredPrivateMethod,
	}
	p.fnOnlyDataVisit = fnOnlyDataVisit{
		isThisNested:       true,
		isNewTargetAllowed: true,
		argumentsRef:       &fn.ArgumentsRef,
	}

	if opts.isMethod {
		decoratorScope = p.propMethodDecoratorScope
		p.fnOnlyDataVisit.innerClassNameRef = oldFnOnlyData.innerClassNameRef
		p.fnOnlyDataVisit.isInStaticClassContext = oldFnOnlyData.isInStaticClassContext
	}

	if fn.Name != nil {
		p.recordDeclaredSymbol(fn.Name.Ref)
	}

	p.pushScopeForVisitPass(js_ast.ScopeFunctionArgs, scopeLoc)
	p.visitArgs(fn.Args, visitArgsOpts{
		hasRestArg:               fn.HasRestArg,
		body:                     fn.Body.Block.Stmts,
		isUniqueFormalParameters: fn.IsUniqueFormalParameters,
		decoratorScope:           decoratorScope,
	})
	p.pushScopeForVisitPass(js_ast.ScopeFunctionBody, fn.Body.Loc)
	if fn.Name != nil {
		p.validateDeclaredSymbolName(fn.Name.Loc, p.symbols[fn.Name.Ref.InnerIndex].OriginalName)
	}
	fn.Body.Block.Stmts = p.visitStmtsAndPrependTempRefs(fn.Body.Block.Stmts, prependTempRefsOpts{fnBodyLoc: &fn.Body.Loc, kind: stmtsFnBody})
	p.popScope()
	p.lowerFunction(&fn.IsAsync, &fn.IsGenerator, &fn.Args, fn.Body.Loc, &fn.Body.Block, nil, &fn.HasRestArg, false /* isArrow */)
	p.popScope()

	p.fnOrArrowDataVisit = oldFnOrArrowData
	p.fnOnlyDataVisit = oldFnOnlyData
}

func (p *parser) recordExport(loc logger.Loc, alias string, ref ast.Ref) {
	if name, ok := p.namedExports[alias]; ok {
		// Duplicate exports are an error
		p.log.AddErrorWithNotes(&p.tracker, js_lexer.RangeOfIdentifier(p.source, loc),
			fmt.Sprintf("Multiple exports with the same name %q", alias),
			[]logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, name.AliasLoc),
				fmt.Sprintf("The name %q was originally exported here:", alias))})
	} else {
		p.namedExports[alias] = js_ast.NamedExport{AliasLoc: loc, Ref: ref}
	}
}

type importsExportsScanResult struct {
	stmts               []js_ast.Stmt
	keptImportEquals    bool
	removedImportEquals bool
}

// Returns true if this is an unused TypeScript import-equals statement
func (p *parser) checkForUnusedTSImportEquals(s *js_ast.SLocal, result *importsExportsScanResult) bool {
	if s.WasTSImportEquals && !s.IsExport {
		decl := s.Decls[0]

		// Skip to the underlying reference
		value := s.Decls[0].ValueOrNil
		for {
			if dot, ok := value.Data.(*js_ast.EDot); ok {
				value = dot.Target
			} else {
				break
			}
		}

		// Is this an identifier reference and not a require() call?
		valueRef := ast.InvalidRef
		switch v := value.Data.(type) {
		case *js_ast.EIdentifier:
			valueRef = v.Ref
		case *js_ast.EImportIdentifier:
			valueRef = v.Ref
		}
		if valueRef != ast.InvalidRef {
			// Is this import statement unused?
			if ref := decl.Binding.Data.(*js_ast.BIdentifier).Ref; p.symbols[ref.InnerIndex].UseCountEstimate == 0 {
				// Also don't count the referenced identifier
				p.ignoreUsage(valueRef)

				// Import-equals statements can come in any order. Removing one
				// could potentially cause another one to be removable too.
				// Continue iterating until a fixed point has been reached to make
				// sure we get them all.
				result.removedImportEquals = true
				return true
			} else {
				result.keptImportEquals = true
			}
		}
	}

	return false
}

func (p *parser) scanForUnusedTSImportEquals(stmts []js_ast.Stmt) (result importsExportsScanResult) {
	stmtsEnd := 0

	for _, stmt := range stmts {
		if s, ok := stmt.Data.(*js_ast.SLocal); ok && p.checkForUnusedTSImportEquals(s, &result) {
			// Remove unused import-equals statements, since those likely
			// correspond to types instead of values
			continue
		}

		// Filter out statements we skipped over
		stmts[stmtsEnd] = stmt
		stmtsEnd++
	}

	result.stmts = stmts[:stmtsEnd]
	return
}

func (p *parser) scanForImportsAndExports(stmts []js_ast.Stmt) (result importsExportsScanResult) {
	unusedImportFlags := p.options.ts.Config.UnusedImportFlags()
	stmtsEnd := 0

	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SImport:
			record := &p.importRecords[s.ImportRecordIndex]

			// We implement TypeScript's "preserveValueImports" tsconfig.json setting
			// to support the use case of compiling partial modules for compile-to-
			// JavaScript languages such as Svelte. These languages try to reference
			// imports in ways that are impossible for TypeScript and esbuild to know
			// about when they are only given a partial module to compile. Here is an
			// example of some Svelte code that contains a TypeScript snippet:
			//
			//   <script lang="ts">
			//     import Counter from './Counter.svelte';
			//     export let name: string = 'world';
			//   </script>
			//   <main>
			//     <h1>Hello {name}!</h1>
			//     <Counter />
			//   </main>
			//
			// Tools that use esbuild to compile TypeScript code inside a Svelte
			// file like this only give esbuild the contents of the <script> tag.
			// The "preserveValueImports" setting avoids removing unused import
			// names, which means additional code appended after the TypeScript-
			// to-JavaScript conversion can still access those unused imports.
			//
			// There are two scenarios where we don't do this:
			//
			//   * If we're bundling, then we know we aren't being used to compile
			//     a partial module. The parser is seeing the entire code for the
			//     module so it's safe to remove unused imports. And also we don't
			//     want the linker to generate errors about missing imports if the
			//     imported file is also in the bundle.
			//
			//   * If identifier minification is enabled, then using esbuild as a
			//     partial-module transform library wouldn't work anyway because
			//     the names wouldn't match. And that means we're minifying so the
			//     user is expecting the output to be as small as possible. So we
			//     should omit unused imports.
			//
			keepUnusedImports := p.options.ts.Parse && (unusedImportFlags&config.TSUnusedImport_KeepValues) != 0 &&
				p.options.mode != config.ModeBundle && !p.options.minifyIdentifiers

			// Forbid non-default imports for JSON import assertions
			if (record.Flags&ast.AssertTypeJSON) != 0 && p.options.mode == config.ModeBundle && s.Items != nil {
				for _, item := range *s.Items {
					if p.options.ts.Parse && p.tsUseCounts[item.Name.Ref.InnerIndex] == 0 && (unusedImportFlags&config.TSUnusedImport_KeepValues) == 0 {
						// Do not count imports that TypeScript interprets as type annotations
						continue
					}
					if item.Alias != "default" {
						p.log.AddErrorWithNotes(&p.tracker, js_lexer.RangeOfIdentifier(p.source, item.AliasLoc),
							fmt.Sprintf("Cannot use non-default import %q with a JSON import assertion", item.Alias),
							p.notesForAssertTypeJSON(record, item.Alias))
					}
				}
			}

			// TypeScript always trims unused imports. This is important for
			// correctness since some imports might be fake (only in the type
			// system and used for type-only imports).
			if (p.options.minifySyntax || p.options.ts.Parse) && !keepUnusedImports {
				foundImports := false
				isUnusedInTypeScript := true

				// Remove the default name if it's unused
				if s.DefaultName != nil {
					foundImports = true
					symbol := p.symbols[s.DefaultName.Ref.InnerIndex]

					// TypeScript has a separate definition of unused
					if p.options.ts.Parse && (p.tsUseCounts[s.DefaultName.Ref.InnerIndex] != 0 || (p.options.ts.Config.UnusedImportFlags()&config.TSUnusedImport_KeepValues) != 0) {
						isUnusedInTypeScript = false
					}

					// Remove the symbol if it's never used outside a dead code region
					if symbol.UseCountEstimate == 0 && (p.options.ts.Parse || !p.moduleScope.ContainsDirectEval) {
						s.DefaultName = nil
					}
				}

				// Remove the star import if it's unused
				if s.StarNameLoc != nil {
					foundImports = true
					symbol := p.symbols[s.NamespaceRef.InnerIndex]

					// TypeScript has a separate definition of unused
					if p.options.ts.Parse && (p.tsUseCounts[s.NamespaceRef.InnerIndex] != 0 || (p.options.ts.Config.UnusedImportFlags()&config.TSUnusedImport_KeepValues) != 0) {
						isUnusedInTypeScript = false
					}

					// Remove the symbol if it's never used outside a dead code region
					if symbol.UseCountEstimate == 0 && (p.options.ts.Parse || !p.moduleScope.ContainsDirectEval) {
						// Make sure we don't remove this if it was used for a property
						// access while bundling
						if importItems, ok := p.importItemsForNamespace[s.NamespaceRef]; ok && len(importItems.entries) == 0 {
							s.StarNameLoc = nil
						}
					}
				}

				// Remove items if they are unused
				if s.Items != nil {
					foundImports = true
					itemsEnd := 0

					for _, item := range *s.Items {
						symbol := p.symbols[item.Name.Ref.InnerIndex]

						// TypeScript has a separate definition of unused
						if p.options.ts.Parse && (p.tsUseCounts[item.Name.Ref.InnerIndex] != 0 || (p.options.ts.Config.UnusedImportFlags()&config.TSUnusedImport_KeepValues) != 0) {
							isUnusedInTypeScript = false
						}

						// Remove the symbol if it's never used outside a dead code region
						if symbol.UseCountEstimate != 0 || (!p.options.ts.Parse && p.moduleScope.ContainsDirectEval) {
							(*s.Items)[itemsEnd] = item
							itemsEnd++
						}
					}

					// Filter the array by taking a slice
					if itemsEnd == 0 {
						s.Items = nil
					} else {
						*s.Items = (*s.Items)[:itemsEnd]
					}
				}

				// Omit this statement if we're parsing TypeScript and all imports are
				// unused. Note that this is distinct from the case where there were
				// no imports at all (e.g. "import 'foo'"). In that case we want to keep
				// the statement because the user is clearly trying to import the module
				// for side effects.
				//
				// This culling is important for correctness when parsing TypeScript
				// because a) the TypeScript compiler does ths and we want to match it
				// and b) this may be a fake module that only exists in the type system
				// and doesn't actually exist in reality.
				//
				// We do not want to do this culling in JavaScript though because the
				// module may have side effects even if all imports are unused.
				if p.options.ts.Parse && foundImports && isUnusedInTypeScript && (unusedImportFlags&config.TSUnusedImport_KeepStmt) == 0 {
					// Ignore import records with a pre-filled source index. These are
					// for injected files and we definitely do not want to trim these.
					if !record.SourceIndex.IsValid() && !record.CopySourceIndex.IsValid() {
						record.Flags |= ast.IsUnused
						continue
					}
				}
			}

			if p.options.mode != config.ModePassThrough {
				if s.StarNameLoc != nil {
					// "importItemsForNamespace" has property accesses off the namespace
					if importItems, ok := p.importItemsForNamespace[s.NamespaceRef]; ok && len(importItems.entries) > 0 {
						// Sort keys for determinism
						sorted := make([]string, 0, len(importItems.entries))
						for alias := range importItems.entries {
							sorted = append(sorted, alias)
						}
						sort.Strings(sorted)

						// Create named imports for these property accesses. This will
						// cause missing imports to generate useful warnings.
						//
						// It will also improve bundling efficiency for internal imports
						// by still converting property accesses off the namespace into
						// bare identifiers even if the namespace is still needed.
						for _, alias := range sorted {
							name := importItems.entries[alias]
							p.namedImports[name.Ref] = js_ast.NamedImport{
								Alias:             alias,
								AliasLoc:          name.Loc,
								NamespaceRef:      s.NamespaceRef,
								ImportRecordIndex: s.ImportRecordIndex,
							}

							// Make sure the printer prints this as a property access
							p.symbols[name.Ref.InnerIndex].NamespaceAlias = &ast.NamespaceAlias{
								NamespaceRef: s.NamespaceRef,
								Alias:        alias,
							}

							// Also record these automatically-generated top-level namespace alias symbols
							p.declaredSymbols = append(p.declaredSymbols, js_ast.DeclaredSymbol{
								Ref:        name.Ref,
								IsTopLevel: true,
							})
						}
					}
				}

				if s.DefaultName != nil {
					p.namedImports[s.DefaultName.Ref] = js_ast.NamedImport{
						Alias:             "default",
						AliasLoc:          s.DefaultName.Loc,
						NamespaceRef:      s.NamespaceRef,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				}

				if s.StarNameLoc != nil {
					p.namedImports[s.NamespaceRef] = js_ast.NamedImport{
						AliasIsStar:       true,
						AliasLoc:          *s.StarNameLoc,
						NamespaceRef:      ast.InvalidRef,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				}

				if s.Items != nil {
					for _, item := range *s.Items {
						p.namedImports[item.Name.Ref] = js_ast.NamedImport{
							Alias:             item.Alias,
							AliasLoc:          item.AliasLoc,
							NamespaceRef:      s.NamespaceRef,
							ImportRecordIndex: s.ImportRecordIndex,
						}
					}
				}
			}

			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			if s.StarNameLoc != nil {
				record.Flags |= ast.ContainsImportStar
			}

			if s.DefaultName != nil {
				record.Flags |= ast.ContainsDefaultAlias
			} else if s.Items != nil {
				for _, item := range *s.Items {
					if item.Alias == "default" {
						record.Flags |= ast.ContainsDefaultAlias
					} else if item.Alias == "__esModule" {
						record.Flags |= ast.ContainsESModuleAlias
					}
				}
			}

		case *js_ast.SFunction:
			if s.IsExport {
				p.recordExport(s.Fn.Name.Loc, p.symbols[s.Fn.Name.Ref.InnerIndex].OriginalName, s.Fn.Name.Ref)
			}

		case *js_ast.SClass:
			if s.IsExport {
				p.recordExport(s.Class.Name.Loc, p.symbols[s.Class.Name.Ref.InnerIndex].OriginalName, s.Class.Name.Ref)
			}

		case *js_ast.SLocal:
			if s.IsExport {
				js_ast.ForEachIdentifierBindingInDecls(s.Decls, func(loc logger.Loc, b *js_ast.BIdentifier) {
					p.recordExport(loc, p.symbols[b.Ref.InnerIndex].OriginalName, b.Ref)
				})
			}

			// Remove unused import-equals statements, since those likely
			// correspond to types instead of values
			if p.checkForUnusedTSImportEquals(s, &result) {
				continue
			}

		case *js_ast.SExportDefault:
			p.recordExport(s.DefaultName.Loc, "default", s.DefaultName.Ref)

		case *js_ast.SExportClause:
			for _, item := range s.Items {
				p.recordExport(item.AliasLoc, item.Alias, item.Name.Ref)
			}

		case *js_ast.SExportStar:
			record := &p.importRecords[s.ImportRecordIndex]
			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			if s.Alias != nil {
				// "export * as ns from 'path'"
				p.namedImports[s.NamespaceRef] = js_ast.NamedImport{
					AliasIsStar:       true,
					AliasLoc:          s.Alias.Loc,
					NamespaceRef:      ast.InvalidRef,
					ImportRecordIndex: s.ImportRecordIndex,
					IsExported:        true,
				}
				p.recordExport(s.Alias.Loc, s.Alias.OriginalName, s.NamespaceRef)

				record.Flags |= ast.ContainsImportStar
			} else {
				// "export * from 'path'"
				p.exportStarImportRecords = append(p.exportStarImportRecords, s.ImportRecordIndex)
			}

		case *js_ast.SExportFrom:
			record := &p.importRecords[s.ImportRecordIndex]
			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			for _, item := range s.Items {
				// Note that the imported alias is not item.Alias, which is the
				// exported alias. This is somewhat confusing because each
				// SExportFrom statement is basically SImport + SExportClause in one.
				p.namedImports[item.Name.Ref] = js_ast.NamedImport{
					Alias:             item.OriginalName,
					AliasLoc:          item.Name.Loc,
					NamespaceRef:      s.NamespaceRef,
					ImportRecordIndex: s.ImportRecordIndex,
					IsExported:        true,
				}
				p.recordExport(item.Name.Loc, item.Alias, item.Name.Ref)

				if item.OriginalName == "default" {
					record.Flags |= ast.ContainsDefaultAlias
				} else if item.OriginalName == "__esModule" {
					record.Flags |= ast.ContainsESModuleAlias
				}
			}

			// Forbid non-default imports for JSON import assertions
			if (record.Flags&ast.AssertTypeJSON) != 0 && p.options.mode == config.ModeBundle {
				for _, item := range s.Items {
					if item.OriginalName != "default" {
						p.log.AddErrorWithNotes(&p.tracker, js_lexer.RangeOfIdentifier(p.source, item.Name.Loc),
							fmt.Sprintf("Cannot use non-default import %q with a JSON import assertion", item.OriginalName),
							p.notesForAssertTypeJSON(record, item.OriginalName))
					}
				}
			}

			// TypeScript always trims unused re-exports. This is important for
			// correctness since some re-exports might be fake (only in the type
			// system and used for type-only stuff).
			if p.options.ts.Parse && len(s.Items) == 0 && (unusedImportFlags&config.TSUnusedImport_KeepStmt) == 0 {
				continue
			}
		}

		// Filter out statements we skipped over
		stmts[stmtsEnd] = stmt
		stmtsEnd++
	}

	result.stmts = stmts[:stmtsEnd]
	return
}

func (p *parser) appendPart(parts []js_ast.Part, stmts []js_ast.Stmt) []js_ast.Part {
	p.symbolUses = make(map[ast.Ref]js_ast.SymbolUse)
	p.importSymbolPropertyUses = nil
	p.symbolCallUses = nil
	p.declaredSymbols = nil
	p.importRecordsForCurrentPart = nil
	p.scopesForCurrentPart = nil

	part := js_ast.Part{
		Stmts:      p.visitStmtsAndPrependTempRefs(stmts, prependTempRefsOpts{}),
		SymbolUses: p.symbolUses,
	}

	// Sanity check
	if p.currentScope != p.moduleScope {
		panic("Internal error: Scope stack imbalance")
	}

	// Insert any relocated variable statements now
	if len(p.relocatedTopLevelVars) > 0 {
		alreadyDeclared := make(map[ast.Ref]bool)
		for _, local := range p.relocatedTopLevelVars {
			// Follow links because "var" declarations may be merged due to hoisting
			for {
				link := p.symbols[local.Ref.InnerIndex].Link
				if link == ast.InvalidRef {
					break
				}
				local.Ref = link
			}

			// Only declare a given relocated variable once
			if !alreadyDeclared[local.Ref] {
				alreadyDeclared[local.Ref] = true
				part.Stmts = append(part.Stmts, js_ast.Stmt{Loc: local.Loc, Data: &js_ast.SLocal{
					Decls: []js_ast.Decl{{
						Binding: js_ast.Binding{Loc: local.Loc, Data: &js_ast.BIdentifier{Ref: local.Ref}},
					}},
				}})
			}
		}
		p.relocatedTopLevelVars = nil
	}

	if len(part.Stmts) > 0 {
		var flags js_ast.StmtsCanBeRemovedIfUnusedFlags
		if p.options.mode == config.ModePassThrough {
			// Exports are tracked separately, so export clauses can normally always
			// be removed. Except we should keep them if we're not doing any format
			// conversion because exports are not re-emitted in that case.
			flags |= js_ast.KeepExportClauses
		}
		part.CanBeRemovedIfUnused = p.astHelpers.StmtsCanBeRemovedIfUnused(part.Stmts, flags)
		part.DeclaredSymbols = p.declaredSymbols
		part.ImportRecordIndices = p.importRecordsForCurrentPart
		part.ImportSymbolPropertyUses = p.importSymbolPropertyUses
		part.SymbolCallUses = p.symbolCallUses
		part.Scopes = p.scopesForCurrentPart
		parts = append(parts, part)
	}
	return parts
}

func newParser(log logger.Log, source logger.Source, lexer js_lexer.Lexer, options *Options) *parser {
	if options.defines == nil {
		defaultDefines := config.ProcessDefines(nil)
		options.defines = &defaultDefines
	}

	p := &parser{
		log:                log,
		source:             source,
		tracker:            logger.MakeLineColumnTracker(&source),
		lexer:              lexer,
		allowIn:            true,
		options:            *options,
		runtimeImports:     make(map[string]ast.LocRef),
		promiseRef:         ast.InvalidRef,
		regExpRef:          ast.InvalidRef,
		bigIntRef:          ast.InvalidRef,
		afterArrowBodyLoc:  logger.Loc{Start: -1},
		firstJSXElementLoc: logger.Loc{Start: -1},
		importMetaRef:      ast.InvalidRef,
		superCtorRef:       ast.InvalidRef,

		// For lowering private methods
		weakMapRef:     ast.InvalidRef,
		weakSetRef:     ast.InvalidRef,
		privateGetters: make(map[ast.Ref]ast.Ref),
		privateSetters: make(map[ast.Ref]ast.Ref),

		// These are for TypeScript
		refToTSNamespaceMemberData: make(map[ast.Ref]js_ast.TSNamespaceMemberData),
		emittedNamespaceVars:       make(map[ast.Ref]bool),
		isExportedInsideNamespace:  make(map[ast.Ref]ast.Ref),
		localTypeNames:             make(map[string]bool),

		// These are for handling ES6 imports and exports
		importItemsForNamespace: make(map[ast.Ref]namespaceImportItems),
		isImportItem:            make(map[ast.Ref]bool),
		namedImports:            make(map[ast.Ref]js_ast.NamedImport),
		namedExports:            make(map[string]js_ast.NamedExport),

		// For JSX runtime imports
		jsxRuntimeImports: make(map[string]ast.LocRef),
		jsxLegacyImports:  make(map[string]ast.LocRef),

		// Add "/* @__KEY__ */" comments when mangling properties to support
		// running esbuild (or other tools like Terser) again on the output.
		// This checks both "--mangle-props" and "--reserve-props" so that
		// you can turn this on with just "--reserve-props=." if you want to.
		shouldAddKeyComment: options.mangleProps != nil || options.reserveProps != nil,

		suppressWarningsAboutWeirdCode: helpers.IsInsideNodeModules(source.KeyPath.Text),
	}

	if len(options.dropLabels) > 0 {
		p.dropLabelsMap = make(map[string]struct{})
		for _, name := range options.dropLabels {
			p.dropLabelsMap[name] = struct{}{}
		}
	}

	if !options.minifyWhitespace {
		p.exprComments = make(map[logger.Loc][]string)
	}

	p.astHelpers = js_ast.MakeHelperContext(func(ref ast.Ref) bool {
		return p.symbols[ref.InnerIndex].Kind == ast.SymbolUnbound
	})

	p.pushScopeForParsePass(js_ast.ScopeEntry, logger.Loc{Start: locModuleScope})

	return p
}

var defaultJSXFactory = []string{"React", "createElement"}
var defaultJSXFragment = []string{"React", "Fragment"}

const defaultJSXImportSource = "react"

func Parse(log logger.Log, source logger.Source, options Options) (result js_ast.AST, ok bool) {
	ok = true
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			ok = false
		} else if r != nil {
			panic(r)
		}
	}()

	// Default options for JSX elements
	if len(options.jsx.Factory.Parts) == 0 {
		options.jsx.Factory = config.DefineExpr{Parts: defaultJSXFactory}
	}
	if len(options.jsx.Fragment.Parts) == 0 && options.jsx.Fragment.Constant == nil {
		options.jsx.Fragment = config.DefineExpr{Parts: defaultJSXFragment}
	}
	if len(options.jsx.ImportSource) == 0 {
		options.jsx.ImportSource = defaultJSXImportSource
	}

	p := newParser(log, source, js_lexer.NewLexer(log, source, options.ts), &options)

	// Consume a leading hashbang comment
	hashbang := ""
	if p.lexer.Token == js_lexer.THashbang {
		hashbang = p.lexer.Identifier.String
		p.lexer.Next()
	}

	// Allow top-level await
	p.fnOrArrowDataParse.await = allowExpr
	p.fnOrArrowDataParse.isTopLevel = true

	// Parse the file in the first pass, but do not bind symbols
	stmts := p.parseStmtsUpTo(js_lexer.TEndOfFile, parseStmtOpts{
		isModuleScope:          true,
		allowDirectivePrologue: true,
	})
	p.prepareForVisitPass()

	// Insert a "use strict" directive if "alwaysStrict" is active
	var directives []string
	if tsAlwaysStrict := p.options.tsAlwaysStrict; tsAlwaysStrict != nil && tsAlwaysStrict.Value {
		directives = append(directives, "use strict")
	}

	// Strip off all leading directives
	{
		totalCount := 0
		keptCount := 0

		for _, stmt := range stmts {
			switch s := stmt.Data.(type) {
			case *js_ast.SComment:
				stmts[keptCount] = stmt
				keptCount++
				totalCount++
				continue

			case *js_ast.SDirective:
				if p.isStrictMode() && s.LegacyOctalLoc.Start > 0 {
					p.markStrictModeFeature(legacyOctalEscape, p.source.RangeOfLegacyOctalEscape(s.LegacyOctalLoc), "")
				}
				directive := helpers.UTF16ToString(s.Value)

				// Remove duplicate directives
				found := false
				for _, existing := range directives {
					if existing == directive {
						found = true
						break
					}
				}
				if !found {
					directives = append(directives, directive)
				}

				// Remove this directive from the statement list
				totalCount++
				continue
			}

			// Stop when the directive prologue ends
			break
		}

		if keptCount < totalCount {
			stmts = append(stmts[:keptCount], stmts[totalCount:]...)
		}
	}

	// Add an empty part for the namespace export that we can fill in later
	nsExportPart := js_ast.Part{
		SymbolUses:           make(map[ast.Ref]js_ast.SymbolUse),
		CanBeRemovedIfUnused: true,
	}

	var before = []js_ast.Part{nsExportPart}
	var parts []js_ast.Part
	var after []js_ast.Part

	// Insert any injected import statements now that symbols have been declared
	for _, file := range p.options.injectedFiles {
		exportsNoConflict := make([]string, 0, len(file.Exports))
		symbols := make(map[string]ast.LocRef)

		if file.DefineName != "" {
			ref := p.newSymbol(ast.SymbolOther, file.DefineName)
			p.moduleScope.Generated = append(p.moduleScope.Generated, ref)
			symbols["default"] = ast.LocRef{Ref: ref}
			exportsNoConflict = append(exportsNoConflict, "default")
			p.injectedDefineSymbols = append(p.injectedDefineSymbols, ref)
		} else {
		nextExport:
			for _, export := range file.Exports {
				// Skip injecting this symbol if it's already declared locally (i.e. it's not a reference to a global)
				if _, ok := p.moduleScope.Members[export.Alias]; ok {
					continue
				}

				parts := strings.Split(export.Alias, ".")

				// The key must be a dot-separated identifier list
				for _, part := range parts {
					if !js_ast.IsIdentifier(part) {
						continue nextExport
					}
				}

				ref := p.newSymbol(ast.SymbolInjected, export.Alias)
				symbols[export.Alias] = ast.LocRef{Ref: ref}
				if len(parts) == 1 {
					// Handle the identifier case by generating an injected symbol directly
					p.moduleScope.Members[export.Alias] = js_ast.ScopeMember{Ref: ref}
				} else {
					// Handle the dot case using a map. This map is similar to the map
					// "options.defines.DotDefines" but is kept separate instead of being
					// implemented using the same mechanism because we allow you to use
					// "define" to rewrite something to an injected symbol (i.e. we allow
					// two levels of mappings). This was historically necessary to be able
					// to map a dot name to an injected symbol because we previously didn't
					// support dot names as injected symbols. But now dot names as injected
					// symbols has been implemented, so supporting two levels of mappings
					// is only for backward-compatibility.
					if p.injectedDotNames == nil {
						p.injectedDotNames = make(map[string][]injectedDotName)
					}
					tail := parts[len(parts)-1]
					p.injectedDotNames[tail] = append(p.injectedDotNames[tail], injectedDotName{parts: parts, injectedDefineIndex: uint32(len(p.injectedDefineSymbols))})
					p.injectedDefineSymbols = append(p.injectedDefineSymbols, ref)
				}
				exportsNoConflict = append(exportsNoConflict, export.Alias)
				if p.injectedSymbolSources == nil {
					p.injectedSymbolSources = make(map[ast.Ref]injectedSymbolSource)
				}
				p.injectedSymbolSources[ref] = injectedSymbolSource{
					source: file.Source,
					loc:    export.Loc,
				}
			}
		}

		if file.IsCopyLoader {
			before, _ = p.generateImportStmt(file.Source.KeyPath.Text, logger.Range{}, exportsNoConflict, before, symbols, nil, &file.Source.Index)
		} else {
			before, _ = p.generateImportStmt(file.Source.KeyPath.Text, logger.Range{}, exportsNoConflict, before, symbols, &file.Source.Index, nil)
		}
	}

	// When "using" declarations appear at the top level, we change all TDZ
	// variables in the top-level scope into "var" so that they aren't harmed
	// when they are moved into the try/catch statement that lowering will
	// generate.
	//
	// This is necessary because exported function declarations must be hoisted
	// outside of the try/catch statement because they can be evaluated before
	// this module is evaluated due to ESM cross-file function hoisting. And
	// these function bodies might reference anything else in this scope, which
	// must still work when those things are moved inside a try/catch statement.
	//
	// Before:
	//
	//   using foo = get()
	//   export function fn() {
	//     return [foo, new Bar]
	//   }
	//   class Bar {}
	//
	// After ("fn" is hoisted, "Bar" is converted to "var"):
	//
	//   export function fn() {
	//     return [foo, new Bar]
	//   }
	//   try {
	//     var foo = get();
	//     var Bar = class {};
	//   } catch (_) {
	//     ...
	//   } finally {
	//     ...
	//   }
	//
	// This is also necessary because other code might be appended to the code
	// that we're processing and expect to be able to access top-level variables.
	p.willWrapModuleInTryCatchForUsing = p.shouldLowerUsingDeclarations(stmts)

	// Bind symbols in a second pass over the AST. I started off doing this in a
	// single pass, but it turns out it's pretty much impossible to do this
	// correctly while handling arrow functions because of the grammar
	// ambiguities.
	//
	// Note that top-level lowered "using" declarations disable tree-shaking
	// because we only do tree-shaking on top-level statements and lowering
	// a top-level "using" declaration moves all top-level statements into a
	// nested scope.
	if !p.options.treeShaking || p.willWrapModuleInTryCatchForUsing {
		// When tree shaking is disabled, everything comes in a single part
		parts = p.appendPart(parts, stmts)
	} else {
		var preprocessedEnums map[int][]js_ast.Part
		if p.scopesInOrderForEnum != nil {
			// Preprocess TypeScript enums to improve code generation. Otherwise
			// uses of an enum before that enum has been declared won't be inlined:
			//
			//   console.log(Foo.FOO) // We want "FOO" to be inlined here
			//   const enum Foo { FOO = 0 }
			//
			// The TypeScript compiler itself contains code with this pattern, so
			// it's important to implement this optimization.
			for i, stmt := range stmts {
				if _, ok := stmt.Data.(*js_ast.SEnum); ok {
					if preprocessedEnums == nil {
						preprocessedEnums = make(map[int][]js_ast.Part)
					}
					oldScopesInOrder := p.scopesInOrder
					p.scopesInOrder = p.scopesInOrderForEnum[stmt.Loc]
					preprocessedEnums[i] = p.appendPart(nil, []js_ast.Stmt{stmt})
					p.scopesInOrder = oldScopesInOrder
				}
			}
		}

		// When tree shaking is enabled, each top-level statement is potentially a separate part
		for i, stmt := range stmts {
			switch s := stmt.Data.(type) {
			case *js_ast.SLocal:
				// Split up top-level multi-declaration variable statements
				for _, decl := range s.Decls {
					clone := *s
					clone.Decls = []js_ast.Decl{decl}
					parts = p.appendPart(parts, []js_ast.Stmt{{Loc: stmt.Loc, Data: &clone}})
				}

			case *js_ast.SImport, *js_ast.SExportFrom, *js_ast.SExportStar:
				if p.options.mode != config.ModePassThrough {
					// Move imports (and import-like exports) to the top of the file to
					// ensure that if they are converted to a require() call, the effects
					// will take place before any other statements are evaluated.
					before = p.appendPart(before, []js_ast.Stmt{stmt})
				} else {
					// If we aren't doing any format conversion, just keep these statements
					// inline where they were. Exports are sorted so order doesn't matter:
					// https://262.ecma-international.org/6.0/#sec-module-namespace-exotic-objects.
					// However, this is likely an aesthetic issue that some people will
					// complain about. In addition, there are code transformation tools
					// such as TypeScript and Babel with bugs where the order of exports
					// in the file is incorrectly preserved instead of sorted, so preserving
					// the order of exports ourselves here may be preferable.
					parts = p.appendPart(parts, []js_ast.Stmt{stmt})
				}

			case *js_ast.SExportEquals:
				// TypeScript "export = value;" becomes "module.exports = value;". This
				// must happen at the end after everything is parsed because TypeScript
				// moves this statement to the end when it generates code.
				after = p.appendPart(after, []js_ast.Stmt{stmt})

			case *js_ast.SEnum:
				parts = append(parts, preprocessedEnums[i]...)
				p.scopesInOrder = p.scopesInOrder[len(p.scopesInOrderForEnum[stmt.Loc]):]

			default:
				parts = p.appendPart(parts, []js_ast.Stmt{stmt})
			}
		}
	}

	// Insert a variable for "import.meta" at the top of the file if it was used.
	// We don't need to worry about "use strict" directives because this only
	// happens when bundling, in which case we are flatting the module scopes of
	// all modules together anyway so such directives are meaningless.
	if p.importMetaRef != ast.InvalidRef {
		importMetaStmt := js_ast.Stmt{Data: &js_ast.SLocal{
			Kind: p.selectLocalKind(js_ast.LocalConst),
			Decls: []js_ast.Decl{{
				Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: p.importMetaRef}},
				ValueOrNil: js_ast.Expr{Data: &js_ast.EObject{}},
			}},
		}}
		before = append(before, js_ast.Part{
			Stmts:                []js_ast.Stmt{importMetaStmt},
			SymbolUses:           make(map[ast.Ref]js_ast.SymbolUse),
			DeclaredSymbols:      []js_ast.DeclaredSymbol{{Ref: p.importMetaRef, IsTopLevel: true}},
			CanBeRemovedIfUnused: true,
		})
	}

	// Pop the module scope to apply the "ContainsDirectEval" rules
	p.popScope()

	result = p.toAST(before, parts, after, hashbang, directives)
	result.SourceMapComment = p.lexer.SourceMappingURL
	return
}

func LazyExportAST(log logger.Log, source logger.Source, options Options, expr js_ast.Expr, apiCall string) js_ast.AST {
	// Don't create a new lexer using js_lexer.NewLexer() here since that will
	// actually attempt to parse the first token, which might cause a syntax
	// error.
	p := newParser(log, source, js_lexer.Lexer{}, &options)
	p.prepareForVisitPass()

	// Optionally call a runtime API function to transform the expression
	if apiCall != "" {
		p.symbolUses = make(map[ast.Ref]js_ast.SymbolUse)
		expr = p.callRuntime(expr.Loc, apiCall, []js_ast.Expr{expr})
	}

	// Add an empty part for the namespace export that we can fill in later
	nsExportPart := js_ast.Part{
		SymbolUses:           make(map[ast.Ref]js_ast.SymbolUse),
		CanBeRemovedIfUnused: true,
	}

	// Defer the actual code generation until linking
	part := js_ast.Part{
		Stmts:      []js_ast.Stmt{{Loc: expr.Loc, Data: &js_ast.SLazyExport{Value: expr}}},
		SymbolUses: p.symbolUses,
	}
	p.symbolUses = nil

	ast := p.toAST([]js_ast.Part{nsExportPart}, []js_ast.Part{part}, nil, "", nil)
	ast.HasLazyExport = true
	return ast
}

func GlobResolveAST(log logger.Log, source logger.Source, importRecords []ast.ImportRecord, object *js_ast.EObject, name string) js_ast.AST {
	// Don't create a new lexer using js_lexer.NewLexer() here since that will
	// actually attempt to parse the first token, which might cause a syntax
	// error.
	p := newParser(log, source, js_lexer.Lexer{}, &Options{})
	p.prepareForVisitPass()

	// Add an empty part for the namespace export that we can fill in later
	nsExportPart := js_ast.Part{
		SymbolUses:           make(map[ast.Ref]js_ast.SymbolUse),
		CanBeRemovedIfUnused: true,
	}

	if len(p.importRecords) != 0 {
		panic("Internal error")
	}
	p.importRecords = importRecords

	importRecordIndices := make([]uint32, 0, len(importRecords))
	for importRecordIndex := range importRecords {
		importRecordIndices = append(importRecordIndices, uint32(importRecordIndex))
	}

	p.symbolUses = make(map[ast.Ref]js_ast.SymbolUse)
	ref := p.newSymbol(ast.SymbolOther, name)
	p.moduleScope.Generated = append(p.moduleScope.Generated, ref)

	part := js_ast.Part{
		Stmts: []js_ast.Stmt{{Data: &js_ast.SLocal{
			IsExport: true,
			Decls: []js_ast.Decl{{
				Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: ref}},
				ValueOrNil: p.callRuntime(logger.Loc{}, "__glob", []js_ast.Expr{{Data: object}}),
			}},
		}}},
		ImportRecordIndices: importRecordIndices,
		SymbolUses:          p.symbolUses,
	}
	p.symbolUses = nil

	p.esmExportKeyword.Len = 1
	return p.toAST([]js_ast.Part{nsExportPart}, []js_ast.Part{part}, nil, "", nil)
}

func ParseDefineExpr(text string) (config.DefineExpr, js_ast.E) {
	if text == "" {
		return config.DefineExpr{}, nil
	}

	// Try a property chain
	parts := strings.Split(text, ".")
	for i, part := range parts {
		if !js_ast.IsIdentifier(part) {
			parts = nil
			break
		}

		// Don't allow most keywords as the identifier
		if i == 0 {
			if token, ok := js_lexer.Keywords[part]; ok && token != js_lexer.TNull && token != js_lexer.TThis &&
				(token != js_lexer.TImport || len(parts) < 2 || parts[1] != "meta") {
				parts = nil
				break
			}
		}
	}
	if parts != nil {
		return config.DefineExpr{Parts: parts}, nil
	}

	// Try parsing a value
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	expr, ok := ParseJSON(log, logger.Source{Contents: text}, JSONOptions{
		IsForDefine: true,
	})
	if !ok {
		return config.DefineExpr{}, nil
	}

	// Only primitive literals are inlined directly
	switch expr.Data.(type) {
	case *js_ast.ENull, *js_ast.EBoolean, *js_ast.EString, *js_ast.ENumber, *js_ast.EBigInt:
		return config.DefineExpr{Constant: expr.Data}, nil
	}

	// If it's not a primitive, return the whole compound JSON value to be injected out-of-line
	return config.DefineExpr{}, expr.Data
}

type whyESM uint8

const (
	whyESMUnknown whyESM = iota
	whyESMExportKeyword
	whyESMImportMeta
	whyESMTopLevelAwait
	whyESMFileMJS
	whyESMFileMTS
	whyESMTypeModulePackageJSON
	whyESMImportStatement
)

// Say why this the current file is being considered an ES module
func (p *parser) whyESModule() (whyESM, []logger.MsgData) {
	because := "This file is considered to be an ECMAScript module because"
	switch {
	case p.esmExportKeyword.Len > 0:
		return whyESMExportKeyword, []logger.MsgData{p.tracker.MsgData(p.esmExportKeyword,
			because+" of the \"export\" keyword here:")}

	case p.esmImportMeta.Len > 0:
		return whyESMImportMeta, []logger.MsgData{p.tracker.MsgData(p.esmImportMeta,
			because+" of the use of \"import.meta\" here:")}

	case p.topLevelAwaitKeyword.Len > 0:
		return whyESMTopLevelAwait, []logger.MsgData{p.tracker.MsgData(p.topLevelAwaitKeyword,
			because+" of the top-level \"await\" keyword here:")}

	case p.options.moduleTypeData.Type == js_ast.ModuleESM_MJS:
		return whyESMFileMJS, []logger.MsgData{{Text: because + " the file name ends in \".mjs\"."}}

	case p.options.moduleTypeData.Type == js_ast.ModuleESM_MTS:
		return whyESMFileMTS, []logger.MsgData{{Text: because + " the file name ends in \".mts\"."}}

	case p.options.moduleTypeData.Type == js_ast.ModuleESM_PackageJSON:
		tracker := logger.MakeLineColumnTracker(p.options.moduleTypeData.Source)
		return whyESMTypeModulePackageJSON, []logger.MsgData{tracker.MsgData(p.options.moduleTypeData.Range,
			because+" the enclosing \"package.json\" file sets the type of this file to \"module\":")}

	// This case must come last because some code cares about the "import"
	// statement keyword and some doesn't, and we don't want to give code
	// that doesn't care about the "import" statement the wrong error message.
	case p.esmImportStatementKeyword.Len > 0:
		return whyESMImportStatement, []logger.MsgData{p.tracker.MsgData(p.esmImportStatementKeyword,
			because+" of the \"import\" keyword here:")}
	}
	return whyESMUnknown, nil
}

func (p *parser) prepareForVisitPass() {
	p.pushScopeForVisitPass(js_ast.ScopeEntry, logger.Loc{Start: locModuleScope})
	p.fnOrArrowDataVisit.isOutsideFnOrArrow = true
	p.moduleScope = p.currentScope

	// Force-enable strict mode if that's the way TypeScript is configured
	if tsAlwaysStrict := p.options.tsAlwaysStrict; tsAlwaysStrict != nil && tsAlwaysStrict.Value {
		p.currentScope.StrictMode = js_ast.ImplicitStrictModeTSAlwaysStrict
	}

	// Determine whether or not this file is ESM
	p.isFileConsideredToHaveESMExports =
		p.esmExportKeyword.Len > 0 ||
			p.esmImportMeta.Len > 0 ||
			p.topLevelAwaitKeyword.Len > 0 ||
			p.options.moduleTypeData.Type.IsESM()
	p.isFileConsideredESM =
		p.isFileConsideredToHaveESMExports ||
			p.esmImportStatementKeyword.Len > 0

	// Legacy HTML comments are not allowed in ESM files
	if p.isFileConsideredESM && p.lexer.LegacyHTMLCommentRange.Len > 0 {
		_, notes := p.whyESModule()
		p.log.AddErrorWithNotes(&p.tracker, p.lexer.LegacyHTMLCommentRange,
			"Legacy HTML single-line comments are not allowed in ECMAScript modules", notes)
	}

	// ECMAScript modules are always interpreted as strict mode. This has to be
	// done before "hoistSymbols" because strict mode can alter hoisting (!).
	if p.isFileConsideredESM {
		p.moduleScope.RecursiveSetStrictMode(js_ast.ImplicitStrictModeESM)
	}

	p.hoistSymbols(p.moduleScope)

	if p.options.mode != config.ModePassThrough {
		p.requireRef = p.declareCommonJSSymbol(ast.SymbolUnbound, "require")
	} else {
		p.requireRef = p.newSymbol(ast.SymbolUnbound, "require")
	}

	// CommonJS-style exports are only enabled if this isn't using ECMAScript-
	// style exports. You can still use "require" in ESM, just not "module" or
	// "exports". You can also still use "import" in CommonJS.
	if p.options.mode != config.ModePassThrough && !p.isFileConsideredToHaveESMExports {
		// CommonJS-style exports
		p.exportsRef = p.declareCommonJSSymbol(ast.SymbolHoisted, "exports")
		p.moduleRef = p.declareCommonJSSymbol(ast.SymbolHoisted, "module")
	} else {
		// ESM-style exports
		p.exportsRef = p.newSymbol(ast.SymbolHoisted, "exports")
		p.moduleRef = p.newSymbol(ast.SymbolHoisted, "module")
	}

	// Handle "@jsx" and "@jsxFrag" pragmas now that lexing is done
	if p.options.jsx.Parse {
		if jsxRuntime := p.lexer.JSXRuntimePragmaComment; jsxRuntime.Text != "" {
			if jsxRuntime.Text == "automatic" {
				p.options.jsx.AutomaticRuntime = true
			} else if jsxRuntime.Text == "classic" {
				p.options.jsx.AutomaticRuntime = false
			} else {
				p.log.AddIDWithNotes(logger.MsgID_JS_UnsupportedJSXComment, logger.Warning, &p.tracker, jsxRuntime.Range,
					fmt.Sprintf("Invalid JSX runtime: %q", jsxRuntime.Text),
					[]logger.MsgData{{Text: "The JSX runtime can only be set to either \"classic\" or \"automatic\"."}})
			}
		}

		if jsxFactory := p.lexer.JSXFactoryPragmaComment; jsxFactory.Text != "" {
			if p.options.jsx.AutomaticRuntime {
				p.log.AddID(logger.MsgID_JS_UnsupportedJSXComment, logger.Warning, &p.tracker, jsxFactory.Range,
					"The JSX factory cannot be set when using React's \"automatic\" JSX transform")
			} else if expr, _ := ParseDefineExpr(jsxFactory.Text); len(expr.Parts) > 0 {
				p.options.jsx.Factory = expr
			} else {
				p.log.AddID(logger.MsgID_JS_UnsupportedJSXComment, logger.Warning, &p.tracker, jsxFactory.Range,
					fmt.Sprintf("Invalid JSX factory: %s", jsxFactory.Text))
			}
		}

		if jsxFragment := p.lexer.JSXFragmentPragmaComment; jsxFragment.Text != "" {
			if p.options.jsx.AutomaticRuntime {
				p.log.AddID(logger.MsgID_JS_UnsupportedJSXComment, logger.Warning, &p.tracker, jsxFragment.Range,
					"The JSX fragment cannot be set when using React's \"automatic\" JSX transform")
			} else if expr, _ := ParseDefineExpr(jsxFragment.Text); len(expr.Parts) > 0 || expr.Constant != nil {
				p.options.jsx.Fragment = expr
			} else {
				p.log.AddID(logger.MsgID_JS_UnsupportedJSXComment, logger.Warning, &p.tracker, jsxFragment.Range,
					fmt.Sprintf("Invalid JSX fragment: %s", jsxFragment.Text))
			}
		}

		if jsxImportSource := p.lexer.JSXImportSourcePragmaComment; jsxImportSource.Text != "" {
			if !p.options.jsx.AutomaticRuntime {
				p.log.AddIDWithNotes(logger.MsgID_JS_UnsupportedJSXComment, logger.Warning, &p.tracker, jsxImportSource.Range,
					"The JSX import source cannot be set without also enabling React's \"automatic\" JSX transform",
					[]logger.MsgData{{Text: "You can enable React's \"automatic\" JSX transform for this file by using a \"@jsxRuntime automatic\" comment."}})
			} else {
				p.options.jsx.ImportSource = jsxImportSource.Text
			}
		}
	}

	// Force-enable strict mode if the JSX "automatic" runtime is enabled and
	// there is at least one JSX element. This is because the automatically-
	// generated import statement turns the file into an ES module. This behavior
	// matches TypeScript which also does this. See this PR for more information:
	// https://github.com/microsoft/TypeScript/pull/39199
	if p.currentScope.StrictMode == js_ast.SloppyMode && p.options.jsx.AutomaticRuntime && p.firstJSXElementLoc.Start != -1 {
		p.currentScope.StrictMode = js_ast.ImplicitStrictModeJSXAutomaticRuntime
	}
}

func (p *parser) declareCommonJSSymbol(kind ast.SymbolKind, name string) ast.Ref {
	member, ok := p.moduleScope.Members[name]

	// If the code declared this symbol using "var name", then this is actually
	// not a collision. For example, node will let you do this:
	//
	//   var exports;
	//   module.exports.foo = 123;
	//   console.log(exports.foo);
	//
	// This works because node's implementation of CommonJS wraps the entire
	// source file like this:
	//
	//   (function(require, exports, module, __filename, __dirname) {
	//     var exports;
	//     module.exports.foo = 123;
	//     console.log(exports.foo);
	//   })
	//
	// Both the "exports" argument and "var exports" are hoisted variables, so
	// they don't collide.
	if ok && p.symbols[member.Ref.InnerIndex].Kind == ast.SymbolHoisted &&
		kind == ast.SymbolHoisted && !p.isFileConsideredToHaveESMExports {
		return member.Ref
	}

	// Create a new symbol if we didn't merge with an existing one above
	ref := p.newSymbol(kind, name)

	// If the variable wasn't declared, declare it now. This means any references
	// to this name will become bound to this symbol after this (since we haven't
	// run the visit pass yet).
	if !ok {
		p.moduleScope.Members[name] = js_ast.ScopeMember{Ref: ref, Loc: logger.Loc{Start: -1}}
		return ref
	}

	// If the variable was declared, then it shadows this symbol. The code in
	// this module will be unable to reference this symbol. However, we must
	// still add the symbol to the scope so it gets minified (automatically-
	// generated code may still reference the symbol).
	p.moduleScope.Generated = append(p.moduleScope.Generated, ref)
	return ref
}

// Compute a character frequency histogram for everything that's not a bound
// symbol. This is used to modify how minified names are generated for slightly
// better gzip compression. Even though it's a very small win, we still do it
// because it's simple to do and very cheap to compute.
func (p *parser) computeCharacterFrequency() *ast.CharFreq {
	if !p.options.minifyIdentifiers || p.source.Index == runtime.SourceIndex {
		return nil
	}

	// Add everything in the file to the histogram
	charFreq := &ast.CharFreq{}
	charFreq.Scan(p.source.Contents, 1)

	// Subtract out all comments
	for _, commentRange := range p.lexer.AllComments {
		charFreq.Scan(p.source.TextForRange(commentRange), -1)
	}

	// Subtract out all import paths
	for _, record := range p.importRecords {
		if !record.SourceIndex.IsValid() {
			charFreq.Scan(record.Path.Text, -1)
		}
	}

	// Subtract out all symbols that will be minified
	var visit func(*js_ast.Scope)
	visit = func(scope *js_ast.Scope) {
		for _, member := range scope.Members {
			symbol := &p.symbols[member.Ref.InnerIndex]
			if symbol.SlotNamespace() != ast.SlotMustNotBeRenamed {
				charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate))
			}
		}
		if scope.Label.Ref != ast.InvalidRef {
			symbol := &p.symbols[scope.Label.Ref.InnerIndex]
			if symbol.SlotNamespace() != ast.SlotMustNotBeRenamed {
				charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate)-1)
			}
		}
		for _, child := range scope.Children {
			visit(child)
		}
	}
	visit(p.moduleScope)

	// Subtract out all properties that will be mangled
	for _, ref := range p.mangledProps {
		symbol := &p.symbols[ref.InnerIndex]
		charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate))
	}

	return charFreq
}

func (p *parser) generateImportStmt(
	path string,
	pathRange logger.Range,
	imports []string,
	parts []js_ast.Part,
	symbols map[string]ast.LocRef,
	sourceIndex *uint32,
	copySourceIndex *uint32,
) ([]js_ast.Part, uint32) {
	if pathRange.Len == 0 {
		isFirst := true
		for _, it := range symbols {
			if isFirst || it.Loc.Start < pathRange.Loc.Start {
				pathRange.Loc = it.Loc
			}
			isFirst = false
		}
	}

	namespaceRef := p.newSymbol(ast.SymbolOther, "import_"+js_ast.GenerateNonUniqueNameFromPath(path))
	p.moduleScope.Generated = append(p.moduleScope.Generated, namespaceRef)
	declaredSymbols := make([]js_ast.DeclaredSymbol, 1+len(imports))
	clauseItems := make([]js_ast.ClauseItem, len(imports))
	importRecordIndex := p.addImportRecord(ast.ImportStmt, pathRange, path, nil, 0)
	if sourceIndex != nil {
		p.importRecords[importRecordIndex].SourceIndex = ast.MakeIndex32(*sourceIndex)
	}
	if copySourceIndex != nil {
		p.importRecords[importRecordIndex].CopySourceIndex = ast.MakeIndex32(*copySourceIndex)
	}
	declaredSymbols[0] = js_ast.DeclaredSymbol{Ref: namespaceRef, IsTopLevel: true}

	// Create per-import information
	for i, alias := range imports {
		it := symbols[alias]
		declaredSymbols[i+1] = js_ast.DeclaredSymbol{Ref: it.Ref, IsTopLevel: true}
		clauseItems[i] = js_ast.ClauseItem{
			Alias:    alias,
			AliasLoc: it.Loc,
			Name:     ast.LocRef{Loc: it.Loc, Ref: it.Ref},
		}
		p.isImportItem[it.Ref] = true
		p.namedImports[it.Ref] = js_ast.NamedImport{
			Alias:             alias,
			AliasLoc:          it.Loc,
			NamespaceRef:      namespaceRef,
			ImportRecordIndex: importRecordIndex,
		}
	}

	// Append a single import to the end of the file (ES6 imports are hoisted
	// so we don't need to worry about where the import statement goes)
	return append(parts, js_ast.Part{
		DeclaredSymbols:     declaredSymbols,
		ImportRecordIndices: []uint32{importRecordIndex},
		Stmts: []js_ast.Stmt{{Loc: pathRange.Loc, Data: &js_ast.SImport{
			NamespaceRef:      namespaceRef,
			Items:             &clauseItems,
			ImportRecordIndex: importRecordIndex,
			IsSingleLine:      true,
		}}},
	}), importRecordIndex
}

// Sort the keys for determinism
func sortedKeysOfMapStringLocRef(in map[string]ast.LocRef) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (p *parser) toAST(before, parts, after []js_ast.Part, hashbang string, directives []string) js_ast.AST {
	// Insert an import statement for any runtime imports we generated
	if len(p.runtimeImports) > 0 && !p.options.omitRuntimeForTests {
		keys := sortedKeysOfMapStringLocRef(p.runtimeImports)
		sourceIndex := runtime.SourceIndex
		before, _ = p.generateImportStmt("<runtime>", logger.Range{}, keys, before, p.runtimeImports, &sourceIndex, nil)
	}

	// Insert an import statement for any jsx runtime imports we generated
	if len(p.jsxRuntimeImports) > 0 && !p.options.omitJSXRuntimeForTests {
		keys := sortedKeysOfMapStringLocRef(p.jsxRuntimeImports)

		// Determine the runtime source and whether it's prod or dev
		path := p.options.jsx.ImportSource
		if p.options.jsx.Development {
			path = path + "/jsx-dev-runtime"
		} else {
			path = path + "/jsx-runtime"
		}

		before, _ = p.generateImportStmt(path, logger.Range{}, keys, before, p.jsxRuntimeImports, nil, nil)
	}

	// Insert an import statement for any legacy jsx imports we generated (i.e., createElement)
	if len(p.jsxLegacyImports) > 0 && !p.options.omitJSXRuntimeForTests {
		keys := sortedKeysOfMapStringLocRef(p.jsxLegacyImports)
		path := p.options.jsx.ImportSource
		before, _ = p.generateImportStmt(path, logger.Range{}, keys, before, p.jsxLegacyImports, nil, nil)
	}

	// Insert imports for each glob pattern
	for _, glob := range p.globPatternImports {
		symbols := map[string]ast.LocRef{glob.name: {Loc: glob.approximateRange.Loc, Ref: glob.ref}}
		var importRecordIndex uint32
		before, importRecordIndex = p.generateImportStmt(helpers.GlobPatternToString(glob.parts), glob.approximateRange, []string{glob.name}, before, symbols, nil, nil)
		record := &p.importRecords[importRecordIndex]
		record.AssertOrWith = glob.assertOrWith
		record.GlobPattern = &ast.GlobPattern{
			Parts:       glob.parts,
			ExportAlias: glob.name,
			Kind:        glob.kind,
		}
	}

	// Generated imports are inserted before other code instead of appending them
	// to the end of the file. Appending them should work fine because JavaScript
	// import statements are "hoisted" to run before the importing file. However,
	// some buggy JavaScript toolchains such as the TypeScript compiler convert
	// ESM into CommonJS by replacing "import" statements inline without doing
	// any hoisting, which is incorrect. See the following issue for more info:
	// https://github.com/microsoft/TypeScript/issues/16166. Since JSX-related
	// imports are present in the generated code when bundling is disabled, and
	// could therefore be processed by these buggy tools, it's more robust to put
	// them at the top even though it means potentially reallocating almost the
	// entire array of parts.
	if len(before) > 0 {
		parts = append(before, parts...)
	}
	parts = append(parts, after...)

	// Handle import paths after the whole file has been visited because we need
	// symbol usage counts to be able to remove unused type-only imports in
	// TypeScript code.
	keptImportEquals := false
	removedImportEquals := false
	partsEnd := 0
	for partIndex, part := range parts {
		p.importRecordsForCurrentPart = nil
		p.declaredSymbols = nil

		result := p.scanForImportsAndExports(part.Stmts)
		part.Stmts = result.stmts
		keptImportEquals = keptImportEquals || result.keptImportEquals
		removedImportEquals = removedImportEquals || result.removedImportEquals

		part.ImportRecordIndices = append(part.ImportRecordIndices, p.importRecordsForCurrentPart...)
		part.DeclaredSymbols = append(part.DeclaredSymbols, p.declaredSymbols...)

		if len(part.Stmts) > 0 || uint32(partIndex) == js_ast.NSExportPartIndex {
			if p.moduleScope.ContainsDirectEval && len(part.DeclaredSymbols) > 0 {
				// If this file contains a direct call to "eval()", all parts that
				// declare top-level symbols must be kept since the eval'd code may
				// reference those symbols.
				part.CanBeRemovedIfUnused = false
			}
			parts[partsEnd] = part
			partsEnd++
		}
	}
	parts = parts[:partsEnd]

	// We need to iterate multiple times if an import-equals statement was
	// removed and there are more import-equals statements that may be removed.
	// In the example below, a/b/c should be kept but x/y/z should be removed
	// (and removal requires multiple passes):
	//
	//   import a = foo.a
	//   import b = a.b
	//   import c = b.c
	//
	//   import x = foo.x
	//   import y = x.y
	//   import z = y.z
	//
	//   export let bar = c
	//
	// This is a smaller version of the general import/export scanning loop above.
	// We only want to repeat the code that eliminates TypeScript import-equals
	// statements, not the other code in the loop above.
	for keptImportEquals && removedImportEquals {
		keptImportEquals = false
		removedImportEquals = false
		partsEnd := 0
		for partIndex, part := range parts {
			result := p.scanForUnusedTSImportEquals(part.Stmts)
			part.Stmts = result.stmts
			keptImportEquals = keptImportEquals || result.keptImportEquals
			removedImportEquals = removedImportEquals || result.removedImportEquals
			if len(part.Stmts) > 0 || uint32(partIndex) == js_ast.NSExportPartIndex {
				parts[partsEnd] = part
				partsEnd++
			}
		}
		parts = parts[:partsEnd]
	}

	// Do a second pass for exported items now that imported items are filled out
	for _, part := range parts {
		for _, stmt := range part.Stmts {
			if s, ok := stmt.Data.(*js_ast.SExportClause); ok {
				for _, item := range s.Items {
					// Mark re-exported imports as such
					if namedImport, ok := p.namedImports[item.Name.Ref]; ok {
						namedImport.IsExported = true
						p.namedImports[item.Name.Ref] = namedImport
					}
				}
			}
		}
	}

	// Analyze cross-part dependencies for tree shaking and code splitting
	{
		// Map locals to parts
		p.topLevelSymbolToParts = make(map[ast.Ref][]uint32)
		for partIndex, part := range parts {
			for _, declared := range part.DeclaredSymbols {
				if declared.IsTopLevel {
					// If this symbol was merged, use the symbol at the end of the
					// linked list in the map. This is the case for multiple "var"
					// declarations with the same name, for example.
					ref := declared.Ref
					for p.symbols[ref.InnerIndex].Link != ast.InvalidRef {
						ref = p.symbols[ref.InnerIndex].Link
					}
					p.topLevelSymbolToParts[ref] = append(
						p.topLevelSymbolToParts[ref], uint32(partIndex))
				}
			}
		}

		// Pulling in the exports of this module always pulls in the export part
		p.topLevelSymbolToParts[p.exportsRef] = append(p.topLevelSymbolToParts[p.exportsRef], js_ast.NSExportPartIndex)
	}

	// Make a wrapper symbol in case we need to be wrapped in a closure
	wrapperRef := p.newSymbol(ast.SymbolOther, "require_"+p.source.IdentifierName)

	// Assign slots to symbols in nested scopes. This is some precomputation for
	// the symbol renaming pass that will happen later in the linker. It's done
	// now in the parser because we want it to be done in parallel per file and
	// we're already executing code in a dedicated goroutine for this file.
	var nestedScopeSlotCounts ast.SlotCounts
	if p.options.minifyIdentifiers {
		nestedScopeSlotCounts = renamer.AssignNestedScopeSlots(p.moduleScope, p.symbols)
	}

	exportsKind := js_ast.ExportsNone
	usesExportsRef := p.symbols[p.exportsRef.InnerIndex].UseCountEstimate > 0
	usesModuleRef := p.symbols[p.moduleRef.InnerIndex].UseCountEstimate > 0

	if p.esmExportKeyword.Len > 0 || p.esmImportMeta.Len > 0 || p.topLevelAwaitKeyword.Len > 0 {
		exportsKind = js_ast.ExportsESM
	} else if usesExportsRef || usesModuleRef || p.hasTopLevelReturn {
		exportsKind = js_ast.ExportsCommonJS
	} else {
		// If this module has no exports, try to determine what kind of module it
		// is by looking at node's "type" field in "package.json" and/or whether
		// the file extension is ".mjs"/".mts" or ".cjs"/".cts".
		switch {
		case p.options.moduleTypeData.Type.IsCommonJS():
			// ".cjs" or ".cts" or ("type: commonjs" and (".js" or ".jsx" or ".ts" or ".tsx"))
			exportsKind = js_ast.ExportsCommonJS

		case p.options.moduleTypeData.Type.IsESM():
			// ".mjs" or ".mts" or ("type: module" and (".js" or ".jsx" or ".ts" or ".tsx"))
			exportsKind = js_ast.ExportsESM

		default:
			// Treat unknown modules containing an import statement as ESM. Otherwise
			// the bundler will treat this file as CommonJS if it's imported and ESM
			// if it's not imported.
			if p.esmImportStatementKeyword.Len > 0 {
				exportsKind = js_ast.ExportsESM
			}
		}
	}

	return js_ast.AST{
		Parts:                           parts,
		ModuleTypeData:                  p.options.moduleTypeData,
		ModuleScope:                     p.moduleScope,
		CharFreq:                        p.computeCharacterFrequency(),
		Symbols:                         p.symbols,
		ExportsRef:                      p.exportsRef,
		ModuleRef:                       p.moduleRef,
		WrapperRef:                      wrapperRef,
		Hashbang:                        hashbang,
		Directives:                      directives,
		NamedImports:                    p.namedImports,
		NamedExports:                    p.namedExports,
		TSEnums:                         p.tsEnums,
		ConstValues:                     p.constValues,
		ExprComments:                    p.exprComments,
		NestedScopeSlotCounts:           nestedScopeSlotCounts,
		TopLevelSymbolToPartsFromParser: p.topLevelSymbolToParts,
		ExportStarImportRecords:         p.exportStarImportRecords,
		ImportRecords:                   p.importRecords,
		ApproximateLineCount:            int32(p.lexer.ApproximateNewlineCount) + 1,
		MangledProps:                    p.mangledProps,
		ReservedProps:                   p.reservedProps,
		ManifestForYarnPnP:              p.manifestForYarnPnP,

		// CommonJS features
		UsesExportsRef: usesExportsRef,
		UsesModuleRef:  usesModuleRef,
		ExportsKind:    exportsKind,

		// ES6 features
		ExportKeyword:            p.esmExportKeyword,
		TopLevelAwaitKeyword:     p.topLevelAwaitKeyword,
		LiveTopLevelAwaitKeyword: p.liveTopLevelAwaitKeyword,
	}
}
