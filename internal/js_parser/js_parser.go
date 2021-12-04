package js_parser

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
	"unsafe"

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
// 2. Visit each node in the AST, bind identifiers to declared symbols, do
//    constant folding, substitute compile-time variable definitions, and
//    lower certain syntactic constructs as appropriate given the language
//    target.
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
	lexer                      js_lexer.Lexer
	allowIn                    bool
	allowPrivateIdentifiers    bool
	hasTopLevelReturn          bool
	latestReturnHadSemicolon   bool
	hasImportMeta              bool
	hasESModuleSyntax          bool
	warnedThisIsUndefined      bool
	topLevelAwaitKeyword       logger.Range
	fnOrArrowDataParse         fnOrArrowDataParse
	fnOrArrowDataVisit         fnOrArrowDataVisit
	fnOnlyDataVisit            fnOnlyDataVisit
	allocatedNames             []string
	latestArrowArgLoc          logger.Loc
	forbidSuffixAfterAsLoc     logger.Loc
	currentScope               *js_ast.Scope
	scopesForCurrentPart       []*js_ast.Scope
	symbols                    []js_ast.Symbol
	tsUseCounts                []uint32
	exportsRef                 js_ast.Ref
	requireRef                 js_ast.Ref
	moduleRef                  js_ast.Ref
	importMetaRef              js_ast.Ref
	promiseRef                 js_ast.Ref
	findSymbolHelper           func(loc logger.Loc, name string) js_ast.Ref
	symbolForDefineHelper      func(int) js_ast.Ref
	injectedDefineSymbols      []js_ast.Ref
	injectedSymbolSources      map[js_ast.Ref]injectedSymbolSource
	symbolUses                 map[js_ast.Ref]js_ast.SymbolUse
	declaredSymbols            []js_ast.DeclaredSymbol
	runtimeImports             map[string]js_ast.Ref
	duplicateCaseChecker       duplicateCaseChecker
	unrepresentableIdentifiers map[string]bool
	legacyOctalLiterals        map[js_ast.E]logger.Range

	// For strict mode handling
	hoistedRefForSloppyModeBlockFn map[js_ast.Ref]js_ast.Ref

	// For lowering private methods
	weakMapRef     js_ast.Ref
	weakSetRef     js_ast.Ref
	privateGetters map[js_ast.Ref]js_ast.Ref
	privateSetters map[js_ast.Ref]js_ast.Ref

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
	//
	// The "shouldFoldNumericConstants" flag is enabled inside each enum body block
	// since TypeScript requires numeric constant folding in enum definitions.
	refToTSNamespaceMemberData map[js_ast.Ref]js_ast.TSNamespaceMemberData
	tsNamespaceTarget          js_ast.E
	tsNamespaceMemberData      js_ast.TSNamespaceMemberData
	shouldFoldNumericConstants bool
	emittedNamespaceVars       map[js_ast.Ref]bool
	isExportedInsideNamespace  map[js_ast.Ref]js_ast.Ref
	localTypeNames             map[string]bool

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
	enclosingNamespaceArgRef *js_ast.Ref

	// Imports (both ES6 and CommonJS) are tracked at the top level
	importRecords               []ast.ImportRecord
	importRecordsForCurrentPart []uint32
	exportStarImportRecords     []uint32

	// These are for handling ES6 imports and exports
	es6ImportKeyword        logger.Range
	es6ExportKeyword        logger.Range
	enclosingClassKeyword   logger.Range
	importItemsForNamespace map[js_ast.Ref]map[string]js_ast.LocRef
	isImportItem            map[js_ast.Ref]bool
	namedImports            map[js_ast.Ref]js_ast.NamedImport
	namedExports            map[string]js_ast.NamedExport
	topLevelSymbolToParts   map[js_ast.Ref][]uint32
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

	// These properties are for the visit pass, which runs after the parse pass.
	// The visit pass binds identifiers to declared symbols, does constant
	// folding, substitutes compile-time variable definitions, and lowers certain
	// syntactic constructs as appropriate.
	stmtExprValue     js_ast.E
	callTarget        js_ast.E
	templateTag       js_ast.E
	deleteTarget      js_ast.E
	loopBody          js_ast.S
	moduleScope       *js_ast.Scope
	isControlFlowDead bool

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

	// This helps recognize the "await import()" pattern. When this is present,
	// warnings about non-string import paths will be omitted inside try blocks.
	awaitTarget js_ast.E

	// This helps recognize the "import().catch()" pattern. We also try to avoid
	// warning about this just like the "try { await import() }" pattern.
	thenCatchChain thenCatchChain

	// Temporary variables used for lowering
	tempRefsToDeclare         []tempRef
	tempRefCount              int
	topLevelTempRefsToDeclare []tempRef
	topLevelTempRefCount      int

	// When bundling, hoisted top-level local variables declared with "var" in
	// nested scopes are moved up to be declared in the top-level scope instead.
	// The old "var" statements are turned into regular assignments instead. This
	// makes it easier to quickly scan the top-level statements for "var" locals
	// with the guarantee that all will be found.
	relocatedTopLevelVars []js_ast.LocRef

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
	classPrivateBrandChecksToLower map[string]bool

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
}

type injectedSymbolSource struct {
	source logger.Source
	loc    logger.Loc
}

type importNamespaceCallKind uint8

const (
	exprKindCall importNamespaceCallKind = iota
	exprKindNew
	exprKindJSXTag
)

type importNamespaceCall struct {
	ref  js_ast.Ref
	kind importNamespaceCallKind
}

type thenCatchChain struct {
	nextTarget      js_ast.E
	hasMultipleArgs bool
	hasCatch        bool
}

// This is used as part of an incremental build cache key. Some of these values
// can potentially change between builds if they are derived from nearby
// "package.json" or "tsconfig.json" files that were changed since the last
// build.
type Options struct {
	injectedFiles []config.InjectedFile
	jsx           config.JSXOptions
	tsTarget      *config.TSTarget

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
	unsupportedJSFeatures compat.JSFeature
	originalTargetEnv     string

	// Byte-sized values go here (gathered together here to keep this object compact)
	ts                      config.TSOptions
	mode                    config.Mode
	platform                config.Platform
	outputFormat            config.Format
	moduleType              config.ModuleType
	targetFromAPI           config.TargetFromAPI
	asciiOnly               bool
	keepNames               bool
	mangleSyntax            bool
	minifyIdentifiers       bool
	omitRuntimeForTests     bool
	ignoreDCEAnnotations    bool
	treeShaking             bool
	unusedImportsTS         config.UnusedImportsTS
	useDefineForClassFields config.MaybeBool
}

func OptionsFromConfig(options *config.Options) Options {
	return Options{
		injectedFiles: options.InjectedFiles,
		jsx:           options.JSX,
		defines:       options.Defines,
		tsTarget:      options.TSTarget,
		optionsThatSupportStructuralEquality: optionsThatSupportStructuralEquality{
			unsupportedJSFeatures:   options.UnsupportedJSFeatures,
			originalTargetEnv:       options.OriginalTargetEnv,
			ts:                      options.TS,
			mode:                    options.Mode,
			platform:                options.Platform,
			outputFormat:            options.OutputFormat,
			moduleType:              options.ModuleType,
			targetFromAPI:           options.TargetFromAPI,
			asciiOnly:               options.ASCIIOnly,
			keepNames:               options.KeepNames,
			mangleSyntax:            options.MangleSyntax,
			minifyIdentifiers:       options.MinifyIdentifiers,
			omitRuntimeForTests:     options.OmitRuntimeForTests,
			ignoreDCEAnnotations:    options.IgnoreDCEAnnotations,
			treeShaking:             options.TreeShaking,
			unusedImportsTS:         options.UnusedImportsTS,
			useDefineForClassFields: options.UseDefineForClassFields,
		},
	}
}

func (a *Options) Equal(b *Options) bool {
	// Compare "optionsThatSupportStructuralEquality"
	if a.optionsThatSupportStructuralEquality != b.optionsThatSupportStructuralEquality {
		return false
	}

	// Compare "TSTarget"
	if (a.tsTarget == nil && b.tsTarget != nil) || (a.tsTarget != nil && b.tsTarget == nil) ||
		(a.tsTarget != nil && b.tsTarget != nil && *a.tsTarget != *b.tsTarget) {
		return false
	}

	// Compare "InjectedFiles"
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

	// Compare "JSX"
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

func jsxExprsEqual(a config.JSXExpr, b config.JSXExpr) bool {
	if !stringArraysEqual(a.Parts, b.Parts) {
		return false
	}

	if a.Constant != nil {
		if b.Constant == nil || !valuesLookTheSame(a.Constant, b.Constant) {
			return false
		}
	} else if b.Constant != nil {
		return false
	}

	return true
}

func stringArraysEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x != b[i] {
			return false
		}
	}
	return true
}

type tempRef struct {
	ref        js_ast.Ref
	valueOrNil js_ast.Expr
}

const (
	locModuleScope = -1
)

type scopeOrder struct {
	loc   logger.Loc
	scope *js_ast.Scope
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
	needsAsyncLoc       logger.Loc
	asyncRange          logger.Range
	arrowArgErrors      *deferredArrowArgErrors
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

	// Allow TypeScript decorators in function arguments
	allowTSDecorators bool
}

// This is function-specific information used during visiting. It is saved and
// restored on the call stack around code that parses nested functions and
// arrow expressions.
type fnOrArrowDataVisit struct {
	isArrow            bool
	isAsync            bool
	isGenerator        bool
	isInsideLoop       bool
	isInsideSwitch     bool
	isOutsideFnOrArrow bool
	shouldLowerSuper   bool

	// This is used to silence unresolvable imports due to "require" calls inside
	// a try/catch statement. The assumption is that the try/catch statement is
	// there to handle the case where the reference to "require" crashes.
	tryBodyCount int
}

type superHelpers struct {
	getRef js_ast.Ref
	setRef js_ast.Ref
}

// This is function-specific information used during visiting. It is saved and
// restored on the call stack around code that parses nested functions (but not
// nested arrow functions).
type fnOnlyDataVisit struct {
	// These helpers are necessary to forward access to "super" into lowered
	// async functions. Lowering transforms async functions into generator
	// functions, which no longer have direct access to "super".
	superHelpers *superHelpers

	// This is a reference to the magic "arguments" variable that exists inside
	// functions in JavaScript. It will be non-nil inside functions and nil
	// otherwise.
	argumentsRef *js_ast.Ref

	// Arrow functions don't capture the value of "this" and "arguments". Instead,
	// the values are inherited from the surrounding context. If arrow functions
	// are turned into regular functions due to lowering, we will need to generate
	// local variables to capture these values so they are preserved correctly.
	thisCaptureRef      *js_ast.Ref
	argumentsCaptureRef *js_ast.Ref

	// Inside a static class property initializer, "this" expressions should be
	// replaced with the class name.
	thisClassStaticRef *js_ast.Ref

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
}

const bloomFilterSize = 251

type duplicateCaseValue struct {
	hash  uint32
	value js_ast.Expr
}

type duplicateCaseChecker struct {
	bloomFilter [(bloomFilterSize + 7) / 8]byte
	cases       []duplicateCaseValue
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
						r := p.source.RangeOfOperatorBefore(expr.Loc, "case")
						earlierRange := p.source.RangeOfOperatorBefore(c.value.Loc, "case")
						text := "This case clause will never be evaluated because it duplicates an earlier case clause"
						if couldBeIncorrect {
							text = "This case clause may never be evaluated because it likely duplicates an earlier case clause"
						}
						kind := logger.Warning
						if p.suppressWarningsAboutWeirdCode {
							kind = logger.Debug
						}
						p.log.AddWithNotes(kind, &p.tracker, r, text,
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
		return ok && js_lexer.UTF16EqualsUTF16(a.Value, b.Value), false

	case *js_ast.EBigInt:
		b, ok := right.Data.(*js_ast.EBigInt)
		return ok && a.Value == b.Value, false

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

func isJumpStatement(data js_ast.S) bool {
	switch data.(type) {
	case *js_ast.SBreak, *js_ast.SContinue, *js_ast.SReturn, *js_ast.SThrow:
		return true
	}

	return false
}

func isPrimitiveToReorder(data js_ast.E) bool {
	switch e := data.(type) {
	case *js_ast.EInlinedEnum:
		return isPrimitiveToReorder(e.Value.Data)

	case *js_ast.ENull, *js_ast.EUndefined, *js_ast.EString, *js_ast.EBoolean, *js_ast.ENumber, *js_ast.EBigInt:
		return true
	}
	return false
}

type sideEffects uint8

const (
	couldHaveSideEffects sideEffects = iota
	noSideEffects
)

func toNullOrUndefinedWithSideEffects(data js_ast.E) (isNullOrUndefined bool, sideEffects sideEffects, ok bool) {
	switch e := data.(type) {
	case *js_ast.EInlinedEnum:
		return toNullOrUndefinedWithSideEffects(e.Value.Data)

		// Never null or undefined
	case *js_ast.EBoolean, *js_ast.ENumber, *js_ast.EString, *js_ast.ERegExp,
		*js_ast.EFunction, *js_ast.EArrow, *js_ast.EBigInt:
		return false, noSideEffects, true

	// Never null or undefined
	case *js_ast.EObject, *js_ast.EArray, *js_ast.EClass:
		return false, couldHaveSideEffects, true

	// Always null or undefined
	case *js_ast.ENull, *js_ast.EUndefined:
		return true, noSideEffects, true

	case *js_ast.EUnary:
		switch e.Op {
		case
			// Always number or bigint
			js_ast.UnOpPos, js_ast.UnOpNeg, js_ast.UnOpCpl,
			js_ast.UnOpPreDec, js_ast.UnOpPreInc, js_ast.UnOpPostDec, js_ast.UnOpPostInc,
			// Always boolean
			js_ast.UnOpNot, js_ast.UnOpTypeof, js_ast.UnOpDelete:
			return false, couldHaveSideEffects, true

		// Always undefined
		case js_ast.UnOpVoid:
			return true, couldHaveSideEffects, true
		}

	case *js_ast.EBinary:
		switch e.Op {
		case
			// Always string or number or bigint
			js_ast.BinOpAdd, js_ast.BinOpAddAssign,
			// Always number or bigint
			js_ast.BinOpSub, js_ast.BinOpMul, js_ast.BinOpDiv, js_ast.BinOpRem, js_ast.BinOpPow,
			js_ast.BinOpSubAssign, js_ast.BinOpMulAssign, js_ast.BinOpDivAssign, js_ast.BinOpRemAssign, js_ast.BinOpPowAssign,
			js_ast.BinOpShl, js_ast.BinOpShr, js_ast.BinOpUShr,
			js_ast.BinOpShlAssign, js_ast.BinOpShrAssign, js_ast.BinOpUShrAssign,
			js_ast.BinOpBitwiseOr, js_ast.BinOpBitwiseAnd, js_ast.BinOpBitwiseXor,
			js_ast.BinOpBitwiseOrAssign, js_ast.BinOpBitwiseAndAssign, js_ast.BinOpBitwiseXorAssign,
			// Always boolean
			js_ast.BinOpLt, js_ast.BinOpLe, js_ast.BinOpGt, js_ast.BinOpGe, js_ast.BinOpIn, js_ast.BinOpInstanceof,
			js_ast.BinOpLooseEq, js_ast.BinOpLooseNe, js_ast.BinOpStrictEq, js_ast.BinOpStrictNe:
			return false, couldHaveSideEffects, true

		case js_ast.BinOpComma:
			if isNullOrUndefined, _, ok := toNullOrUndefinedWithSideEffects(e.Right.Data); ok {
				return isNullOrUndefined, couldHaveSideEffects, true
			}
		}
	}

	return false, noSideEffects, false
}

func toBooleanWithSideEffects(data js_ast.E) (boolean bool, sideEffects sideEffects, ok bool) {
	switch e := data.(type) {
	case *js_ast.EInlinedEnum:
		return toBooleanWithSideEffects(e.Value.Data)

	case *js_ast.ENull, *js_ast.EUndefined:
		return false, noSideEffects, true

	case *js_ast.EBoolean:
		return e.Value, noSideEffects, true

	case *js_ast.ENumber:
		return e.Value != 0 && !math.IsNaN(e.Value), noSideEffects, true

	case *js_ast.EBigInt:
		return e.Value != "0", noSideEffects, true

	case *js_ast.EString:
		return len(e.Value) > 0, noSideEffects, true

	case *js_ast.EFunction, *js_ast.EArrow, *js_ast.ERegExp:
		return true, noSideEffects, true

	case *js_ast.EObject, *js_ast.EArray, *js_ast.EClass:
		return true, couldHaveSideEffects, true

	case *js_ast.EUnary:
		switch e.Op {
		case js_ast.UnOpVoid:
			return false, couldHaveSideEffects, true

		case js_ast.UnOpTypeof:
			// Never an empty string
			return true, couldHaveSideEffects, true

		case js_ast.UnOpNot:
			if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Value.Data); ok {
				return !boolean, sideEffects, true
			}
		}

	case *js_ast.EBinary:
		switch e.Op {
		case js_ast.BinOpLogicalOr:
			// "anything || truthy" is truthy
			if boolean, _, ok := toBooleanWithSideEffects(e.Right.Data); ok && boolean {
				return true, couldHaveSideEffects, true
			}

		case js_ast.BinOpLogicalAnd:
			// "anything && falsy" is falsy
			if boolean, _, ok := toBooleanWithSideEffects(e.Right.Data); ok && !boolean {
				return false, couldHaveSideEffects, true
			}

		case js_ast.BinOpComma:
			// "anything, truthy/falsy" is truthy/falsy
			if boolean, _, ok := toBooleanWithSideEffects(e.Right.Data); ok {
				return boolean, couldHaveSideEffects, true
			}
		}
	}

	return false, couldHaveSideEffects, false
}

func toNumberWithoutSideEffects(data js_ast.E) (float64, bool) {
	switch e := data.(type) {
	case *js_ast.EInlinedEnum:
		return toNumberWithoutSideEffects(e.Value.Data)

	case *js_ast.ENull:
		return 0, true

	case *js_ast.EUndefined:
		return math.NaN(), true

	case *js_ast.EBoolean:
		if e.Value {
			return 1, true
		} else {
			return 0, true
		}

	case *js_ast.ENumber:
		return e.Value, true
	}

	return 0, false
}

// Returns true if the result of the "typeof" operator on this expression is
// statically determined and this expression has no side effects (i.e. can be
// removed without consequence).
func typeofWithoutSideEffects(data js_ast.E) (string, bool) {
	switch e := data.(type) {
	case *js_ast.EInlinedEnum:
		return typeofWithoutSideEffects(e.Value.Data)

	case *js_ast.ENull:
		return "object", true

	case *js_ast.EUndefined:
		return "undefined", true

	case *js_ast.EBoolean:
		return "boolean", true

	case *js_ast.ENumber:
		return "number", true

	case *js_ast.EBigInt:
		return "bigint", true

	case *js_ast.EString:
		return "string", true

	case *js_ast.EFunction, *js_ast.EArrow:
		return "function", true
	}

	return "", false
}

// Returns true if this expression is known to result in a primitive value (i.e.
// null, undefined, boolean, number, bigint, or string), even if the expression
// cannot be removed due to side effects.
func isPrimitiveWithSideEffects(data js_ast.E) bool {
	switch e := data.(type) {
	case *js_ast.EInlinedEnum:
		return isPrimitiveWithSideEffects(e.Value.Data)

	case *js_ast.ENull, *js_ast.EUndefined, *js_ast.EBoolean, *js_ast.ENumber, *js_ast.EBigInt, *js_ast.EString:
		return true

	case *js_ast.EUnary:
		switch e.Op {
		case
			// Number or bigint
			js_ast.UnOpPos, js_ast.UnOpNeg, js_ast.UnOpCpl,
			js_ast.UnOpPreDec, js_ast.UnOpPreInc, js_ast.UnOpPostDec, js_ast.UnOpPostInc,
			// Boolean
			js_ast.UnOpNot, js_ast.UnOpDelete,
			// Undefined
			js_ast.UnOpVoid,
			// String
			js_ast.UnOpTypeof:
			return true
		}

	case *js_ast.EBinary:
		switch e.Op {
		case
			// Boolean
			js_ast.BinOpLt, js_ast.BinOpLe, js_ast.BinOpGt, js_ast.BinOpGe, js_ast.BinOpIn,
			js_ast.BinOpInstanceof, js_ast.BinOpLooseEq, js_ast.BinOpLooseNe, js_ast.BinOpStrictEq, js_ast.BinOpStrictNe,
			// String, number, or bigint
			js_ast.BinOpAdd, js_ast.BinOpAddAssign,
			// Number or bigint
			js_ast.BinOpSub, js_ast.BinOpMul, js_ast.BinOpDiv, js_ast.BinOpRem, js_ast.BinOpPow,
			js_ast.BinOpSubAssign, js_ast.BinOpMulAssign, js_ast.BinOpDivAssign, js_ast.BinOpRemAssign, js_ast.BinOpPowAssign,
			js_ast.BinOpShl, js_ast.BinOpShr, js_ast.BinOpUShr,
			js_ast.BinOpShlAssign, js_ast.BinOpShrAssign, js_ast.BinOpUShrAssign,
			js_ast.BinOpBitwiseOr, js_ast.BinOpBitwiseAnd, js_ast.BinOpBitwiseXor,
			js_ast.BinOpBitwiseOrAssign, js_ast.BinOpBitwiseAndAssign, js_ast.BinOpBitwiseXorAssign:
			return true

		// These always return one of the arguments unmodified
		case js_ast.BinOpLogicalAnd, js_ast.BinOpLogicalOr, js_ast.BinOpNullishCoalescing,
			js_ast.BinOpLogicalAndAssign, js_ast.BinOpLogicalOrAssign, js_ast.BinOpNullishCoalescingAssign:
			return isPrimitiveWithSideEffects(e.Left.Data) && isPrimitiveWithSideEffects(e.Right.Data)

		case js_ast.BinOpComma:
			return isPrimitiveWithSideEffects(e.Right.Data)
		}

	case *js_ast.EIf:
		return isPrimitiveWithSideEffects(e.Yes.Data) && isPrimitiveWithSideEffects(e.No.Data)
	}

	return false
}

// Returns "equal, ok". If "ok" is false, then nothing is known about the two
// values. If "ok" is true, the equality or inequality of the two values is
// stored in "equal".
func checkEqualityIfNoSideEffects(left js_ast.E, right js_ast.E) (bool, bool) {
	if r, ok := right.(*js_ast.EInlinedEnum); ok {
		return checkEqualityIfNoSideEffects(left, r.Value.Data)
	}

	switch l := left.(type) {
	case *js_ast.EInlinedEnum:
		return checkEqualityIfNoSideEffects(l.Value.Data, right)

	case *js_ast.ENull:
		_, ok := right.(*js_ast.ENull)
		return ok, ok

	case *js_ast.EUndefined:
		_, ok := right.(*js_ast.EUndefined)
		return ok, ok

	case *js_ast.EBoolean:
		r, ok := right.(*js_ast.EBoolean)
		return ok && l.Value == r.Value, ok

	case *js_ast.ENumber:
		r, ok := right.(*js_ast.ENumber)
		return ok && l.Value == r.Value, ok

	case *js_ast.EBigInt:
		r, ok := right.(*js_ast.EBigInt)
		return ok && l.Value == r.Value, ok

	case *js_ast.EString:
		r, ok := right.(*js_ast.EString)
		return ok && js_lexer.UTF16EqualsUTF16(l.Value, r.Value), ok
	}

	return false, false
}

func valuesLookTheSame(left js_ast.E, right js_ast.E) bool {
	if b, ok := right.(*js_ast.EInlinedEnum); ok {
		return valuesLookTheSame(left, b.Value.Data)
	}

	switch a := left.(type) {
	case *js_ast.EInlinedEnum:
		return valuesLookTheSame(a.Value.Data, right)

	case *js_ast.EIdentifier:
		if b, ok := right.(*js_ast.EIdentifier); ok && a.Ref == b.Ref {
			return true
		}

	case *js_ast.EDot:
		if b, ok := right.(*js_ast.EDot); ok && a.HasSameFlagsAs(b) &&
			a.Name == b.Name && valuesLookTheSame(a.Target.Data, b.Target.Data) {
			return true
		}

	case *js_ast.EIndex:
		if b, ok := right.(*js_ast.EIndex); ok && a.HasSameFlagsAs(b) &&
			valuesLookTheSame(a.Target.Data, b.Target.Data) && valuesLookTheSame(a.Index.Data, b.Index.Data) {
			return true
		}

	case *js_ast.EIf:
		if b, ok := right.(*js_ast.EIf); ok && valuesLookTheSame(a.Test.Data, b.Test.Data) &&
			valuesLookTheSame(a.Yes.Data, b.Yes.Data) && valuesLookTheSame(a.No.Data, b.No.Data) {
			return true
		}

	case *js_ast.EUnary:
		if b, ok := right.(*js_ast.EUnary); ok && a.Op == b.Op && valuesLookTheSame(a.Value.Data, b.Value.Data) {
			return true
		}

	case *js_ast.EBinary:
		if b, ok := right.(*js_ast.EBinary); ok && a.Op == b.Op && valuesLookTheSame(a.Left.Data, b.Left.Data) &&
			valuesLookTheSame(a.Right.Data, b.Right.Data) {
			return true
		}

	case *js_ast.ECall:
		if b, ok := right.(*js_ast.ECall); ok && a.HasSameFlagsAs(b) &&
			len(a.Args) == len(b.Args) && valuesLookTheSame(a.Target.Data, b.Target.Data) {
			for i := range a.Args {
				if !valuesLookTheSame(a.Args[i].Data, b.Args[i].Data) {
					return false
				}
			}
			return true
		}

	// Special-case to distinguish between negative an non-negative zero when mangling
	// "a ? -0 : 0" => "a ? -0 : 0"
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Equality_comparisons_and_sameness
	case *js_ast.ENumber:
		b, ok := right.(*js_ast.ENumber)
		if ok && a.Value == 0 && b.Value == 0 && math.Signbit(a.Value) != math.Signbit(b.Value) {
			return false
		}
	}

	equal, ok := checkEqualityIfNoSideEffects(left, right)
	return ok && equal
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
			(a.ValueOrNil.Data == nil || valuesLookTheSame(a.ValueOrNil.Data, b.ValueOrNil.Data))

	case *js_ast.SThrow:
		b, ok := right.(*js_ast.SThrow)
		return ok && valuesLookTheSame(a.Value.Data, b.Value.Data)
	}

	return false
}

func hasValueForThisInCall(expr js_ast.Expr) bool {
	switch expr.Data.(type) {
	case *js_ast.EDot, *js_ast.EIndex:
		return true

	default:
		return false
	}
}

func (p *parser) selectLocalKind(kind js_ast.LocalKind) js_ast.LocalKind {
	// Safari workaround: Automatically avoid TDZ issues when bundling
	if p.options.mode == config.ModeBundle && p.currentScope.Parent == nil {
		return js_ast.LocalVar
	}

	// Optimization: use "let" instead of "const" because it's shorter. This is
	// only done when bundling because assigning to "const" is only an error when
	// bundling.
	if p.options.mode == config.ModeBundle && kind == js_ast.LocalConst && p.options.mangleSyntax {
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
		Label:   js_ast.LocRef{Ref: js_ast.InvalidRef},
	}
	if parent != nil {
		parent.Children = append(parent.Children, scope)
		scope.StrictMode = parent.StrictMode
		scope.UseStrictLoc = parent.UseStrictLoc
	}
	p.currentScope = scope

	// Enforce that scope locations are strictly increasing to help catch bugs
	// where the pushed scopes are mistmatched between the first and second passes
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
			if kind != js_ast.SymbolHoistedFunction {
				scope.Members[name] = member
			}
		}
	}

	// Remember the length in case we call popAndDiscardScope() later
	scopeIndex := len(p.scopesInOrder)
	p.scopesInOrder = append(p.scopesInOrder, scopeOrder{loc, scope})
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
			if p.options.mode == config.ModeBundle && p.currentScope.Parent == nil && p.hasESModuleSyntax {
				continue
			}

			p.symbols[member.Ref.InnerIndex].MustNotBeRenamed = true
		}
	}

	p.currentScope = p.currentScope.Parent
}

func (p *parser) popAndDiscardScope(scopeIndex int) {
	// Move up to the parent scope
	toDiscard := p.currentScope
	parent := toDiscard.Parent
	p.currentScope = parent

	// Truncate the scope order where we started to pretend we never saw this scope
	p.scopesInOrder = p.scopesInOrder[:scopeIndex]

	// Remove the last child from the parent scope
	last := len(parent.Children) - 1
	if parent.Children[last] != toDiscard {
		panic("Internal error")
	}
	parent.Children = parent.Children[:last]
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

func (p *parser) newSymbol(kind js_ast.SymbolKind, name string) js_ast.Ref {
	ref := js_ast.Ref{SourceIndex: p.source.Index, InnerIndex: uint32(len(p.symbols))}
	p.symbols = append(p.symbols, js_ast.Symbol{
		Kind:         kind,
		OriginalName: name,
		Link:         js_ast.InvalidRef,
	})
	if p.options.ts.Parse {
		p.tsUseCounts = append(p.tsUseCounts, 0)
	}
	return ref
}

// This is similar to "js_ast.MergeSymbols" but it works with this parser's
// one-level symbol map instead of the linker's two-level symbol map. It also
// doesn't handle cycles since they shouldn't come up due to the way this
// function is used.
func (p *parser) mergeSymbols(old js_ast.Ref, new js_ast.Ref) {
	oldSymbol := &p.symbols[old.InnerIndex]
	newSymbol := &p.symbols[new.InnerIndex]
	oldSymbol.Link = new
	newSymbol.UseCountEstimate += oldSymbol.UseCountEstimate
	if oldSymbol.MustNotBeRenamed {
		newSymbol.MustNotBeRenamed = true
	}
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

func (p *parser) canMergeSymbols(scope *js_ast.Scope, existing js_ast.SymbolKind, new js_ast.SymbolKind) mergeResult {
	if existing == js_ast.SymbolUnbound {
		return mergeReplaceWithNew
	}

	// In TypeScript, imports are allowed to silently collide with symbols within
	// the module. Presumably this is because the imports may be type-only:
	//
	//   import {Foo} from 'bar'
	//   class Foo {}
	//
	if p.options.ts.Parse && existing == js_ast.SymbolImport {
		return mergeReplaceWithNew
	}

	// "enum Foo {} enum Foo {}"
	if new == js_ast.SymbolTSEnum && existing == js_ast.SymbolTSEnum {
		return mergeKeepExisting
	}

	// "namespace Foo { ... } enum Foo {}"
	if new == js_ast.SymbolTSEnum && existing == js_ast.SymbolTSNamespace {
		return mergeReplaceWithNew
	}

	// "namespace Foo { ... } namespace Foo { ... }"
	// "function Foo() {} namespace Foo { ... }"
	// "enum Foo {} namespace Foo { ... }"
	if new == js_ast.SymbolTSNamespace {
		switch existing {
		case js_ast.SymbolTSNamespace, js_ast.SymbolHoistedFunction, js_ast.SymbolGeneratorOrAsyncFunction, js_ast.SymbolTSEnum, js_ast.SymbolClass:
			return mergeKeepExisting
		}
	}

	// "var foo; var foo;"
	// "var foo; function foo() {}"
	// "function foo() {} var foo;"
	// "function *foo() {} function *foo() {}" but not "{ function *foo() {} function *foo() {} }"
	if new.IsHoistedOrFunction() && existing.IsHoistedOrFunction() &&
		(scope.Kind == js_ast.ScopeEntry || scope.Kind == js_ast.ScopeFunctionBody ||
			(new.IsHoisted() && existing.IsHoisted())) {
		return mergeKeepExisting
	}

	// "get #foo() {} set #foo() {}"
	// "set #foo() {} get #foo() {}"
	if (existing == js_ast.SymbolPrivateGet && new == js_ast.SymbolPrivateSet) ||
		(existing == js_ast.SymbolPrivateSet && new == js_ast.SymbolPrivateGet) {
		return mergeBecomePrivateGetSetPair
	}
	if (existing == js_ast.SymbolPrivateStaticGet && new == js_ast.SymbolPrivateStaticSet) ||
		(existing == js_ast.SymbolPrivateStaticSet && new == js_ast.SymbolPrivateStaticGet) {
		return mergeBecomePrivateStaticGetSetPair
	}

	// "try {} catch (e) { var e }"
	if existing == js_ast.SymbolCatchIdentifier && new == js_ast.SymbolHoisted {
		return mergeReplaceWithNew
	}

	// "function() { var arguments }"
	if existing == js_ast.SymbolArguments && new == js_ast.SymbolHoisted {
		return mergeKeepExisting
	}

	// "function() { let arguments }"
	if existing == js_ast.SymbolArguments && new != js_ast.SymbolHoisted {
		return mergeOverwriteWithNew
	}

	return mergeForbidden
}

func (p *parser) addSymbolAlreadyDeclaredError(name string, newLoc logger.Loc, oldLoc logger.Loc) {
	p.log.AddWithNotes(logger.Error, &p.tracker,
		js_lexer.RangeOfIdentifier(p.source, newLoc),
		fmt.Sprintf("The symbol %q has already been declared", name),

		[]logger.MsgData{p.tracker.MsgData(
			js_lexer.RangeOfIdentifier(p.source, oldLoc),
			fmt.Sprintf("The symbol %q was originally declared here:", name),
		)},
	)
}

func (p *parser) declareSymbol(kind js_ast.SymbolKind, loc logger.Loc, name string) js_ast.Ref {
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

		case mergeBecomePrivateGetSetPair:
			ref = existing.Ref
			symbol.Kind = js_ast.SymbolPrivateGetSetPair

		case mergeBecomePrivateStaticGetSetPair:
			ref = existing.Ref
			symbol.Kind = js_ast.SymbolPrivateStaticGetSetPair

		case mergeOverwriteWithNew:
		}
	}

	// Overwrite this name in the declaring scope
	p.currentScope.Members[name] = js_ast.ScopeMember{Ref: ref, Loc: loc}
	return ref

}

func (p *parser) hoistSymbols(scope *js_ast.Scope) {
	if !scope.Kind.StopsHoisting() {
	nextMember:
		for _, member := range scope.Members {
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
			if scope.Parent.Kind == js_ast.ScopeCatchBinding && symbol.Kind != js_ast.SymbolHoisted {
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
			if symbol.Kind == js_ast.SymbolHoistedFunction {
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
				hoistedRef := p.newSymbol(js_ast.SymbolHoisted, symbol.OriginalName)
				scope.Generated = append(scope.Generated, hoistedRef)
				if p.hoistedRefForSloppyModeBlockFn == nil {
					p.hoistedRefForSloppyModeBlockFn = make(map[js_ast.Ref]js_ast.Ref)
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
					symbol.MustNotBeRenamed = true
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
					if existingSymbol.Kind == js_ast.SymbolUnbound || existingSymbol.Kind == js_ast.SymbolHoisted ||
						(existingSymbol.Kind.IsFunction() && (s.Kind == js_ast.ScopeEntry || s.Kind == js_ast.ScopeFunctionBody)) {
						// Silently merge this symbol into the existing symbol
						symbol.Link = existingMember.Ref
						s.Members[symbol.OriginalName] = existingMember
						continue nextMember
					}

					// Otherwise if this isn't a catch identifier or "arguments", it's a collision
					if existingSymbol.Kind != js_ast.SymbolCatchIdentifier && existingSymbol.Kind != js_ast.SymbolArguments {
						// An identifier binding from a catch statement and a function
						// declaration can both silently shadow another hoisted symbol
						if symbol.Kind != js_ast.SymbolCatchIdentifier && symbol.Kind != js_ast.SymbolHoistedFunction {
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

func (p *parser) declareBinding(kind js_ast.SymbolKind, binding js_ast.Binding, opts parseStmtOpts) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		name := p.loadNameFromRef(b.Ref)
		if !opts.isTypeScriptDeclare || (opts.isNamespaceScope && opts.isExport) {
			b.Ref = p.declareSymbol(kind, binding.Loc, name)
		}

	case *js_ast.BArray:
		for _, i := range b.Items {
			p.declareBinding(kind, i.Binding, opts)
		}

	case *js_ast.BObject:
		for _, property := range b.Properties {
			p.declareBinding(kind, property.Value, opts)
		}

	default:
		panic("Internal error")
	}
}

func (p *parser) recordUsage(ref js_ast.Ref) {
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

func (p *parser) ignoreUsage(ref js_ast.Ref) {
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
	ref, ok := p.runtimeImports[name]
	if !ok {
		ref = p.newSymbol(js_ast.SymbolOther, name)
		p.moduleScope.Generated = append(p.moduleScope.Generated, ref)
		p.runtimeImports[name] = ref
	}
	p.recordUsage(ref)
	return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
}

func (p *parser) callRuntime(loc logger.Loc, name string, args []js_ast.Expr) js_ast.Expr {
	return js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
		Target: p.importFromRuntime(loc, name),
		Args:   args,
	}}
}

func (p *parser) valueToSubstituteForRequire(loc logger.Loc) js_ast.Expr {
	if p.source.Index != runtime.SourceIndex &&
		config.ShouldCallRuntimeRequire(p.options.mode, p.options.outputFormat) {
		return p.importFromRuntime(loc, "__require")
	}

	p.recordUsage(p.requireRef)
	return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.requireRef}}
}

func (p *parser) makePromiseRef() js_ast.Ref {
	if p.promiseRef == js_ast.InvalidRef {
		p.promiseRef = p.newSymbol(js_ast.SymbolUnbound, "Promise")
	}
	return p.promiseRef
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
func (p *parser) storeNameInRef(name string) js_ast.Ref {
	c := (*reflect.StringHeader)(unsafe.Pointer(&p.source.Contents))
	n := (*reflect.StringHeader)(unsafe.Pointer(&name))

	// Is the data in "name" a subset of the data in "p.source.Contents"?
	if n.Data >= c.Data && n.Data+uintptr(n.Len) < c.Data+uintptr(c.Len) {
		// The name is a slice of the file contents, so we can just reference it by
		// length and don't have to allocate anything. This is the common case.
		//
		// It's stored as a negative value so we'll crash if we try to use it. That
		// way we'll catch cases where we've forgotten to call loadNameFromRef().
		// The length is the negative part because we know it's non-zero.
		return js_ast.Ref{SourceIndex: -uint32(n.Len), InnerIndex: uint32(n.Data - c.Data)}
	} else {
		// The name is some memory allocated elsewhere. This is either an inline
		// string constant in the parser or an identifier with escape sequences
		// in the source code, which is very unusual. Stash it away for later.
		// This uses allocations but it should hopefully be very uncommon.
		ref := js_ast.Ref{SourceIndex: 0x80000000, InnerIndex: uint32(len(p.allocatedNames))}
		p.allocatedNames = append(p.allocatedNames, name)
		return ref
	}
}

// This is the inverse of storeNameInRef() above
func (p *parser) loadNameFromRef(ref js_ast.Ref) string {
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
}

func (p *parser) logExprErrors(errors *deferredErrors) {
	if errors.invalidExprDefaultValue.Len > 0 {
		p.log.Add(logger.Error, &p.tracker, errors.invalidExprDefaultValue, "Unexpected \"=\"")
	}

	if errors.invalidExprAfterQuestion.Len > 0 {
		r := errors.invalidExprAfterQuestion
		p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Unexpected %q", p.source.Contents[r.Loc.Start:r.Loc.Start+r.Len]))
	}

	if errors.arraySpreadFeature.Len > 0 {
		p.markSyntaxFeature(compat.ArraySpread, errors.arraySpreadFeature)
	}
}

// The "await" and "yield" expressions are never allowed in argument lists but
// may or may not be allowed otherwise depending on the details of the enclosing
// function or module. This needs to be handled when parsing an arrow function
// argument list because we don't know if these expressions are not allowed until
// we reach the "=>" token (or discover the absence of one).
//
// Specifically, for await:
//
//   // This is ok
//   async function foo() { (x = await y) }
//
//   // This is an error
//   async function foo() { (x = await y) => {} }
//
// And for yield:
//
//   // This is ok
//   function* foo() { (x = yield y) }
//
//   // This is an error
//   function* foo() { (x = yield y) => {} }
//
type deferredArrowArgErrors struct {
	invalidExprAwait logger.Range
	invalidExprYield logger.Range
}

func (p *parser) logArrowArgErrors(errors *deferredArrowArgErrors) {
	if errors.invalidExprAwait.Len > 0 {
		r := errors.invalidExprAwait
		p.log.Add(logger.Error, &p.tracker, r, "Cannot use an \"await\" expression here:")
	}

	if errors.invalidExprYield.Len > 0 {
		r := errors.invalidExprYield
		p.log.Add(logger.Error, &p.tracker, r, "Cannot use a \"yield\" expression here:")
	}
}

func (p *parser) keyNameForError(key js_ast.Expr) string {
	switch k := key.Data.(type) {
	case *js_ast.EString:
		return fmt.Sprintf("%q", js_lexer.UTF16ToString(k.Value))
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

// This assumes the caller has already checked for TStringLiteral or TNoSubstitutionTemplateLiteral
func (p *parser) parseStringLiteral() js_ast.Expr {
	var legacyOctalLoc logger.Loc
	loc := p.lexer.Loc()
	text := p.lexer.StringLiteral()
	if p.lexer.LegacyOctalLoc.Start > loc.Start {
		legacyOctalLoc = p.lexer.LegacyOctalLoc
	}
	value := js_ast.Expr{Loc: loc, Data: &js_ast.EString{
		Value:          text,
		LegacyOctalLoc: legacyOctalLoc,
		PreferTemplate: p.lexer.Token == js_lexer.TNoSubstitutionTemplateLiteral,
	}}
	p.lexer.Next()
	return value
}

type propertyOpts struct {
	asyncRange     logger.Range
	tsDeclareRange logger.Range
	isAsync        bool
	isGenerator    bool

	// Class-related options
	isStatic          bool
	isTSAbstract      bool
	isClass           bool
	classHasExtends   bool
	allowTSDecorators bool
	tsDecorators      []js_ast.Expr
}

func (p *parser) parseProperty(kind js_ast.PropertyKind, opts propertyOpts, errors *deferredErrors) (js_ast.Property, bool) {
	var key js_ast.Expr
	keyRange := p.lexer.Range()
	isComputed := false
	preferQuotedKey := false

	switch p.lexer.Token {
	case js_lexer.TNumericLiteral:
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.ENumber{Value: p.lexer.Number}}
		p.checkForLegacyOctalLiteral(key.Data)
		p.lexer.Next()

	case js_lexer.TStringLiteral:
		key = p.parseStringLiteral()
		preferQuotedKey = !p.options.mangleSyntax

	case js_lexer.TBigIntegerLiteral:
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EBigInt{Value: p.lexer.Identifier}}
		p.markSyntaxFeature(compat.BigInt, p.lexer.Range())
		p.lexer.Next()

	case js_lexer.TPrivateIdentifier:
		if !opts.isClass || len(opts.tsDecorators) > 0 {
			p.lexer.Expected(js_lexer.TIdentifier)
		}
		if opts.tsDeclareRange.Len != 0 {
			p.log.Add(logger.Error, &p.tracker, opts.tsDeclareRange, "\"declare\" cannot be used with a private identifier")
		}
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EPrivateIdentifier{Ref: p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Next()

	case js_lexer.TOpenBracket:
		isComputed = true
		p.markSyntaxFeature(compat.ObjectExtensions, p.lexer.Range())
		p.lexer.Next()
		wasIdentifier := p.lexer.Token == js_lexer.TIdentifier
		expr := p.parseExpr(js_ast.LComma)

		// Handle index signatures
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon && wasIdentifier && opts.isClass {
			if _, ok := expr.Data.(*js_ast.EIdentifier); ok {
				if opts.tsDeclareRange.Len != 0 {
					p.log.Add(logger.Error, &p.tracker, opts.tsDeclareRange, "\"declare\" cannot be used with an index signature")
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

		p.lexer.Expect(js_lexer.TCloseBracket)
		key = expr

	case js_lexer.TAsterisk:
		if kind != js_ast.PropertyNormal || opts.isGenerator {
			p.lexer.Unexpected()
		}
		p.lexer.Next()
		opts.isGenerator = true
		return p.parseProperty(js_ast.PropertyNormal, opts, errors)

	default:
		name := p.lexer.Identifier
		raw := p.lexer.Raw()
		nameRange := p.lexer.Range()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(js_lexer.TIdentifier)
		}
		p.lexer.Next()

		// Support contextual keywords
		if kind == js_ast.PropertyNormal && !opts.isGenerator {
			// Does the following token look like a key?
			couldBeModifierKeyword := p.lexer.IsIdentifierOrKeyword()
			if !couldBeModifierKeyword {
				switch p.lexer.Token {
				case js_lexer.TOpenBracket, js_lexer.TNumericLiteral, js_lexer.TStringLiteral,
					js_lexer.TAsterisk, js_lexer.TPrivateIdentifier:
					couldBeModifierKeyword = true
				}
			}

			// If so, check for a modifier keyword
			if couldBeModifierKeyword {
				switch name {
				case "get":
					if !opts.isAsync && raw == name {
						p.markSyntaxFeature(compat.ObjectAccessors, nameRange)
						return p.parseProperty(js_ast.PropertyGet, opts, nil)
					}

				case "set":
					if !opts.isAsync && raw == name {
						p.markSyntaxFeature(compat.ObjectAccessors, nameRange)
						return p.parseProperty(js_ast.PropertySet, opts, nil)
					}

				case "async":
					if !opts.isAsync && raw == name && !p.lexer.HasNewlineBefore {
						opts.isAsync = true
						opts.asyncRange = nameRange
						p.markLoweredSyntaxFeature(compat.AsyncAwait, nameRange, compat.Generator)
						return p.parseProperty(kind, opts, nil)
					}

				case "static":
					if !opts.isStatic && !opts.isAsync && opts.isClass && raw == name {
						opts.isStatic = true
						return p.parseProperty(kind, opts, nil)
					}

				case "declare":
					if opts.isClass && p.options.ts.Parse && opts.tsDeclareRange.Len == 0 && raw == name {
						opts.tsDeclareRange = nameRange
						scopeIndex := len(p.scopesInOrder)

						if prop, ok := p.parseProperty(kind, opts, nil); ok &&
							prop.Kind == js_ast.PropertyNormal && prop.ValueOrNil.Data == nil {
							// If this is a well-formed class field with the "declare" keyword,
							// keep the declaration to preserve its side-effects, which may
							// include the computed key and/or the TypeScript decorators:
							//
							//   class Foo {
							//     declare [(console.log('side effect 1'), 'foo')]
							//     @decorator(console.log('side effect 2')) declare bar
							//   }
							//
							prop.Kind = js_ast.PropertyDeclare
							return prop, true
						}

						p.discardScopesUpTo(scopeIndex)
						return js_ast.Property{}, false
					}

				case "abstract":
					if opts.isClass && p.options.ts.Parse && !opts.isTSAbstract && raw == name {
						opts.isTSAbstract = true
						scopeIndex := len(p.scopesInOrder)
						p.parseProperty(kind, opts, nil)
						p.discardScopesUpTo(scopeIndex)
						return js_ast.Property{}, false
					}

				case "private", "protected", "public", "readonly", "override":
					// Skip over TypeScript keywords
					if opts.isClass && p.options.ts.Parse && raw == name {
						return p.parseProperty(kind, opts, nil)
					}
				}
			} else if p.lexer.Token == js_lexer.TOpenBrace && name == "static" {
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

				p.lexer.Expect(js_lexer.TCloseBrace)
				return js_ast.Property{
					Kind: js_ast.PropertyClassStaticBlock,
					ClassStaticBlock: &js_ast.ClassStaticBlock{
						Loc:   loc,
						Stmts: stmts,
					},
				}, true
			}
		}

		key = js_ast.Expr{Loc: nameRange.Loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(name)}}

		// Parse a shorthand property
		if !opts.isClass && kind == js_ast.PropertyNormal && p.lexer.Token != js_lexer.TColon &&
			p.lexer.Token != js_lexer.TOpenParen && p.lexer.Token != js_lexer.TLessThan &&
			!opts.isGenerator && !opts.isAsync && js_lexer.Keywords[name] == js_lexer.T(0) {
			if (p.fnOrArrowDataParse.await != allowIdent && name == "await") || (p.fnOrArrowDataParse.yield != allowIdent && name == "yield") {
				p.log.Add(logger.Error, &p.tracker, nameRange, fmt.Sprintf("Cannot use %q as an identifier here:", name))
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
				Key:              key,
				ValueOrNil:       value,
				InitializerOrNil: initializerOrNil,
				WasShorthand:     true,
			}, true
		}
	}

	if p.options.ts.Parse {
		// "class X { foo?: number }"
		// "class X { foo!: number }"
		if opts.isClass && (p.lexer.Token == js_lexer.TQuestion ||
			(p.lexer.Token == js_lexer.TExclamation && !p.lexer.HasNewlineBefore)) {
			p.lexer.Next()
		}

		// "class X { foo?<T>(): T }"
		// "const x = { foo<T>(): T {} }"
		p.skipTypeScriptTypeParameters()
	}

	// Parse a class field with an optional initial value
	if opts.isClass && kind == js_ast.PropertyNormal && !opts.isAsync &&
		!opts.isGenerator && p.lexer.Token != js_lexer.TOpenParen {
		var initializerOrNil js_ast.Expr

		// Forbid the names "constructor" and "prototype" in some cases
		if !isComputed {
			if str, ok := key.Data.(*js_ast.EString); ok && (js_lexer.UTF16EqualsString(str.Value, "constructor") ||
				(opts.isStatic && js_lexer.UTF16EqualsString(str.Value, "prototype"))) {
				p.log.Add(logger.Error, &p.tracker, keyRange, fmt.Sprintf("Invalid field name %q", js_lexer.UTF16ToString(str.Value)))
			}
		}

		// Skip over types
		if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon {
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)
		}

		if p.lexer.Token == js_lexer.TEquals {
			if opts.tsDeclareRange.Len != 0 {
				p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Class fields that use \"declare\" cannot be initialized")
			}

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
				p.log.Add(logger.Error, &p.tracker, keyRange, fmt.Sprintf("Invalid field name %q", name))
			}
			var declare js_ast.SymbolKind
			if opts.isStatic {
				declare = js_ast.SymbolPrivateStaticField
			} else {
				declare = js_ast.SymbolPrivateField
			}
			private.Ref = p.declareSymbol(declare, key.Loc, name)
		}

		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Property{
			TSDecorators:     opts.tsDecorators,
			Kind:             kind,
			IsComputed:       isComputed,
			PreferQuotedKey:  preferQuotedKey,
			IsStatic:         opts.isStatic,
			Key:              key,
			InitializerOrNil: initializerOrNil,
		}, true
	}

	// Parse a method expression
	if p.lexer.Token == js_lexer.TOpenParen || kind != js_ast.PropertyNormal ||
		opts.isClass || opts.isAsync || opts.isGenerator {
		if opts.tsDeclareRange.Len != 0 {
			what := "method"
			if kind == js_ast.PropertyGet {
				what = "getter"
			} else if kind == js_ast.PropertySet {
				what = "setter"
			}
			p.log.Add(logger.Error, &p.tracker, opts.tsDeclareRange, "\"declare\" cannot be used with a "+what)
		}

		if p.lexer.Token == js_lexer.TOpenParen && kind != js_ast.PropertyGet && kind != js_ast.PropertySet {
			p.markSyntaxFeature(compat.ObjectExtensions, p.lexer.Range())
		}
		loc := p.lexer.Loc()
		scopeIndex := p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, loc)
		isConstructor := false

		// Forbid the names "constructor" and "prototype" in some cases
		if opts.isClass && !isComputed {
			if str, ok := key.Data.(*js_ast.EString); ok {
				if !opts.isStatic && js_lexer.UTF16EqualsString(str.Value, "constructor") {
					switch {
					case kind == js_ast.PropertyGet:
						p.log.Add(logger.Error, &p.tracker, keyRange, "Class constructor cannot be a getter")
					case kind == js_ast.PropertySet:
						p.log.Add(logger.Error, &p.tracker, keyRange, "Class constructor cannot be a setter")
					case opts.isAsync:
						p.log.Add(logger.Error, &p.tracker, keyRange, "Class constructor cannot be an async function")
					case opts.isGenerator:
						p.log.Add(logger.Error, &p.tracker, keyRange, "Class constructor cannot be a generator")
					default:
						isConstructor = true
					}
				} else if opts.isStatic && js_lexer.UTF16EqualsString(str.Value, "prototype") {
					p.log.Add(logger.Error, &p.tracker, keyRange, "Invalid static method name \"prototype\"")
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

		fn, hadBody := p.parseFn(nil, fnOrArrowDataParse{
			needsAsyncLoc:      key.Loc,
			asyncRange:         opts.asyncRange,
			await:              await,
			yield:              yield,
			allowSuperCall:     opts.classHasExtends && isConstructor,
			allowSuperProperty: true,
			allowTSDecorators:  opts.allowTSDecorators,
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
		case js_ast.PropertyGet:
			if len(fn.Args) > 0 {
				r := js_lexer.RangeOfIdentifier(p.source, fn.Args[0].Binding.Loc)
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Getter %s must have zero arguments", p.keyNameForError(key)))
			}

		case js_ast.PropertySet:
			if len(fn.Args) != 1 {
				r := js_lexer.RangeOfIdentifier(p.source, key.Loc)
				if len(fn.Args) > 1 {
					r = js_lexer.RangeOfIdentifier(p.source, fn.Args[1].Binding.Loc)
				}
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Setter %s must have exactly one argument", p.keyNameForError(key)))
			}
		}

		// Special-case private identifiers
		if private, ok := key.Data.(*js_ast.EPrivateIdentifier); ok {
			var declare js_ast.SymbolKind
			var suffix string
			switch kind {
			case js_ast.PropertyGet:
				if opts.isStatic {
					declare = js_ast.SymbolPrivateStaticGet
				} else {
					declare = js_ast.SymbolPrivateGet
				}
				suffix = "_get"
			case js_ast.PropertySet:
				if opts.isStatic {
					declare = js_ast.SymbolPrivateStaticSet
				} else {
					declare = js_ast.SymbolPrivateSet
				}
				suffix = "_set"
			default:
				if opts.isStatic {
					declare = js_ast.SymbolPrivateStaticMethod
				} else {
					declare = js_ast.SymbolPrivateMethod
				}
				suffix = "_fn"
			}
			name := p.loadNameFromRef(private.Ref)
			if name == "#constructor" {
				p.log.Add(logger.Error, &p.tracker, keyRange, fmt.Sprintf("Invalid method name %q", name))
			}
			private.Ref = p.declareSymbol(declare, key.Loc, name)
			methodRef := p.newSymbol(js_ast.SymbolOther, name[1:]+suffix)
			if kind == js_ast.PropertySet {
				p.privateSetters[private.Ref] = methodRef
			} else {
				p.privateGetters[private.Ref] = methodRef
			}
		}

		return js_ast.Property{
			TSDecorators:    opts.tsDecorators,
			Kind:            kind,
			IsComputed:      isComputed,
			PreferQuotedKey: preferQuotedKey,
			IsMethod:        true,
			IsStatic:        opts.isStatic,
			Key:             key,
			ValueOrNil:      value,
		}, true
	}

	// Parse an object key/value pair
	p.lexer.Expect(js_lexer.TColon)
	value := p.parseExprOrBindings(js_ast.LComma, errors)
	return js_ast.Property{
		Kind:            kind,
		IsComputed:      isComputed,
		PreferQuotedKey: preferQuotedKey,
		Key:             key,
		ValueOrNil:      value,
	}, true
}

func (p *parser) parsePropertyBinding() js_ast.PropertyBinding {
	var key js_ast.Expr
	isComputed := false
	preferQuotedKey := false

	switch p.lexer.Token {
	case js_lexer.TDotDotDot:
		p.lexer.Next()
		value := js_ast.Binding{Loc: p.lexer.Loc(), Data: &js_ast.BIdentifier{Ref: p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Expect(js_lexer.TIdentifier)
		return js_ast.PropertyBinding{
			IsSpread: true,
			Value:    value,
		}

	case js_lexer.TNumericLiteral:
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.ENumber{Value: p.lexer.Number}}
		p.checkForLegacyOctalLiteral(key.Data)
		p.lexer.Next()

	case js_lexer.TStringLiteral:
		key = p.parseStringLiteral()
		preferQuotedKey = !p.options.mangleSyntax

	case js_lexer.TBigIntegerLiteral:
		key = js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EBigInt{Value: p.lexer.Identifier}}
		p.markSyntaxFeature(compat.BigInt, p.lexer.Range())
		p.lexer.Next()

	case js_lexer.TOpenBracket:
		isComputed = true
		p.lexer.Next()
		key = p.parseExpr(js_ast.LComma)
		p.lexer.Expect(js_lexer.TCloseBracket)

	default:
		name := p.lexer.Identifier
		loc := p.lexer.Loc()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(js_lexer.TIdentifier)
		}
		p.lexer.Next()
		key = js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(name)}}

		if p.lexer.Token != js_lexer.TColon && p.lexer.Token != js_lexer.TOpenParen {
			ref := p.storeNameInRef(name)
			value := js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ref}}

			var defaultValueOrNil js_ast.Expr
			if p.lexer.Token == js_lexer.TEquals {
				p.lexer.Next()
				defaultValueOrNil = p.parseExpr(js_ast.LComma)
			}

			return js_ast.PropertyBinding{
				Key:               key,
				Value:             value,
				DefaultValueOrNil: defaultValueOrNil,
			}
		}
	}

	p.lexer.Expect(js_lexer.TColon)
	value := p.parseBinding()

	var defaultValueOrNil js_ast.Expr
	if p.lexer.Token == js_lexer.TEquals {
		p.lexer.Next()
		defaultValueOrNil = p.parseExpr(js_ast.LComma)
	}

	return js_ast.PropertyBinding{
		IsComputed:        isComputed,
		PreferQuotedKey:   preferQuotedKey,
		Key:               key,
		Value:             value,
		DefaultValueOrNil: defaultValueOrNil,
	}
}

func (p *parser) parseArrowBody(args []js_ast.Arg, data fnOrArrowDataParse) *js_ast.EArrow {
	arrowLoc := p.lexer.Loc()

	// Newlines are not allowed before "=>"
	if p.lexer.HasNewlineBefore {
		p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Unexpected newline before \"=>\"")
		panic(js_lexer.LexerPanic{})
	}

	p.lexer.Expect(js_lexer.TEqualsGreaterThan)

	for _, arg := range args {
		p.declareBinding(js_ast.SymbolHoisted, arg.Binding, parseStmtOpts{})
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
		Body:       js_ast.FnBody{Loc: arrowLoc, Stmts: []js_ast.Stmt{{Loc: expr.Loc, Data: &js_ast.SReturn{ValueOrNil: expr}}}},
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
				arg := js_ast.Arg{Binding: js_ast.Binding{Loc: asyncRange.Loc, Data: &js_ast.BIdentifier{Ref: p.storeNameInRef("async")}}}

				p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, asyncRange.Loc)
				defer p.popScope()

				return js_ast.Expr{Loc: asyncRange.Loc, Data: p.parseArrowBody([]js_ast.Arg{arg}, fnOrArrowDataParse{
					needsAsyncLoc: asyncRange.Loc,
				})}
			}

		// "async x => {}"
		case js_lexer.TIdentifier:
			if level <= js_ast.LAssign {
				// See https://github.com/tc39/ecma262/issues/2034 for details
				isArrowFn := true
				if (flags&exprFlagForLoopInit) != 0 && p.lexer.Identifier == "of" {
					// "for (async of" is only an arrow function if the next token is "=>"
					isArrowFn = p.checkForArrowAfterTheCurrentToken()

					// Do not allow "for (async of []) ;" but do allow "for await (async of []) ;"
					if !isArrowFn && (flags&exprFlagForAwaitLoopInit) == 0 && p.lexer.Raw() == "of" {
						r := logger.Range{Loc: asyncRange.Loc, Len: p.lexer.Range().End() - asyncRange.Loc.Start}
						p.log.Add(logger.Error, &p.tracker, r, "For loop initializers cannot start with \"async of\"")
						panic(js_lexer.LexerPanic{})
					}
				}

				if isArrowFn {
					p.markLoweredSyntaxFeature(compat.AsyncAwait, asyncRange, compat.Generator)
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
			return p.parseParenExpr(asyncRange.Loc, level, parenExprOpts{isAsync: true, asyncRange: asyncRange})

		// "async<T>()"
		// "async <T>() => {}"
		case js_lexer.TLessThan:
			if p.options.ts.Parse && p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() {
				p.lexer.Next()
				return p.parseParenExpr(asyncRange.Loc, level, parenExprOpts{isAsync: true, asyncRange: asyncRange})
			}
		}
	}

	// "async"
	// "async + 1"
	return js_ast.Expr{Loc: asyncRange.Loc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef("async")}}
}

func (p *parser) parseFnExpr(loc logger.Loc, isAsync bool, asyncRange logger.Range) js_ast.Expr {
	p.lexer.Next()
	isGenerator := p.lexer.Token == js_lexer.TAsterisk
	if isGenerator {
		p.markSyntaxFeature(compat.Generator, p.lexer.Range())
		p.lexer.Next()
	} else if isAsync {
		p.markLoweredSyntaxFeature(compat.AsyncAwait, asyncRange, compat.Generator)
	}
	var name *js_ast.LocRef

	p.pushScopeForParsePass(js_ast.ScopeFunctionArgs, loc)
	defer p.popScope()

	// The name is optional
	if p.lexer.Token == js_lexer.TIdentifier {
		// Don't declare the name "arguments" since it's shadowed and inaccessible
		name = &js_ast.LocRef{Loc: p.lexer.Loc()}
		if text := p.lexer.Identifier; text != "arguments" {
			name.Ref = p.declareSymbol(js_ast.SymbolHoistedFunction, name.Loc, text)
		} else {
			name.Ref = p.newSymbol(js_ast.SymbolHoistedFunction, text)
		}
		p.lexer.Next()
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters()
	}

	await := allowIdent
	yield := allowIdent
	if isAsync {
		await = allowExpr
	}
	if isGenerator {
		yield = allowExpr
	}

	fn, _ := p.parseFn(name, fnOrArrowDataParse{
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
	isAsync      bool
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

		if opts.isAsync {
			p.markLoweredSyntaxFeature(compat.AsyncAwait, opts.asyncRange, compat.Generator)
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
				p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: commaAfterSpread, Len: 1}, "Unexpected \",\" after rest pattern")
			}
			p.logArrowArgErrors(&arrowArgErrors)

			// Now that we've decided we're an arrow function, report binding pattern
			// conversion errors
			if len(invalidLog.invalidTokens) > 0 {
				for _, token := range invalidLog.invalidTokens {
					p.log.Add(logger.Error, &p.tracker, token, "Invalid binding pattern")
				}
				panic(js_lexer.LexerPanic{})
			}

			// Also report syntax features used in bindings
			for _, entry := range invalidLog.syntaxFeatures {
				p.markSyntaxFeature(entry.feature, entry.token)
			}

			await := allowIdent
			if opts.isAsync {
				await = allowExpr
			}

			arrow := p.parseArrowBody(args, fnOrArrowDataParse{
				needsAsyncLoc: loc,
				await:         await,
			})
			arrow.IsAsync = opts.isAsync
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
		p.log.Add(logger.Error, &p.tracker, typeColonRange, "Unexpected \":\"")
		panic(js_lexer.LexerPanic{})
	}

	// Are these arguments for a call to a function named "async"?
	if opts.isAsync {
		p.logExprErrors(&errors)
		async := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef("async")}}
		return js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
			Target: async,
			Args:   items,
		}}
	}

	// Is this a chain of expressions and comma operators?
	if len(items) > 0 {
		p.logExprErrors(&errors)
		if spreadRange.Len > 0 {
			p.log.Add(logger.Error, &p.tracker, spreadRange, "Unexpected \"...\"")
			panic(js_lexer.LexerPanic{})
		}
		value := js_ast.JoinAllWithComma(items)
		p.markExprAsParenthesized(value)
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
			p.log.Add(logger.Error, &p.tracker, equalsRange, "A rest argument cannot have a default initializer")
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
		if e.IsParenthesized {
			invalidLog.invalidTokens = append(invalidLog.invalidTokens, p.source.RangeOfOperatorBefore(expr.Loc, "("))
		}
		p.markSyntaxFeature(compat.Destructuring, p.source.RangeOfOperatorAfter(expr.Loc, "["))
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
			items = append(items, js_ast.ArrayBinding{Binding: binding, DefaultValueOrNil: initializerOrNil})
		}
		return js_ast.Binding{Loc: expr.Loc, Data: &js_ast.BArray{
			Items:        items,
			HasSpread:    isSpread,
			IsSingleLine: e.IsSingleLine,
		}}, invalidLog

	case *js_ast.EObject:
		if e.CommaAfterSpread.Start != 0 {
			invalidLog.invalidTokens = append(invalidLog.invalidTokens, logger.Range{Loc: e.CommaAfterSpread, Len: 1})
		}
		if e.IsParenthesized {
			invalidLog.invalidTokens = append(invalidLog.invalidTokens, p.source.RangeOfOperatorBefore(expr.Loc, "("))
		}
		p.markSyntaxFeature(compat.Destructuring, p.source.RangeOfOperatorAfter(expr.Loc, "{"))
		properties := []js_ast.PropertyBinding{}
		for _, item := range e.Properties {
			if item.IsMethod || item.Kind == js_ast.PropertyGet || item.Kind == js_ast.PropertySet {
				invalidLog.invalidTokens = append(invalidLog.invalidTokens, js_lexer.RangeOfIdentifier(p.source, item.Key.Loc))
				continue
			}
			binding, initializerOrNil, log := p.convertExprToBindingAndInitializer(item.ValueOrNil, invalidLog, false)
			invalidLog = log
			if initializerOrNil.Data == nil {
				initializerOrNil = item.InitializerOrNil
			}
			properties = append(properties, js_ast.PropertyBinding{
				IsSpread:          item.Kind == js_ast.PropertySpread,
				IsComputed:        item.IsComputed,
				Key:               item.Key,
				Value:             binding,
				DefaultValueOrNil: initializerOrNil,
			})
		}
		return js_ast.Binding{Loc: expr.Loc, Data: &js_ast.BObject{
			Properties:   properties,
			IsSingleLine: e.IsSingleLine,
		}}, invalidLog

	default:
		invalidLog.invalidTokens = append(invalidLog.invalidTokens, logger.Range{Loc: expr.Loc})
		return js_ast.Binding{}, invalidLog
	}
}

type exprFlag uint8

const (
	exprFlagTSDecorator exprFlag = 1 << iota
	exprFlagForLoopInit
	exprFlagForAwaitLoopInit
)

func (p *parser) parsePrefix(level js_ast.L, errors *deferredErrors, flags exprFlag) js_ast.Expr {
	loc := p.lexer.Loc()

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

		p.log.Add(logger.Error, &p.tracker, superRange, "Unexpected \"super\"")
		return js_ast.Expr{Loc: loc, Data: js_ast.ESuperShared}

	case js_lexer.TOpenParen:
		p.lexer.Next()

		// Arrow functions aren't allowed in the middle of expressions
		if level > js_ast.LAssign {
			// Allow "in" inside parentheses
			oldAllowIn := p.allowIn
			p.allowIn = true

			value := p.parseExpr(js_ast.LLowest)
			p.markExprAsParenthesized(value)
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
			p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Cannot use \"this\" here:")
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
			if p.classPrivateBrandChecksToLower == nil {
				p.classPrivateBrandChecksToLower = make(map[string]bool)
			}
			p.classPrivateBrandChecksToLower[name] = true
		}

		return js_ast.Expr{Loc: loc, Data: &js_ast.EPrivateIdentifier{Ref: p.storeNameInRef(name)}}

	case js_lexer.TIdentifier:
		name := p.lexer.Identifier
		nameRange := p.lexer.Range()
		raw := p.lexer.Raw()
		p.lexer.Next()

		// Handle async and await expressions
		switch name {
		case "async":
			if raw == "async" {
				return p.parseAsyncPrefixExpr(nameRange, level, flags)
			}

		case "await":
			switch p.fnOrArrowDataParse.await {
			case forbidAll:
				p.log.Add(logger.Error, &p.tracker, nameRange, "The keyword \"await\" cannot be used here:")

			case allowExpr:
				if raw != "await" {
					p.log.Add(logger.Error, &p.tracker, nameRange, "The keyword \"await\" cannot be escaped")
				} else {
					if p.fnOrArrowDataParse.isTopLevel {
						p.topLevelAwaitKeyword = nameRange
						p.markSyntaxFeature(compat.TopLevelAwait, nameRange)
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
				p.log.Add(logger.Error, &p.tracker, nameRange, "The keyword \"yield\" cannot be used here:")

			case allowExpr:
				if raw != "yield" {
					p.log.Add(logger.Error, &p.tracker, nameRange, "The keyword \"yield\" cannot be escaped")
				} else {
					if level > js_ast.LAssign {
						p.log.Add(logger.Error, &p.tracker, nameRange, "Cannot use a \"yield\" expression here without parentheses:")
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
						p.log.Add(logger.Error, &p.tracker, nameRange, "Cannot use \"yield\" outside a generator function")
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
		p.markSyntaxFeature(compat.BigInt, p.lexer.Range())
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBigInt{Value: value}}

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
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpTypeof, Value: value}}

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
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Deleting the private name %q is forbidden", name))
			}
		}
		return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{Op: js_ast.UnOpDelete, Value: value}}

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
		classKeyword := p.lexer.Range()
		p.markSyntaxFeature(compat.Class, classKeyword)
		p.lexer.Next()
		var name *js_ast.LocRef

		p.pushScopeForParsePass(js_ast.ScopeClassName, loc)

		// Parse an optional class name
		if p.lexer.Token == js_lexer.TIdentifier {
			if nameText := p.lexer.Identifier; !p.options.ts.Parse || nameText != "implements" {
				if p.fnOrArrowDataParse.await != allowIdent && nameText == "await" {
					p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Cannot use \"await\" as an identifier here:")
				}
				name = &js_ast.LocRef{Loc: p.lexer.Loc(), Ref: p.newSymbol(js_ast.SymbolOther, nameText)}
				p.lexer.Next()
			}
		}

		// Even anonymous classes can have TypeScript type parameters
		if p.options.ts.Parse {
			p.skipTypeScriptTypeParameters()
		}

		class := p.parseClass(classKeyword, name, parseClassOpts{})

		p.popScope()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EClass{Class: class}}

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

		if p.lexer.Token == js_lexer.TOpenParen {
			args = p.parseCallArgs()
		}

		return js_ast.Expr{Loc: loc, Data: &js_ast.ENew{Target: target, Args: args}}

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
				dotsLoc := p.lexer.Loc()
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
				p.lexer.Next()
				value := p.parseExpr(js_ast.LComma)
				properties = append(properties, js_ast.Property{
					Kind:       js_ast.PropertySpread,
					ValueOrNil: value,
				})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == js_lexer.TComma {
					commaAfterSpread = p.lexer.Loc()
				}
			} else {
				// This property may turn out to be a type in TypeScript, which should be ignored
				if property, ok := p.parseProperty(js_ast.PropertyNormal, propertyOpts{}, &selfErrors); ok {
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
		//
		//   An arrow function with type parameters:
		//     <A>(x) => {}
		//     <A, B>(x) => {}
		//     <A = B>(x) => {}
		//     <A extends B>(x) => {}
		//
		// TSX:
		//
		//   A JSX element:
		//     <A>(x) => {}</A>
		//     <A extends>(x) => {}</A>
		//     <A extends={false}>(x) => {}</A>
		//
		//   An arrow function with type parameters:
		//     <A, B>(x) => {}
		//     <A extends B>(x) => {}
		//
		//   A syntax error:
		//     <[]>(x)
		//     <A[]>(x)
		//     <A>(x) => {}
		//     <A = B>(x) => {}

		if p.options.ts.Parse && p.options.jsx.Parse && p.isTSArrowFnJSX() {
			p.skipTypeScriptTypeParameters()
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
			p.log.AddWithNotes(logger.Error, &p.tracker, p.lexer.Range(), "The JSX syntax extension is not currently enabled", []logger.MsgData{{
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
				p.log.Add(logger.Error, &p.tracker, p.lexer.Range(),
					"This syntax is not allowed in files with the \".mts\" or \".cts\" extension")
			}

			// "<T>(x)"
			// "<T>(x) => {}"
			if p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() {
				p.lexer.Expect(js_lexer.TOpenParen)
				return p.parseParenExpr(loc, level, parenExprOpts{})
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
	if isStar {
		if p.lexer.HasNewlineBefore {
			p.lexer.Unexpected()
		}
		p.lexer.Next()
	}

	var valueOrNil js_ast.Expr

	// The yield expression only has a value in certain cases
	switch p.lexer.Token {
	case js_lexer.TCloseBrace, js_lexer.TCloseBracket, js_lexer.TCloseParen,
		js_lexer.TColon, js_lexer.TComma, js_lexer.TSemicolon:

	default:
		if isStar || !p.lexer.HasNewlineBefore {
			valueOrNil = p.parseExpr(js_ast.LYield)
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
		p.es6ImportKeyword = js_lexer.RangeOfIdentifier(p.source, loc)
		p.lexer.Next()
		if p.lexer.IsContextualKeyword("meta") {
			r := p.lexer.Range()
			p.lexer.Next()
			p.hasImportMeta = true
			if p.options.unsupportedJSFeatures.Has(compat.ImportMeta) {
				r = logger.Range{Loc: loc, Len: r.End() - loc.Start}
				p.markSyntaxFeature(compat.ImportMeta, r)
			}
			return js_ast.Expr{Loc: loc, Data: js_ast.EImportMetaShared}
		} else {
			p.lexer.ExpectedString("\"meta\"")
		}
	}

	if level > js_ast.LCall {
		r := js_lexer.RangeOfIdentifier(p.source, loc)
		p.log.Add(logger.Error, &p.tracker, r, "Cannot use an \"import\" expression here without parentheses:")
	}

	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	p.lexer.PreserveAllCommentsBefore = true
	p.lexer.Expect(js_lexer.TOpenParen)
	comments := p.lexer.CommentsToPreserveBefore
	p.lexer.PreserveAllCommentsBefore = false

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

	p.lexer.Expect(js_lexer.TCloseParen)

	p.allowIn = oldAllowIn
	return js_ast.Expr{Loc: loc, Data: &js_ast.EImportCall{
		Expr:                    value,
		OptionsOrNil:            optionsOrNil,
		LeadingInteriorComments: comments,
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
	hadPureCommentBefore := p.lexer.HasPureCommentBefore && !p.options.ignoreDCEAnnotations
	expr := p.parsePrefix(level, errors, flags)

	// There is no formal spec for "__PURE__" comments but from reverse-
	// engineering, it looks like they apply to the next CallExpression or
	// NewExpression. So in "/* @__PURE__ */ a().b() + c()" the comment applies
	// to the expression "a().b()".
	if hadPureCommentBefore && level < js_ast.LCall {
		expr = p.parseSuffix(expr, js_ast.LCall-1, errors, flags)
		switch e := expr.Data.(type) {
		case *js_ast.ECall:
			e.CanBeUnwrappedIfUnused = true
		case *js_ast.ENew:
			e.CanBeUnwrappedIfUnused = true
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
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EDot{
					Target:        left,
					Name:          name,
					NameLoc:       nameLoc,
					OptionalChain: oldOptionalChain,
				}}
			}

			optionalChain = oldOptionalChain

		case js_lexer.TQuestionDot:
			p.lexer.Next()
			optionalStart := js_ast.OptionalChainStart

			// Remove unnecessary optional chains
			if p.options.mangleSyntax {
				if isNullOrUndefined, _, ok := toNullOrUndefinedWithSideEffects(left.Data); ok && !isNullOrUndefined {
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

				p.lexer.Expect(js_lexer.TCloseBracket)
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIndex{
					Target:        left,
					Index:         index,
					OptionalChain: optionalStart,
				}}

			case js_lexer.TOpenParen:
				// "a?.()"
				if level >= js_ast.LCall {
					return left
				}
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ECall{
					Target:        left,
					Args:          p.parseCallArgs(),
					OptionalChain: optionalStart,
				}}

			case js_lexer.TLessThan:
				// "a?.<T>()"
				if !p.options.ts.Parse {
					p.lexer.Expected(js_lexer.TIdentifier)
				}
				p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
				if p.lexer.Token != js_lexer.TOpenParen {
					p.lexer.Expected(js_lexer.TOpenParen)
				}
				if level >= js_ast.LCall {
					return left
				}
				left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ECall{
					Target:        left,
					Args:          p.parseCallArgs(),
					OptionalChain: optionalStart,
				}}

			default:
				if p.lexer.Token == js_lexer.TPrivateIdentifier && p.allowPrivateIdentifiers {
					// "a?.#b"
					name := p.lexer.Identifier
					nameLoc := p.lexer.Loc()
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
					left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EDot{
						Target:        left,
						Name:          name,
						NameLoc:       nameLoc,
						OptionalChain: optionalStart,
					}}
				}
			}

			// Only continue if we have started
			if optionalStart == js_ast.OptionalChainStart {
				optionalChain = js_ast.OptionalChainContinue
			}

		case js_lexer.TNoSubstitutionTemplateLiteral:
			if oldOptionalChain != js_ast.OptionalChainNone {
				p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Template literals cannot have an optional chain as a tag")
			}
			headLoc := p.lexer.Loc()
			headCooked, headRaw := p.lexer.CookedAndRawTemplateContents()
			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
				TagOrNil:   left,
				HeadLoc:    headLoc,
				HeadCooked: headCooked,
				HeadRaw:    headRaw,
			}}

		case js_lexer.TTemplateHead:
			if oldOptionalChain != js_ast.OptionalChainNone {
				p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Template literals cannot have an optional chain as a tag")
			}
			headLoc := p.lexer.Loc()
			headCooked, headRaw := p.lexer.CookedAndRawTemplateContents()
			parts, _ := p.parseTemplateParts(true /* includeRaw */)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
				TagOrNil:   left,
				HeadLoc:    headLoc,
				HeadCooked: headCooked,
				HeadRaw:    headRaw,
				Parts:      parts,
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
			if (flags & exprFlagTSDecorator) != 0 {
				return left
			}

			p.lexer.Next()

			// Allow "in" inside the brackets
			oldAllowIn := p.allowIn
			p.allowIn = true

			index := p.parseExpr(js_ast.LLowest)

			p.allowIn = oldAllowIn

			p.lexer.Expect(js_lexer.TCloseBracket)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EIndex{
				Target:        left,
				Index:         index,
				OptionalChain: oldOptionalChain,
			}}
			optionalChain = oldOptionalChain

		case js_lexer.TOpenParen:
			if level >= js_ast.LCall {
				return left
			}
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.ECall{
				Target:        left,
				Args:          p.parseCallArgs(),
				OptionalChain: oldOptionalChain,
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
			if p.options.ts.Parse && p.trySkipTypeScriptTypeArgumentsWithBacktracking() {
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
				p.lexer.Unexpected()
			}

			p.lexer.Next()
			right := p.parseExpr(js_ast.LLogicalOr)
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLogicalOr, Left: left, Right: right}}

			// Prevent "||" inside "??" from the left
			if level < js_ast.LNullishCoalescing {
				left = p.parseSuffix(left, js_ast.LNullishCoalescing+1, nil, flags)
				if p.lexer.Token == js_lexer.TQuestionQuestion {
					p.lexer.Unexpected()
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
				p.lexer.Unexpected()
			}

			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpLogicalAnd, Left: left, Right: p.parseExpr(js_ast.LLogicalAnd)}}

			// Prevent "&&" inside "??" from the left
			if level < js_ast.LNullishCoalescing {
				left = p.parseSuffix(left, js_ast.LNullishCoalescing+1, nil, flags)
				if p.lexer.Token == js_lexer.TQuestionQuestion {
					p.lexer.Unexpected()
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
			if !p.suppressWarningsAboutWeirdCode {
				if e, ok := left.Data.(*js_ast.EUnary); ok && e.Op == js_ast.UnOpNot {
					r := logger.Range{Loc: left.Loc, Len: p.source.LocBeforeWhitespace(p.lexer.Loc()).Start - left.Loc.Start}
					data := p.tracker.MsgData(r, "Suspicious use of the \"!\" operator inside the \"in\" operator")
					data.Location.Suggestion = fmt.Sprintf("(%s)", p.source.TextForRange(r))
					p.log.AddMsg(logger.Msg{
						Kind: logger.Warning,
						Data: data,
						Notes: []logger.MsgData{{Text: "The code \"!x in y\" is parsed as \"(!x) in y\". " +
							"You need to insert parentheses to get \"!(x in y)\" instead."}},
					})
				}
			}

			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpIn, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		case js_lexer.TInstanceof:
			if level >= js_ast.LCompare {
				return left
			}

			// Warn about "!a instanceof b" instead of "!(a instanceof b)". Here's an
			// example of code with this problem: https://github.com/mrdoob/three.js/pull/11182.
			if !p.suppressWarningsAboutWeirdCode {
				if e, ok := left.Data.(*js_ast.EUnary); ok && e.Op == js_ast.UnOpNot {
					r := logger.Range{Loc: left.Loc, Len: p.source.LocBeforeWhitespace(p.lexer.Loc()).Start - left.Loc.Start}
					data := p.tracker.MsgData(r, "Suspicious use of the \"!\" operator inside the \"instanceof\" operator")
					data.Location.Suggestion = fmt.Sprintf("(%s)", p.source.TextForRange(r))
					p.log.AddMsg(logger.Msg{
						Kind: logger.Warning,
						Data: data,
						Notes: []logger.MsgData{{Text: "The code \"!x instanceof y\" is parsed as \"(!x) instanceof y\". " +
							"You need to insert parentheses to get \"!(x instanceof y)\" instead."}},
					})
				}
			}

			p.lexer.Next()
			left = js_ast.Expr{Loc: left.Loc, Data: &js_ast.EBinary{Op: js_ast.BinOpInstanceof, Left: left, Right: p.parseExpr(js_ast.LCompare)}}

		default:
			// Handle the TypeScript "as" operator
			if p.options.ts.Parse && level < js_ast.LCompare && !p.lexer.HasNewlineBefore && p.lexer.IsContextualKeyword("as") {
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

func (p *parser) parseExprOrLetStmt(opts parseStmtOpts) (js_ast.Expr, js_ast.Stmt, []js_ast.Decl) {
	letRange := p.lexer.Range()
	raw := p.lexer.Raw()

	if p.lexer.Token != js_lexer.TIdentifier || raw != "let" {
		var flags exprFlag
		if opts.isForLoopInit {
			flags |= exprFlagForLoopInit
		}
		if opts.isForAwaitLoopInit {
			flags |= exprFlagForAwaitLoopInit
		}
		return p.parseExprCommon(js_ast.LLowest, nil, flags), js_ast.Stmt{}, nil
	}

	p.lexer.Next()

	switch p.lexer.Token {
	case js_lexer.TIdentifier, js_lexer.TOpenBracket, js_lexer.TOpenBrace:
		if opts.lexicalDecl == lexicalDeclAllowAll || !p.lexer.HasNewlineBefore || p.lexer.Token == js_lexer.TOpenBracket {
			if opts.lexicalDecl != lexicalDeclAllowAll {
				p.forbidLexicalDecl(letRange.Loc)
			}
			p.markSyntaxFeature(compat.Let, letRange)
			decls := p.parseAndDeclareDecls(js_ast.SymbolOther, opts)
			return js_ast.Expr{}, js_ast.Stmt{Loc: letRange.Loc, Data: &js_ast.SLocal{
				Kind:     js_ast.LocalLet,
				Decls:    decls,
				IsExport: opts.isExport,
			}}, decls
		}
	}

	ref := p.storeNameInRef(raw)
	expr := js_ast.Expr{Loc: letRange.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
	return p.parseSuffix(expr, js_ast.LLowest, nil, 0), js_ast.Stmt{}, nil
}

func (p *parser) parseCallArgs() []js_ast.Expr {
	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	args := []js_ast.Expr{}
	p.lexer.Expect(js_lexer.TOpenParen)

	for p.lexer.Token != js_lexer.TCloseParen {
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
		p.lexer.Next()
	}

	p.lexer.Expect(js_lexer.TCloseParen)
	p.allowIn = oldAllowIn
	return args
}

func (p *parser) parseJSXTag() (logger.Range, string, js_ast.Expr) {
	loc := p.lexer.Loc()

	// A missing tag is a fragment
	if p.lexer.Token == js_lexer.TGreaterThan {
		return logger.Range{Loc: loc, Len: 0}, "", js_ast.Expr{}
	}

	// The tag is an identifier
	name := p.lexer.Identifier
	tagRange := p.lexer.Range()
	p.lexer.ExpectInsideJSXElement(js_lexer.TIdentifier)

	// Certain identifiers are strings
	if strings.ContainsAny(name, "-:") || (p.lexer.Token != js_lexer.TDot && name[0] >= 'a' && name[0] <= 'z') {
		return tagRange, name, js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(name)}}
	}

	// Otherwise, this is an identifier
	tag := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.storeNameInRef(name)}}

	// Parse a member expression chain
	for p.lexer.Token == js_lexer.TDot {
		p.lexer.NextInsideJSXElement()
		memberRange := p.lexer.Range()
		member := p.lexer.Identifier
		p.lexer.ExpectInsideJSXElement(js_lexer.TIdentifier)

		// Dashes are not allowed in member expression chains
		index := strings.IndexByte(member, '-')
		if index >= 0 {
			p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: logger.Loc{Start: memberRange.Loc.Start + int32(index)}},
				"Unexpected \"-\"")
			panic(js_lexer.LexerPanic{})
		}

		name += "." + member
		tag = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
			Target:  tag,
			Name:    member,
			NameLoc: memberRange.Loc,
		}}
		tagRange.Len = memberRange.Loc.Start + memberRange.Len - tagRange.Loc.Start
	}

	return tagRange, name, tag
}

func (p *parser) parseJSXElement(loc logger.Loc) js_ast.Expr {
	// Parse the tag
	startRange, startText, startTagOrNil := p.parseJSXTag()

	// The tag may have TypeScript type arguments: "<Foo<T>/>"
	if p.options.ts.Parse {
		// Pass a flag to the type argument skipper because we need to call
		// js_lexer.NextInsideJSXElement() after we hit the closing ">". The next
		// token after the ">" might be an attribute name with a dash in it
		// like this: "<Foo<T> data-disabled/>"
		p.skipTypeScriptTypeArguments(true /* isInsideJSXElement */)
	}

	// Parse attributes
	var previousStringWithBackslashLoc logger.Loc
	properties := []js_ast.Property{}
	if startTagOrNil.Data != nil {
	parseAttributes:
		for {
			switch p.lexer.Token {
			case js_lexer.TIdentifier:
				// Parse the key
				keyRange := p.lexer.Range()
				key := js_ast.Expr{Loc: keyRange.Loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(p.lexer.Identifier)}}
				p.lexer.NextInsideJSXElement()

				// Parse the value
				var value js_ast.Expr
				wasShorthand := false
				if p.lexer.Token != js_lexer.TEquals {
					// Implicitly true value
					wasShorthand = true
					value = js_ast.Expr{Loc: logger.Loc{Start: keyRange.Loc.Start + keyRange.Len}, Data: &js_ast.EBoolean{Value: true}}
				} else {
					// Use NextInsideJSXElement() not Next() so we can parse a JSX-style string literal
					p.lexer.NextInsideJSXElement()
					if p.lexer.Token == js_lexer.TStringLiteral {
						stringLoc := p.lexer.Loc()
						if p.lexer.PreviousBackslashQuoteInJSX.Loc.Start > stringLoc.Start {
							previousStringWithBackslashLoc = stringLoc
						}
						value = js_ast.Expr{Loc: stringLoc, Data: &js_ast.EString{Value: p.lexer.StringLiteral()}}
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
					Key:          key,
					ValueOrNil:   value,
					WasShorthand: wasShorthand,
				})

			case js_lexer.TOpenBrace:
				// Use Next() not ExpectInsideJSXElement() so we can parse "..."
				p.lexer.Next()
				p.lexer.Expect(js_lexer.TDotDotDot)
				value := p.parseExpr(js_ast.LComma)
				properties = append(properties, js_ast.Property{
					Kind:       js_ast.PropertySpread,
					ValueOrNil: value,
				})

				// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
				p.lexer.NextInsideJSXElement()

			default:
				break parseAttributes
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
			TagOrNil:   startTagOrNil,
			Properties: properties,
			CloseLoc:   closeLoc,
		}}
	}

	// Use ExpectJSXElementChild() so we parse child strings
	p.lexer.ExpectJSXElementChild(js_lexer.TGreaterThan)

	// Parse the children of this element
	children := []js_ast.Expr{}
	for {
		switch p.lexer.Token {
		case js_lexer.TStringLiteral:
			children = append(children, js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EString{Value: p.lexer.StringLiteral()}})
			p.lexer.NextJSXElementChild()

		case js_lexer.TOpenBrace:
			// Use Next() instead of NextJSXElementChild() here since the next token is an expression
			p.lexer.Next()

			// The "..." here is ignored (it's used to signal an array type in TypeScript)
			if p.lexer.Token == js_lexer.TDotDotDot && p.options.ts.Parse {
				p.lexer.Next()
			}

			// The expression is optional, and may be absent
			if p.lexer.Token != js_lexer.TCloseBrace {
				children = append(children, p.parseExpr(js_ast.LLowest))
			}

			// Use ExpectJSXElementChild() so we parse child strings
			p.lexer.ExpectJSXElementChild(js_lexer.TCloseBrace)

		case js_lexer.TLessThan:
			lessThanLoc := p.lexer.Loc()
			p.lexer.NextInsideJSXElement()

			if p.lexer.Token != js_lexer.TSlash {
				// This is a child element
				children = append(children, p.parseJSXElement(lessThanLoc))

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
				p.log.AddWithNotes(logger.Error, &p.tracker, endRange, fmt.Sprintf("Expected closing tag %q to match opening tag %q", endText, startText),
					[]logger.MsgData{p.tracker.MsgData(startRange, fmt.Sprintf("The opening tag %q is here:", startText))})
			}
			if p.lexer.Token != js_lexer.TGreaterThan {
				p.lexer.Expected(js_lexer.TGreaterThan)
			}

			return js_ast.Expr{Loc: loc, Data: &js_ast.EJSXElement{
				TagOrNil:   startTagOrNil,
				Properties: properties,
				Children:   children,
				CloseLoc:   lessThanLoc,
			}}

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

func (p *parser) parseAndDeclareDecls(kind js_ast.SymbolKind, opts parseStmtOpts) []js_ast.Decl {
	decls := []js_ast.Decl{}

	for {
		// Forbid "let let" and "const let" but not "var let"
		if (kind == js_ast.SymbolOther || kind == js_ast.SymbolConst) && p.lexer.IsContextualKeyword("let") {
			p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Cannot use \"let\" as an identifier here:")
		}

		var valueOrNil js_ast.Expr
		local := p.parseBinding()
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
		}

		decls = append(decls, js_ast.Decl{Binding: local, ValueOrNil: valueOrNil})

		if p.lexer.Token != js_lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	return decls
}

func (p *parser) requireInitializers(decls []js_ast.Decl) {
	for _, d := range decls {
		if d.ValueOrNil.Data == nil {
			if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
				r := js_lexer.RangeOfIdentifier(p.source, d.Binding.Loc)
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("The constant %q must be initialized",
					p.symbols[id.Ref.InnerIndex].OriginalName))
			} else {
				p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: d.Binding.Loc}, "This constant must be initialized")
			}
		}
	}
}

func (p *parser) forbidInitializers(decls []js_ast.Decl, loopType string, isVar bool) {
	if len(decls) > 1 {
		p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: decls[0].Binding.Loc},
			fmt.Sprintf("for-%s loops must have a single declaration", loopType))
	} else if len(decls) == 1 && decls[0].ValueOrNil.Data != nil {
		if isVar {
			if _, ok := decls[0].Binding.Data.(*js_ast.BIdentifier); ok {
				// This is a weird special case. Initializers are allowed in "var"
				// statements with identifier bindings.
				return
			}
		}
		p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: decls[0].ValueOrNil.Loc},
			fmt.Sprintf("for-%s loop variables cannot have an initializer", loopType))
	}
}

func (p *parser) parseClauseAlias(kind string) string {
	loc := p.lexer.Loc()

	// The alias may now be a string (see https://github.com/tc39/ecma262/pull/2154)
	if p.lexer.Token == js_lexer.TStringLiteral {
		r := p.source.RangeOfString(loc)
		alias, problem, ok := js_lexer.UTF16ToStringWithValidation(p.lexer.StringLiteral())
		if !ok {
			p.log.Add(logger.Error, &p.tracker, r,
				fmt.Sprintf("This %s alias is invalid because it contains the unpaired Unicode surrogate U+%X", kind, problem))
		} else {
			p.markSyntaxFeature(compat.ArbitraryModuleNamespaceNames, r)
		}
		return alias
	}

	// The alias may be a keyword
	if !p.lexer.IsIdentifierOrKeyword() {
		p.lexer.Expect(js_lexer.TIdentifier)
	}

	alias := p.lexer.Identifier
	p.checkForUnrepresentableIdentifier(loc, alias)
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
		name := js_ast.LocRef{Loc: aliasLoc, Ref: p.storeNameInRef(alias)}
		originalName := alias
		p.lexer.Next()

		// "import { type xx } from 'mod'"
		// "import { type xx as yy } from 'mod'"
		// "import { type 'xx' as yy } from 'mod'"
		// "import { type as } from 'mod'"
		// "import { type as as } from 'mod'"
		// "import { type as as as } from 'mod'"
		if p.options.ts.Parse && alias == "type" && p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				if p.lexer.IsContextualKeyword("as") {
					originalName = p.lexer.Identifier
					name = js_ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
					p.lexer.Next()

					if p.lexer.Token == js_lexer.TIdentifier {
						// "import { type as as as } from 'mod'"
						// "import { type as as foo } from 'mod'"
						p.lexer.Next()
					} else {
						// "import { type as as } from 'mod'"
						items = append(items, js_ast.ClauseItem{
							Alias:        alias,
							AliasLoc:     aliasLoc,
							Name:         name,
							OriginalName: originalName,
						})
					}
				} else if p.lexer.Token == js_lexer.TIdentifier {
					// "import { type as xxx } from 'mod'"
					originalName = p.lexer.Identifier
					name = js_ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
					p.lexer.Expect(js_lexer.TIdentifier)

					// Reject forbidden names
					if isEvalOrArguments(originalName) {
						r := js_lexer.RangeOfIdentifier(p.source, name.Loc)
						p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Cannot use %q as an identifier here:", originalName))
					}

					items = append(items, js_ast.ClauseItem{
						Alias:        alias,
						AliasLoc:     aliasLoc,
						Name:         name,
						OriginalName: originalName,
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
				name = js_ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
				p.lexer.Expect(js_lexer.TIdentifier)
			} else if !isIdentifier {
				// An import where the name is a keyword must have an alias
				p.lexer.ExpectedString("\"as\"")
			}

			// Reject forbidden names
			if isEvalOrArguments(originalName) {
				r := js_lexer.RangeOfIdentifier(p.source, name.Loc)
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Cannot use %q as an identifier here:", originalName))
			}

			items = append(items, js_ast.ClauseItem{
				Alias:        alias,
				AliasLoc:     aliasLoc,
				Name:         name,
				OriginalName: originalName,
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
		name := js_ast.LocRef{Loc: aliasLoc, Ref: p.storeNameInRef(alias)}
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

		if p.options.ts.Parse && alias == "type" && p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
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
							Alias:        alias,
							AliasLoc:     aliasLoc,
							Name:         name,
							OriginalName: originalName,
						})
					}
				} else if p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TCloseBrace {
					// "export { type as xxx }"
					// "export { type as 'xxx' }"
					alias = p.parseClauseAlias("export")
					aliasLoc = p.lexer.Loc()
					p.lexer.Next()

					items = append(items, js_ast.ClauseItem{
						Alias:        alias,
						AliasLoc:     aliasLoc,
						Name:         name,
						OriginalName: originalName,
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
				Alias:        alias,
				AliasLoc:     aliasLoc,
				Name:         name,
				OriginalName: originalName,
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
		p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Expected identifier but found %q", p.source.TextForRange(r)))
		panic(js_lexer.LexerPanic{})
	}

	return items, isSingleLine
}

func (p *parser) parseBinding() js_ast.Binding {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case js_lexer.TIdentifier:
		name := p.lexer.Identifier
		if (p.fnOrArrowDataParse.await != allowIdent && name == "await") ||
			(p.fnOrArrowDataParse.yield != allowIdent && name == "yield") {
			p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), fmt.Sprintf("Cannot use %q as an identifier here:", name))
		}
		ref := p.storeNameInRef(name)
		p.lexer.Next()
		return js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ref}}

	case js_lexer.TOpenBracket:
		p.markSyntaxFeature(compat.Destructuring, p.lexer.Range())
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []js_ast.ArrayBinding{}
		hasSpread := false

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != js_lexer.TCloseBracket {
			if p.lexer.Token == js_lexer.TComma {
				binding := js_ast.Binding{Loc: p.lexer.Loc(), Data: js_ast.BMissingShared}
				items = append(items, js_ast.ArrayBinding{Binding: binding})
			} else {
				if p.lexer.Token == js_lexer.TDotDotDot {
					p.lexer.Next()
					hasSpread = true

					// This was a bug in the ES2015 spec that was fixed in ES2016
					if p.lexer.Token != js_lexer.TIdentifier {
						p.markSyntaxFeature(compat.NestedRestBinding, p.lexer.Range())
					}
				}

				binding := p.parseBinding()

				var defaultValueOrNil js_ast.Expr
				if !hasSpread && p.lexer.Token == js_lexer.TEquals {
					p.lexer.Next()
					defaultValueOrNil = p.parseExpr(js_ast.LComma)
				}

				items = append(items, js_ast.ArrayBinding{Binding: binding, DefaultValueOrNil: defaultValueOrNil})

				// Commas after spread elements are not allowed
				if hasSpread && p.lexer.Token == js_lexer.TComma {
					p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Unexpected \",\" after rest pattern")
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
		p.lexer.Expect(js_lexer.TCloseBracket)
		return js_ast.Binding{Loc: loc, Data: &js_ast.BArray{
			Items:        items,
			HasSpread:    hasSpread,
			IsSingleLine: isSingleLine,
		}}

	case js_lexer.TOpenBrace:
		p.markSyntaxFeature(compat.Destructuring, p.lexer.Range())
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []js_ast.PropertyBinding{}

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != js_lexer.TCloseBrace {
			property := p.parsePropertyBinding()
			properties = append(properties, property)

			// Commas after spread elements are not allowed
			if property.IsSpread && p.lexer.Token == js_lexer.TComma {
				p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Unexpected \",\" after rest pattern")
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
		p.lexer.Expect(js_lexer.TCloseBrace)
		return js_ast.Binding{Loc: loc, Data: &js_ast.BObject{
			Properties:   properties,
			IsSingleLine: isSingleLine,
		}}
	}

	p.lexer.Expect(js_lexer.TIdentifier)
	return js_ast.Binding{}
}

func (p *parser) parseFn(name *js_ast.LocRef, data fnOrArrowDataParse) (fn js_ast.Fn, hadBody bool) {
	if data.await == allowExpr && data.yield == allowExpr {
		p.markSyntaxFeature(compat.AsyncGenerator, data.asyncRange)
	}

	fn.Name = name
	fn.HasRestArg = false
	fn.IsAsync = data.await == allowExpr
	fn.IsGenerator = data.yield == allowExpr
	fn.ArgumentsRef = js_ast.InvalidRef
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

		var tsDecorators []js_ast.Expr
		if data.allowTSDecorators {
			tsDecorators = p.parseTypeScriptDecorators()
		}

		if !fn.HasRestArg && p.lexer.Token == js_lexer.TDotDotDot {
			p.markSyntaxFeature(compat.RestArgument, p.lexer.Range())
			p.lexer.Next()
			fn.HasRestArg = true
		}

		isTypeScriptCtorField := false
		isIdentifier := p.lexer.Token == js_lexer.TIdentifier
		text := p.lexer.Identifier
		arg := p.parseBinding()

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
					text = p.lexer.Identifier

					// Re-parse the binding (the current binding is the TypeScript keyword)
					arg = p.parseBinding()
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

		p.declareBinding(js_ast.SymbolHoisted, arg, parseStmtOpts{})

		var defaultValueOrNil js_ast.Expr
		if !fn.HasRestArg && p.lexer.Token == js_lexer.TEquals {
			p.markSyntaxFeature(compat.DefaultArgument, p.lexer.Range())
			p.lexer.Next()
			defaultValueOrNil = p.parseExpr(js_ast.LComma)
		}

		fn.Args = append(fn.Args, js_ast.Arg{
			TSDecorators: tsDecorators,
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
		fn.ArgumentsRef = p.declareSymbol(js_ast.SymbolArguments, fn.OpenParenLoc, "arguments")
		p.symbols[fn.ArgumentsRef.InnerIndex].MustNotBeRenamed = true
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
			p.log.Add(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, fn.Name.Loc),
				"An async function cannot be named \"await\"")
		} else if fn.IsGenerator && p.symbols[fn.Name.Ref.InnerIndex].OriginalName == "yield" && kind == fnExpr {
			p.log.Add(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, fn.Name.Loc),
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
	var name *js_ast.LocRef
	classKeyword := p.lexer.Range()
	if p.lexer.Token == js_lexer.TClass {
		p.markSyntaxFeature(compat.Class, classKeyword)
		p.lexer.Next()
	} else {
		p.lexer.Expected(js_lexer.TClass)
	}

	if !opts.isNameOptional || (p.lexer.Token == js_lexer.TIdentifier && (!p.options.ts.Parse || p.lexer.Identifier != "implements")) {
		nameLoc := p.lexer.Loc()
		nameText := p.lexer.Identifier
		p.lexer.Expect(js_lexer.TIdentifier)
		if p.fnOrArrowDataParse.await != allowIdent && nameText == "await" {
			p.log.Add(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, nameLoc), "Cannot use \"await\" as an identifier here:")
		}
		name = &js_ast.LocRef{Loc: nameLoc, Ref: js_ast.InvalidRef}
		if !opts.isTypeScriptDeclare {
			name.Ref = p.declareSymbol(js_ast.SymbolClass, nameLoc, nameText)
		}
	}

	// Even anonymous classes can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters()
	}

	classOpts := parseClassOpts{
		allowTSDecorators:   true,
		isTypeScriptDeclare: opts.isTypeScriptDeclare,
	}
	if opts.tsDecorators != nil {
		classOpts.tsDecorators = opts.tsDecorators.values
	}
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeClassName, loc)
	class := p.parseClass(classKeyword, name, classOpts)

	if opts.isTypeScriptDeclare {
		p.popAndDiscardScope(scopeIndex)

		if opts.isNamespaceScope && opts.isExport {
			p.hasNonLocalExportDeclareInsideNamespace = true
		}

		return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
	}

	p.popScope()
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SClass{Class: class, IsExport: opts.isExport}}
}

type parseClassOpts struct {
	tsDecorators        []js_ast.Expr
	allowTSDecorators   bool
	isTypeScriptDeclare bool
}

// By the time we call this, the identifier and type parameters have already
// been parsed. We need to start parsing from the "extends" clause.
func (p *parser) parseClass(classKeyword logger.Range, name *js_ast.LocRef, classOpts parseClassOpts) js_ast.Class {
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
			p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
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

	// Allow "in" and private fields inside class bodies
	oldAllowIn := p.allowIn
	oldAllowPrivateIdentifiers := p.allowPrivateIdentifiers
	p.allowIn = true
	p.allowPrivateIdentifiers = true

	// A scope is needed for private identifiers
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeClassBody, bodyLoc)

	opts := propertyOpts{
		isClass:           true,
		allowTSDecorators: classOpts.allowTSDecorators,
		classHasExtends:   extendsOrNil.Data != nil,
	}
	hasConstructor := false

	for p.lexer.Token != js_lexer.TCloseBrace {
		if p.lexer.Token == js_lexer.TSemicolon {
			p.lexer.Next()
			continue
		}

		// Parse decorators for this property
		firstDecoratorLoc := p.lexer.Loc()
		if opts.allowTSDecorators {
			opts.tsDecorators = p.parseTypeScriptDecorators()
		} else {
			opts.tsDecorators = nil
		}

		// This property may turn out to be a type in TypeScript, which should be ignored
		if property, ok := p.parseProperty(js_ast.PropertyNormal, opts, nil); ok {
			properties = append(properties, property)

			// Forbid decorators on class constructors
			if key, ok := property.Key.Data.(*js_ast.EString); ok && js_lexer.UTF16EqualsString(key.Value, "constructor") {
				if len(opts.tsDecorators) > 0 {
					p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: firstDecoratorLoc},
						"TypeScript does not allow decorators on class constructors")
				}
				if property.IsMethod && !property.IsStatic && !property.IsComputed {
					if hasConstructor {
						p.log.Add(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, property.Key.Loc),
							"Classes cannot contain more than one constructor")
					}
					hasConstructor = true
				}
			}
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

	p.lexer.Expect(js_lexer.TCloseBrace)
	return js_ast.Class{
		ClassKeyword: classKeyword,
		TSDecorators: classOpts.tsDecorators,
		Name:         name,
		ExtendsOrNil: extendsOrNil,
		BodyLoc:      bodyLoc,
		Properties:   properties,
	}
}

func (p *parser) parseLabelName() *js_ast.LocRef {
	if p.lexer.Token != js_lexer.TIdentifier || p.lexer.HasNewlineBefore {
		return nil
	}

	name := js_ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(p.lexer.Identifier)}
	p.lexer.Next()
	return &name
}

func (p *parser) parsePath() (logger.Loc, string, *[]ast.AssertEntry) {
	pathLoc := p.lexer.Loc()
	pathText := js_lexer.UTF16ToString(p.lexer.StringLiteral())
	if p.lexer.Token == js_lexer.TNoSubstitutionTemplateLiteral {
		p.lexer.Next()
	} else {
		p.lexer.Expect(js_lexer.TStringLiteral)
	}

	// See https://github.com/tc39/proposal-import-assertions for more info
	var assertions *[]ast.AssertEntry
	if !p.lexer.HasNewlineBefore && p.lexer.IsContextualKeyword("assert") {
		// "import './foo.json' assert { type: 'json' }"
		var entries []ast.AssertEntry
		duplicates := make(map[string]logger.Range)
		p.lexer.Next()
		p.lexer.Expect(js_lexer.TOpenBrace)

		for p.lexer.Token != js_lexer.TCloseBrace {
			// Parse the key
			keyLoc := p.lexer.Loc()
			preferQuotedKey := false
			var key []uint16
			var keyText string
			if p.lexer.IsIdentifierOrKeyword() {
				keyText = p.lexer.Identifier
				key = js_lexer.StringToUTF16(keyText)
			} else if p.lexer.Token == js_lexer.TStringLiteral {
				key = p.lexer.StringLiteral()
				keyText = js_lexer.UTF16ToString(key)
				preferQuotedKey = !p.options.mangleSyntax
			} else {
				p.lexer.Expect(js_lexer.TIdentifier)
			}
			if prevRange, ok := duplicates[keyText]; ok {
				p.log.AddWithNotes(logger.Error, &p.tracker, p.lexer.Range(), fmt.Sprintf("Duplicate import assertion %q", keyText),
					[]logger.MsgData{p.tracker.MsgData(prevRange, fmt.Sprintf("The first %q was here:", keyText))})
			}
			duplicates[keyText] = p.lexer.Range()
			p.lexer.Next()
			p.lexer.Expect(js_lexer.TColon)

			// Parse the value
			valueLoc := p.lexer.Loc()
			value := p.lexer.StringLiteral()
			p.lexer.Expect(js_lexer.TStringLiteral)

			entries = append(entries, ast.AssertEntry{
				Key:             key,
				KeyLoc:          keyLoc,
				Value:           value,
				ValueLoc:        valueLoc,
				PreferQuotedKey: preferQuotedKey,
			})

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(js_lexer.TCloseBrace)
		assertions = &entries
	}

	return pathLoc, pathText, assertions
}

// This assumes the "function" token has already been parsed
func (p *parser) parseFnStmt(loc logger.Loc, opts parseStmtOpts, isAsync bool, asyncRange logger.Range) js_ast.Stmt {
	isGenerator := p.lexer.Token == js_lexer.TAsterisk
	if isGenerator {
		p.markSyntaxFeature(compat.Generator, p.lexer.Range())
		p.lexer.Next()
	} else if isAsync {
		p.markLoweredSyntaxFeature(compat.AsyncAwait, asyncRange, compat.Generator)
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

	var name *js_ast.LocRef
	var nameText string

	// The name is optional for "export default function() {}" pseudo-statements
	if !opts.isNameOptional || p.lexer.Token == js_lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		nameText = p.lexer.Identifier
		if !isAsync && p.fnOrArrowDataParse.await != allowIdent && nameText == "await" {
			p.log.Add(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, nameLoc), "Cannot use \"await\" as an identifier here:")
		}
		p.lexer.Expect(js_lexer.TIdentifier)
		name = &js_ast.LocRef{Loc: nameLoc, Ref: js_ast.InvalidRef}
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.options.ts.Parse {
		p.skipTypeScriptTypeParameters()
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

	fn, hadBody := p.parseFn(name, fnOrArrowDataParse{
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

		return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
	}

	p.popScope()

	// Only declare the function after we know if it had a body or not. Otherwise
	// TypeScript code such as this will double-declare the symbol:
	//
	//     function foo(): void;
	//     function foo(): void {}
	//
	if name != nil {
		kind := js_ast.SymbolHoistedFunction
		if isGenerator || isAsync {
			kind = js_ast.SymbolGeneratorOrAsyncFunction
		}
		name.Ref = p.declareSymbol(kind, name.Loc, nameText)
	}

	// Balance the fake block scope introduced above
	if hasIfScope {
		p.popScope()
	}

	fn.HasIfScope = hasIfScope
	p.validateFunctionName(fn, fnStmt)
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SFunction{Fn: fn, IsExport: opts.isExport}}
}

type deferredTSDecorators struct {
	values []js_ast.Expr

	// If this turns out to be a "declare class" statement, we need to undo the
	// scopes that were potentially pushed while parsing the decorator arguments.
	scopeIndex int
}

type lexicalDecl uint8

const (
	lexicalDeclForbid lexicalDecl = iota
	lexicalDeclAllowAll
	lexicalDeclAllowFnInsideIf
	lexicalDeclAllowFnInsideLabel
)

type parseStmtOpts struct {
	tsDecorators        *deferredTSDecorators
	lexicalDecl         lexicalDecl
	isModuleScope       bool
	isNamespaceScope    bool
	isExport            bool
	isNameOptional      bool // For "export default" pseudo-statements
	isTypeScriptDeclare bool
	isForLoopInit       bool
	isForAwaitLoopInit  bool
}

func (p *parser) parseStmt(opts parseStmtOpts) js_ast.Stmt {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case js_lexer.TSemicolon:
		p.lexer.Next()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SEmpty{}}

	case js_lexer.TExport:
		previousExportKeyword := p.es6ExportKeyword
		if opts.isModuleScope {
			p.es6ExportKeyword = p.lexer.Range()
		} else if !opts.isNamespaceScope {
			p.lexer.Unexpected()
		}
		p.lexer.Next()

		// TypeScript decorators only work on class declarations
		// "@decorator export class Foo {}"
		// "@decorator export abstract class Foo {}"
		// "@decorator export default class Foo {}"
		// "@decorator export default abstract class Foo {}"
		// "@decorator export declare class Foo {}"
		// "@decorator export declare abstract class Foo {}"
		if opts.tsDecorators != nil && p.lexer.Token != js_lexer.TClass && p.lexer.Token != js_lexer.TDefault &&
			!p.lexer.IsContextualKeyword("abstract") && !p.lexer.IsContextualKeyword("declare") {
			p.lexer.Expected(js_lexer.TClass)
		}

		switch p.lexer.Token {
		case js_lexer.TClass, js_lexer.TConst, js_lexer.TFunction, js_lexer.TVar:
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

			if opts.isTypeScriptDeclare && p.lexer.IsContextualKeyword("as") {
				// "export as namespace ns;"
				p.lexer.Next()
				p.lexer.ExpectContextualKeyword("namespace")
				p.lexer.Expect(js_lexer.TIdentifier)
				p.lexer.ExpectOrInsertSemicolon()
				return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
			}

			if p.lexer.IsContextualKeyword("async") {
				// "export async function foo() {}"
				asyncRange := p.lexer.Range()
				p.lexer.Next()
				if p.lexer.HasNewlineBefore {
					p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: logger.Loc{Start: asyncRange.End()}},
						"Unexpected newline after \"async\"")
					panic(js_lexer.LexerPanic{})
				}
				p.lexer.Expect(js_lexer.TFunction)
				opts.isExport = true
				return p.parseFnStmt(loc, opts, true /* isAsync */, asyncRange)
			}

			if p.options.ts.Parse {
				switch p.lexer.Identifier {
				case "type":
					// "export type foo = ..."
					typeRange := p.lexer.Range()
					p.lexer.Next()
					if p.lexer.HasNewlineBefore {
						p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: logger.Loc{Start: typeRange.End()}},
							"Unexpected newline after \"type\"")
						panic(js_lexer.LexerPanic{})
					}
					p.skipTypeScriptTypeStmt(parseStmtOpts{isModuleScope: opts.isModuleScope, isExport: true})
					return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}

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

			// The default name is lazily generated only if no other name is present
			createDefaultName := func() js_ast.LocRef {
				defaultName := js_ast.LocRef{Loc: defaultLoc, Ref: p.newSymbol(js_ast.SymbolOther, p.source.IdentifierName+"_default")}
				p.currentScope.Generated = append(p.currentScope.Generated, defaultName.Ref)
				return defaultName
			}

			// TypeScript decorators only work on class declarations
			// "@decorator export default class Foo {}"
			// "@decorator export default abstract class Foo {}"
			if opts.tsDecorators != nil && p.lexer.Token != js_lexer.TClass && !p.lexer.IsContextualKeyword("abstract") {
				p.lexer.Expected(js_lexer.TClass)
			}

			if p.lexer.IsContextualKeyword("async") {
				asyncRange := p.lexer.Range()
				p.lexer.Next()

				if p.lexer.Token == js_lexer.TFunction && !p.lexer.HasNewlineBefore {
					p.lexer.Next()
					stmt := p.parseFnStmt(loc, parseStmtOpts{
						isNameOptional: true,
						lexicalDecl:    lexicalDeclAllowAll,
					}, true /* isAsync */, asyncRange)
					if _, ok := stmt.Data.(*js_ast.STypeScript); ok {
						return stmt // This was just a type annotation
					}

					// Use the statement name if present, since it's a better name
					var defaultName js_ast.LocRef
					if s, ok := stmt.Data.(*js_ast.SFunction); ok && s.Fn.Name != nil {
						defaultName = js_ast.LocRef{Loc: defaultLoc, Ref: s.Fn.Name.Ref}
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

			if p.lexer.Token == js_lexer.TFunction || p.lexer.Token == js_lexer.TClass || p.lexer.IsContextualKeyword("interface") {
				stmt := p.parseStmt(parseStmtOpts{
					tsDecorators:   opts.tsDecorators,
					isNameOptional: true,
					lexicalDecl:    lexicalDeclAllowAll,
				})
				if _, ok := stmt.Data.(*js_ast.STypeScript); ok {
					return stmt // This was just a type annotation
				}

				// Use the statement name if present, since it's a better name
				var defaultName js_ast.LocRef
				switch s := stmt.Data.(type) {
				case *js_ast.SFunction:
					if s.Fn.Name != nil {
						defaultName = js_ast.LocRef{Loc: defaultLoc, Ref: s.Fn.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}
				case *js_ast.SClass:
					if s.Class.Name != nil {
						defaultName = js_ast.LocRef{Loc: defaultLoc, Ref: s.Class.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}
				default:
					panic("Internal error")
				}

				return js_ast.Stmt{Loc: loc, Data: &js_ast.SExportDefault{DefaultName: defaultName, Value: stmt}}
			}

			isIdentifier := p.lexer.Token == js_lexer.TIdentifier
			name := p.lexer.Identifier
			expr := p.parseExpr(js_ast.LComma)

			// Handle the default export of an abstract class in TypeScript
			if p.options.ts.Parse && isIdentifier && name == "abstract" {
				if _, ok := expr.Data.(*js_ast.EIdentifier); ok && (p.lexer.Token == js_lexer.TClass || opts.tsDecorators != nil) {
					stmt := p.parseClassStmt(loc, parseStmtOpts{
						tsDecorators:   opts.tsDecorators,
						isNameOptional: true,
					})

					// Use the statement name if present, since it's a better name
					var defaultName js_ast.LocRef
					if s, ok := stmt.Data.(*js_ast.SClass); ok && s.Class.Name != nil {
						defaultName = js_ast.LocRef{Loc: defaultLoc, Ref: s.Class.Name.Ref}
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
			var namespaceRef js_ast.Ref
			var alias *js_ast.ExportStarAlias
			var pathLoc logger.Loc
			var pathText string
			var assertions *[]ast.AssertEntry

			if p.lexer.IsContextualKeyword("as") {
				// "export * as ns from 'path'"
				p.lexer.Next()
				name := p.parseClauseAlias("export")
				namespaceRef = p.storeNameInRef(name)
				alias = &js_ast.ExportStarAlias{Loc: p.lexer.Loc(), OriginalName: name}
				p.lexer.Next()
				p.lexer.ExpectContextualKeyword("from")
				pathLoc, pathText, assertions = p.parsePath()
			} else {
				// "export * from 'path'"
				p.lexer.ExpectContextualKeyword("from")
				pathLoc, pathText, assertions = p.parsePath()
				name := js_ast.GenerateNonUniqueNameFromPath(pathText) + "_star"
				namespaceRef = p.storeNameInRef(name)
			}
			importRecordIndex := p.addImportRecord(ast.ImportStmt, pathLoc, pathText, assertions)

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
				pathLoc, pathText, assertions := p.parsePath()
				importRecordIndex := p.addImportRecord(ast.ImportStmt, pathLoc, pathText, assertions)
				name := "import_" + js_ast.GenerateNonUniqueNameFromPath(pathText)
				namespaceRef := p.storeNameInRef(name)
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
			p.es6ExportKeyword = previousExportKeyword // This wasn't an ESM export statement after all
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
		if p.options.ts.Parse {
			scopeIndex := len(p.scopesInOrder)
			tsDecorators := p.parseTypeScriptDecorators()

			// If this turns out to be a "declare class" statement, we need to undo the
			// scopes that were potentially pushed while parsing the decorator arguments.
			// That can look like any one of the following:
			//
			//   "@decorator declare class Foo {}"
			//   "@decorator declare abstract class Foo {}"
			//   "@decorator export declare class Foo {}"
			//   "@decorator export declare abstract class Foo {}"
			//
			opts.tsDecorators = &deferredTSDecorators{
				values:     tsDecorators,
				scopeIndex: scopeIndex,
			}

			// "@decorator class Foo {}"
			// "@decorator abstract class Foo {}"
			// "@decorator declare class Foo {}"
			// "@decorator declare abstract class Foo {}"
			// "@decorator export class Foo {}"
			// "@decorator export abstract class Foo {}"
			// "@decorator export declare class Foo {}"
			// "@decorator export declare abstract class Foo {}"
			// "@decorator export default class Foo {}"
			// "@decorator export default abstract class Foo {}"
			if p.lexer.Token != js_lexer.TClass && p.lexer.Token != js_lexer.TExport &&
				!p.lexer.IsContextualKeyword("abstract") && !p.lexer.IsContextualKeyword("declare") {
				p.lexer.Expected(js_lexer.TClass)
			}

			return p.parseStmt(opts)
		}

		p.lexer.Unexpected()
		return js_ast.Stmt{}

	case js_lexer.TClass:
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(loc)
		}
		return p.parseClassStmt(loc, opts)

	case js_lexer.TVar:
		p.lexer.Next()
		decls := p.parseAndDeclareDecls(js_ast.SymbolHoisted, opts)
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
		p.markSyntaxFeature(compat.Const, p.lexer.Range())
		p.lexer.Next()

		if p.options.ts.Parse && p.lexer.Token == js_lexer.TEnum {
			return p.parseTypeScriptEnumStmt(loc, opts)
		}

		decls := p.parseAndDeclareDecls(js_ast.SymbolConst, opts)
		p.lexer.ExpectOrInsertSemicolon()
		if !opts.isTypeScriptDeclare {
			p.requireInitializers(decls)
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
		yes := p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowFnInsideIf})
		var noOrNil js_ast.Stmt
		if p.lexer.Token == js_lexer.TElse {
			p.lexer.Next()
			noOrNil = p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowFnInsideIf})
		}
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SIf{Test: test, Yes: yes, NoOrNil: noOrNil}}

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
		body := p.parseStmt(parseStmtOpts{})
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SWhile{Test: test, Body: body}}

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
		body := p.parseStmt(parseStmtOpts{})
		p.popScope()

		return js_ast.Stmt{Loc: loc, Data: &js_ast.SWith{Value: test, BodyLoc: bodyLoc, Body: body}}

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

		for p.lexer.Token != js_lexer.TCloseBrace {
			var value js_ast.Expr
			body := []js_ast.Stmt{}

			if p.lexer.Token == js_lexer.TDefault {
				if foundDefault {
					p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "Multiple default clauses are not allowed")
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

		caseBody:
			for {
				switch p.lexer.Token {
				case js_lexer.TCloseBrace, js_lexer.TCase, js_lexer.TDefault:
					break caseBody

				default:
					body = append(body, p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowAll}))
				}
			}

			cases = append(cases, js_ast.Case{ValueOrNil: value, Body: body})
		}

		p.lexer.Expect(js_lexer.TCloseBrace)
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SSwitch{
			Test:    test,
			BodyLoc: bodyLoc,
			Cases:   cases,
		}}

	case js_lexer.TTry:
		p.lexer.Next()
		bodyLoc := p.lexer.Loc()
		p.lexer.Expect(js_lexer.TOpenBrace)
		p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
		body := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
		p.popScope()
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
					ref := p.newSymbol(js_ast.SymbolOther, "e")
					p.currentScope.Generated = append(p.currentScope.Generated, ref)
					bindingOrNil = js_ast.Binding{Loc: p.lexer.Loc(), Data: &js_ast.BIdentifier{Ref: ref}}
				}
			} else {
				p.lexer.Expect(js_lexer.TOpenParen)
				bindingOrNil = p.parseBinding()

				// Skip over types
				if p.options.ts.Parse && p.lexer.Token == js_lexer.TColon {
					p.lexer.Expect(js_lexer.TColon)
					p.skipTypeScriptType(js_ast.LLowest)
				}

				p.lexer.Expect(js_lexer.TCloseParen)

				// Bare identifiers are a special case
				kind := js_ast.SymbolOther
				if _, ok := bindingOrNil.Data.(*js_ast.BIdentifier); ok {
					kind = js_ast.SymbolCatchIdentifier
				}
				p.declareBinding(kind, bindingOrNil, parseStmtOpts{})
			}

			bodyLoc := p.lexer.Loc()
			p.lexer.Expect(js_lexer.TOpenBrace)

			p.pushScopeForParsePass(js_ast.ScopeBlock, bodyLoc)
			stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
			p.popScope()

			p.lexer.Next()
			catch = &js_ast.Catch{Loc: catchLoc, BindingOrNil: bindingOrNil, BodyLoc: bodyLoc, Body: stmts}
			p.popScope()
		}

		if p.lexer.Token == js_lexer.TFinally || catch == nil {
			finallyLoc := p.lexer.Loc()
			p.pushScopeForParsePass(js_ast.ScopeBlock, finallyLoc)
			p.lexer.Expect(js_lexer.TFinally)
			p.lexer.Expect(js_lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
			p.lexer.Next()
			finally = &js_ast.Finally{Loc: finallyLoc, Stmts: stmts}
			p.popScope()
		}

		return js_ast.Stmt{Loc: loc, Data: &js_ast.STry{
			BodyLoc: bodyLoc,
			Body:    body,
			Catch:   catch,
			Finally: finally,
		}}

	case js_lexer.TFor:
		p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()

		// "for await (let x of y) {}"
		isForAwait := p.lexer.IsContextualKeyword("await")
		if isForAwait {
			awaitRange := p.lexer.Range()
			if p.fnOrArrowDataParse.await != allowExpr {
				p.log.Add(logger.Error, &p.tracker, awaitRange, "Cannot use \"await\" outside an async function")
				isForAwait = false
			} else {
				didGenerateError := p.markSyntaxFeature(compat.ForAwait, awaitRange)
				if p.fnOrArrowDataParse.isTopLevel && !didGenerateError {
					p.topLevelAwaitKeyword = awaitRange
					p.markSyntaxFeature(compat.TopLevelAwait, awaitRange)
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
			decls = p.parseAndDeclareDecls(js_ast.SymbolHoisted, parseStmtOpts{})
			initOrNil = js_ast.Stmt{Loc: initLoc, Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: decls}}

		case js_lexer.TConst:
			p.markSyntaxFeature(compat.Const, p.lexer.Range())
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(js_ast.SymbolConst, parseStmtOpts{})
			initOrNil = js_ast.Stmt{Loc: initLoc, Data: &js_ast.SLocal{Kind: js_ast.LocalConst, Decls: decls}}

		case js_lexer.TSemicolon:

		default:
			var expr js_ast.Expr
			var stmt js_ast.Stmt
			expr, stmt, decls = p.parseExprOrLetStmt(parseStmtOpts{
				lexicalDecl:        lexicalDeclAllowAll,
				isForLoopInit:      true,
				isForAwaitLoopInit: isForAwait,
			})
			if stmt.Data != nil {
				badLetRange = logger.Range{}
				initOrNil = stmt
			} else {
				initOrNil = js_ast.Stmt{Loc: initLoc, Data: &js_ast.SExpr{Value: expr}}
			}
		}

		// "in" expressions are allowed again
		p.allowIn = true

		// Detect for-of loops
		if p.lexer.IsContextualKeyword("of") || isForAwait {
			if badLetRange.Len > 0 {
				p.log.Add(logger.Error, &p.tracker, badLetRange, "\"let\" must be wrapped in parentheses to be used as an expression here:")
			}
			if isForAwait && !p.lexer.IsContextualKeyword("of") {
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
			body := p.parseStmt(parseStmtOpts{})
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SForOf{IsAwait: isForAwait, Init: initOrNil, Value: value, Body: body}}
		}

		// Detect for-in loops
		if p.lexer.Token == js_lexer.TIn {
			p.forbidInitializers(decls, "in", isVar)
			p.lexer.Next()
			value := p.parseExpr(js_ast.LLowest)
			p.lexer.Expect(js_lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return js_ast.Stmt{Loc: loc, Data: &js_ast.SForIn{Init: initOrNil, Value: value, Body: body}}
		}

		// Only require "const" statement initializers when we know we're a normal for loop
		if local, ok := initOrNil.Data.(*js_ast.SLocal); ok && local.Kind == js_ast.LocalConst {
			p.requireInitializers(decls)
		}

		p.lexer.Expect(js_lexer.TSemicolon)

		if p.lexer.Token != js_lexer.TSemicolon {
			testOrNil = p.parseExpr(js_ast.LLowest)
		}

		p.lexer.Expect(js_lexer.TSemicolon)

		if p.lexer.Token != js_lexer.TCloseParen {
			updateOrNil = p.parseExpr(js_ast.LLowest)
		}

		p.lexer.Expect(js_lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SFor{
			InitOrNil:   initOrNil,
			TestOrNil:   testOrNil,
			UpdateOrNil: updateOrNil,
			Body:        body,
		}}

	case js_lexer.TImport:
		previousImportKeyword := p.es6ImportKeyword
		p.es6ImportKeyword = p.lexer.Range()
		p.lexer.Next()
		stmt := js_ast.SImport{}
		wasOriginallyBareImport := false

		// "export import foo = bar"
		// "import foo = bar" in a namespace
		if (opts.isExport || (opts.isNamespaceScope && !opts.isTypeScriptDeclare)) && p.lexer.Token != js_lexer.TIdentifier {
			p.lexer.Expected(js_lexer.TIdentifier)
		}

		switch p.lexer.Token {
		case js_lexer.TOpenParen, js_lexer.TDot:
			// "import('path')"
			// "import.meta"
			p.es6ImportKeyword = previousImportKeyword // This wasn't an ESM import statement after all
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
			stmt.DefaultName = &js_ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(defaultName)}
			p.lexer.Next()

			if p.options.ts.Parse {
				// Skip over type-only imports
				if defaultName == "type" {
					switch p.lexer.Token {
					case js_lexer.TIdentifier:
						if p.lexer.Identifier != "from" {
							defaultName = p.lexer.Identifier
							stmt.DefaultName.Loc = p.lexer.Loc()
							p.lexer.Next()
							if p.lexer.Token == js_lexer.TEquals {
								// "import type foo = require('bar');"
								// "import type foo = bar.baz;"
								opts.isTypeScriptDeclare = true
								return p.parseTypeScriptImportEqualsStmt(loc, opts, stmt.DefaultName.Loc, defaultName)
							} else {
								// "import type foo from 'bar';"
								p.lexer.ExpectContextualKeyword("from")
								p.parsePath()
								p.lexer.ExpectOrInsertSemicolon()
								return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
							}
						}

					case js_lexer.TAsterisk:
						// "import type * as foo from 'bar';"
						p.lexer.Next()
						p.lexer.ExpectContextualKeyword("as")
						p.lexer.Expect(js_lexer.TIdentifier)
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}

					case js_lexer.TOpenBrace:
						// "import type {foo} from 'bar';"
						p.parseImportClause()
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
					}
				}

				// Parse TypeScript import assignment statements
				if p.lexer.Token == js_lexer.TEquals || opts.isExport || (opts.isNamespaceScope && !opts.isTypeScriptDeclare) {
					p.es6ImportKeyword = previousImportKeyword // This wasn't an ESM import statement after all
					return p.parseTypeScriptImportEqualsStmt(loc, opts, stmt.DefaultName.Loc, defaultName)
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

		pathLoc, pathText, assertions := p.parsePath()
		stmt.ImportRecordIndex = p.addImportRecord(ast.ImportStmt, pathLoc, pathText, assertions)
		p.importRecords[stmt.ImportRecordIndex].WasOriginallyBareImport = wasOriginallyBareImport
		p.lexer.ExpectOrInsertSemicolon()

		if stmt.StarNameLoc != nil {
			name := p.loadNameFromRef(stmt.NamespaceRef)
			stmt.NamespaceRef = p.declareSymbol(js_ast.SymbolImport, *stmt.StarNameLoc, name)
		} else {
			// Generate a symbol for the namespace
			name := "import_" + js_ast.GenerateNonUniqueNameFromPath(pathText)
			stmt.NamespaceRef = p.newSymbol(js_ast.SymbolOther, name)
			p.currentScope.Generated = append(p.currentScope.Generated, stmt.NamespaceRef)
		}
		itemRefs := make(map[string]js_ast.LocRef)

		// Link the default item to the namespace
		if stmt.DefaultName != nil {
			name := p.loadNameFromRef(stmt.DefaultName.Ref)
			ref := p.declareSymbol(js_ast.SymbolImport, stmt.DefaultName.Loc, name)
			p.isImportItem[ref] = true
			stmt.DefaultName.Ref = ref
		}

		// Link each import item to the namespace
		if stmt.Items != nil {
			for i, item := range *stmt.Items {
				name := p.loadNameFromRef(item.Name.Ref)
				ref := p.declareSymbol(js_ast.SymbolImport, item.Name.Loc, name)
				p.checkForUnrepresentableIdentifier(item.AliasLoc, item.Alias)
				p.isImportItem[ref] = true
				(*stmt.Items)[i].Name.Ref = ref
				itemRefs[item.Alias] = js_ast.LocRef{Loc: item.Name.Loc, Ref: ref}
			}
		}

		// Track the items for this namespace
		p.importItemsForNamespace[stmt.NamespaceRef] = itemRefs

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
			p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), "A return statement cannot be used here:")
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
			p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: logger.Loc{Start: loc.Start + 5}},
				"Unexpected newline after \"throw\"")
			panic(js_lexer.LexerPanic{})
		}
		expr := p.parseExpr(js_ast.LLowest)
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SThrow{Value: expr}}

	case js_lexer.TDebugger:
		p.lexer.Next()
		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SDebugger{}}

	case js_lexer.TOpenBrace:
		p.pushScopeForParsePass(js_ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()
		stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
		p.lexer.Next()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SBlock{Stmts: stmts}}

	default:
		isIdentifier := p.lexer.Token == js_lexer.TIdentifier
		name := p.lexer.Identifier

		// Parse either an async function, an async expression, or a normal expression
		var expr js_ast.Expr
		if isIdentifier && p.lexer.Raw() == "async" {
			asyncRange := p.lexer.Range()
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TFunction && !p.lexer.HasNewlineBefore {
				p.lexer.Next()
				return p.parseFnStmt(asyncRange.Loc, opts, true /* isAsync */, asyncRange)
			}
			expr = p.parseSuffix(p.parseAsyncPrefixExpr(asyncRange, js_ast.LLowest, 0), js_ast.LLowest, nil, 0)
		} else {
			var stmt js_ast.Stmt
			expr, stmt, _ = p.parseExprOrLetStmt(opts)
			if stmt.Data != nil {
				p.lexer.ExpectOrInsertSemicolon()
				return stmt
			}
		}

		if isIdentifier {
			if ident, ok := expr.Data.(*js_ast.EIdentifier); ok {
				if p.lexer.Token == js_lexer.TColon && opts.tsDecorators == nil {
					p.pushScopeForParsePass(js_ast.ScopeLabel, loc)
					defer p.popScope()

					// Parse a labeled statement
					p.lexer.Next()
					name := js_ast.LocRef{Loc: expr.Loc, Ref: ident.Ref}
					nestedOpts := parseStmtOpts{}
					if opts.lexicalDecl == lexicalDeclAllowAll || opts.lexicalDecl == lexicalDeclAllowFnInsideLabel {
						nestedOpts.lexicalDecl = lexicalDeclAllowFnInsideLabel
					}
					stmt := p.parseStmt(nestedOpts)
					return js_ast.Stmt{Loc: loc, Data: &js_ast.SLabel{Name: name, Stmt: stmt}}
				}

				if p.options.ts.Parse {
					switch name {
					case "type":
						if p.lexer.Token == js_lexer.TIdentifier && !p.lexer.HasNewlineBefore {
							// "type Foo = any"
							p.skipTypeScriptTypeStmt(parseStmtOpts{isModuleScope: opts.isModuleScope})
							return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
						}

					case "namespace", "module":
						// "namespace Foo {}"
						// "module Foo {}"
						// "declare module 'fs' {}"
						// "declare module 'fs';"
						if (opts.isModuleScope || opts.isNamespaceScope) && (p.lexer.Token == js_lexer.TIdentifier ||
							(p.lexer.Token == js_lexer.TStringLiteral && opts.isTypeScriptDeclare)) {
							return p.parseTypeScriptNamespaceStmt(loc, opts)
						}

					case "interface":
						// "interface Foo {}"
						p.skipTypeScriptInterfaceStmt(parseStmtOpts{isModuleScope: opts.isModuleScope})
						return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}

					case "abstract":
						if p.lexer.Token == js_lexer.TClass || opts.tsDecorators != nil {
							return p.parseClassStmt(loc, opts)
						}

					case "global":
						// "declare module 'fs' { global { namespace NodeJS {} } }"
						if opts.isNamespaceScope && opts.isTypeScriptDeclare && p.lexer.Token == js_lexer.TOpenBrace {
							p.lexer.Next()
							p.parseStmtsUpTo(js_lexer.TCloseBrace, opts)
							p.lexer.Next()
							return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
						}

					case "declare":
						opts.lexicalDecl = lexicalDeclAllowAll
						opts.isTypeScriptDeclare = true

						// "@decorator declare class Foo {}"
						// "@decorator declare abstract class Foo {}"
						if opts.tsDecorators != nil && p.lexer.Token != js_lexer.TClass && !p.lexer.IsContextualKeyword("abstract") {
							p.lexer.Expected(js_lexer.TClass)
						}

						// "declare global { ... }"
						if p.lexer.IsContextualKeyword("global") {
							p.lexer.Next()
							p.lexer.Expect(js_lexer.TOpenBrace)
							p.parseStmtsUpTo(js_lexer.TCloseBrace, opts)
							p.lexer.Next()
							return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
						}

						// "declare const x: any"
						stmt := p.parseStmt(opts)
						if opts.tsDecorators != nil {
							p.discardScopesUpTo(opts.tsDecorators.scopeIndex)
						}

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
								for _, decl := range s.Decls {
									decls = extractDeclsForBinding(decl.Binding, decls)
								}
							}
							if len(decls) > 0 {
								return js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
									Kind:     js_ast.LocalVar,
									IsExport: true,
									Decls:    decls,
								}}
							}
						}

						return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
					}
				}
			}
		}

		p.lexer.ExpectOrInsertSemicolon()
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: expr}}
	}
}

func extractDeclsForBinding(binding js_ast.Binding, decls []js_ast.Decl) []js_ast.Decl {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		decls = append(decls, js_ast.Decl{Binding: binding})

	case *js_ast.BArray:
		for _, item := range b.Items {
			decls = extractDeclsForBinding(item.Binding, decls)
		}

	case *js_ast.BObject:
		for _, property := range b.Properties {
			decls = extractDeclsForBinding(property.Value, decls)
		}

	default:
		panic("Internal error")
	}

	return decls
}

func (p *parser) addImportRecord(kind ast.ImportKind, loc logger.Loc, text string, assertions *[]ast.AssertEntry) uint32 {
	index := uint32(len(p.importRecords))
	p.importRecords = append(p.importRecords, ast.ImportRecord{
		Kind:       kind,
		Range:      p.source.RangeOfString(loc),
		Path:       logger.Path{Text: text},
		Assertions: assertions,
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
	stmts := p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{})
	p.lexer.Next()

	p.allowIn = oldAllowIn
	p.fnOrArrowDataParse = oldFnOrArrowData
	return js_ast.FnBody{Loc: loc, Stmts: stmts}
}

func (p *parser) forbidLexicalDecl(loc logger.Loc) {
	r := js_lexer.RangeOfIdentifier(p.source, loc)
	p.log.Add(logger.Error, &p.tracker, r, "Cannot use a declaration in a single-statement context")
}

func (p *parser) parseStmtsUpTo(end js_lexer.T, opts parseStmtOpts) []js_ast.Stmt {
	stmts := []js_ast.Stmt{}
	returnWithoutSemicolonStart := int32(-1)
	opts.lexicalDecl = lexicalDeclAllowAll
	isDirectivePrologue := true

	for {
		// Preserve some statement-level comments
		comments := p.lexer.CommentsToPreserveBefore
		if len(comments) > 0 {
			for _, comment := range comments {
				stmts = append(stmts, js_ast.Stmt{
					Loc: comment.Loc,
					Data: &js_ast.SComment{
						Text:           comment.Text,
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

					if js_lexer.UTF16EqualsString(str.Value, "use strict") {
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
					} else if js_lexer.UTF16EqualsString(str.Value, "use asm") {
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
						p.log.Add(logger.Warning, &p.tracker, logger.Range{Loc: logger.Loc{Start: returnWithoutSemicolonStart + 6}},
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
)

func (p *parser) generateTempRef(declare generateTempRefArg, optionalName string) js_ast.Ref {
	scope := p.currentScope
	for !scope.Kind.StopsHoisting() {
		scope = scope.Parent
	}
	if optionalName == "" {
		optionalName = "_" + js_ast.DefaultNameMinifier.NumberToMinifiedName(p.tempRefCount)
		p.tempRefCount++
	}
	ref := p.newSymbol(js_ast.SymbolOther, optionalName)
	if declare == tempRefNeedsDeclare {
		p.tempRefsToDeclare = append(p.tempRefsToDeclare, tempRef{ref: ref})
	}
	scope.Generated = append(scope.Generated, ref)
	return ref
}

func (p *parser) generateTopLevelTempRef() js_ast.Ref {
	ref := p.newSymbol(js_ast.SymbolOther, "_"+js_ast.DefaultNameMinifier.NumberToMinifiedName(p.topLevelTempRefCount))
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
	ref               js_ast.Ref
	declareLoc        logger.Loc
	isInsideWithScope bool
}

func (p *parser) findSymbol(loc logger.Loc, name string) findSymbolResult {
	var ref js_ast.Ref
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
			p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Cannot access %q here:", name))
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
					cache = make(map[string]js_ast.Ref)
					tsNamespace.LazilyGeneratedProperyAccesses = cache
				}
				ref, ok = cache[name]
				if !ok {
					ref = p.newSymbol(js_ast.SymbolOther, name)
					p.symbols[ref.InnerIndex].NamespaceAlias = &js_ast.NamespaceAlias{
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
			ref = p.newSymbol(js_ast.SymbolUnbound, name)
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
		p.symbols[ref.InnerIndex].MustNotBeRenamed = true
	}

	// Track how many times we've referenced this symbol
	p.recordUsage(ref)
	return findSymbolResult{ref, declareLoc, isInsideWithScope}
}

func (p *parser) findLabelSymbol(loc logger.Loc, name string) (ref js_ast.Ref, isLoop bool, ok bool) {
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
	p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("There is no containing label named %q", name))

	// Allocate an "unbound" symbol
	ref = p.newSymbol(js_ast.SymbolUnbound, name)

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
//   function foo() {
//     if (false) { var x; }
//     x = 1;
//   }
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
	if len(p.tempRefsToDeclare) > 0 {
		decls := []js_ast.Decl{}
		for _, temp := range p.tempRefsToDeclare {
			decls = append(decls, js_ast.Decl{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: temp.ref}}, ValueOrNil: temp.valueOrNil})
			p.recordDeclaredSymbol(temp.ref)
		}

		// If the first statement is a super() call, make sure it stays that way
		stmt := js_ast.Stmt{Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: decls}}
		if len(stmts) > 0 && js_ast.IsSuperCall(stmts[0]) {
			stmts = append([]js_ast.Stmt{stmts[0], stmt}, stmts[1:]...)
		} else {
			stmts = append([]js_ast.Stmt{stmt}, stmts...)
		}
	}

	p.tempRefsToDeclare = oldTempRefs
	p.tempRefCount = oldTempRefCount
	return stmts
}

type stmtsKind uint8

const (
	stmtsNormal stmtsKind = iota
	stmtsLoopBody
	stmtsFnBody
)

func (p *parser) visitStmts(stmts []js_ast.Stmt, kind stmtsKind) []js_ast.Stmt {
	// Save the current control-flow liveness. This represents if we are
	// currently inside an "if (false) { ... }" block.
	oldIsControlFlowDead := p.isControlFlowDead

	// Visit all statements first
	visited := make([]js_ast.Stmt, 0, len(stmts))
	var before []js_ast.Stmt
	var after []js_ast.Stmt
	for _, stmt := range stmts {
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
			if !p.currentScope.Kind.StopsHoisting() && p.symbols[int(s.Fn.Name.Ref.InnerIndex)].Kind == js_ast.SymbolHoistedFunction {
				before = p.visitAndAppendStmt(before, stmt)
				continue
			}
		}
		visited = p.visitAndAppendStmt(visited, stmt)
	}

	// Transform block-level function declarations into variable declarations
	if len(before) > 0 {
		var letDecls []js_ast.Decl
		var varDecls []js_ast.Decl
		var nonFnStmts []js_ast.Stmt
		fnStmts := make(map[js_ast.Ref]int)
		for _, stmt := range before {
			s, ok := stmt.Data.(*js_ast.SFunction)
			if !ok {
				// We may get non-function statements here in certain scenarious such as when "KeepNames" is enabled
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
		kind := js_ast.LocalLet
		if p.options.unsupportedJSFeatures.Has(compat.Let) {
			kind = js_ast.LocalVar
		}
		before = append(before[:0], js_ast.Stmt{Loc: letDecls[0].ValueOrNil.Loc, Data: &js_ast.SLocal{Kind: kind, Decls: letDecls}})
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

	// Stop now if we're not mangling
	if !p.options.mangleSyntax {
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

func isDirectiveSupported(s *js_ast.SDirective) bool {
	// When minifying, strip all directives other than "use strict" since
	// that should be the only one that is ever really used by engines in
	// practice. We don't support "use asm" even though that's also
	// technically used in practice because the rest of our minifier would
	// likely cause asm.js code to fail validation anyway.
	return js_lexer.UTF16EqualsString(s.Value, "use strict")
}

func (p *parser) mangleStmts(stmts []js_ast.Stmt, kind stmtsKind) []js_ast.Stmt {
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
					// The variable must be initialized, since we will be substituting
					// the value into the usage.
					if last := prevS.Decls[len(prevS.Decls)-1]; last.ValueOrNil.Data != nil {
						// The binding must be an identifier that is only used once.
						// Ignore destructuring bindings since that's not the simple case.
						// Destructuring bindings could potentially execute side-effecting
						// code which would invalidate reordering.
						if id, ok := last.Binding.Data.(*js_ast.BIdentifier); ok {
							// Don't do this if "__name" was called on this symbol. In that
							// case there is actually more than one use even though it says
							// there is only one. The "__name" use isn't counted so that
							// tree shaking still works when names are kept.
							if symbol := p.symbols[id.Ref.InnerIndex]; symbol.UseCountEstimate == 1 && !symbol.DidKeepName {
								// Try to substitute the identifier with the initializer. This will
								// fail if something with side effects is in between the declaration
								// and the usage.
								if p.substituteSingleUseSymbolInStmt(stmt, id.Ref, last.ValueOrNil) {
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
				}

				// Substitution failed so stop trying
				break
			}
		}

		switch s := stmt.Data.(type) {
		case *js_ast.SEmpty:
			// Strip empty statements
			continue

		case *js_ast.SDirective:
			if !isDirectiveSupported(s) {
				continue
			}

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
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok && !js_ast.IsSuperCall(prevStmt) && !js_ast.IsSuperCall(stmt) {
					prevS.Value = js_ast.JoinWithComma(prevS.Value, s.Value)
					prevS.DoesNotAffectTreeShaking = prevS.DoesNotAffectTreeShaking && s.DoesNotAffectTreeShaking
					continue
				}
			}

		case *js_ast.SSwitch:
			// Absorb a previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok && !js_ast.IsSuperCall(prevStmt) {
					s.Test = js_ast.JoinWithComma(prevS.Value, s.Test)
					result = result[:len(result)-1]
				}
			}

		case *js_ast.SIf:
			// Absorb a previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok && !js_ast.IsSuperCall(prevStmt) {
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
							Test: js_ast.Not(s.Test),
							Yes:  stmtsToSingleStmt(bodyLoc, body),
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
					result = appendIfBodyPreservingScope(result, stmt)
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
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok && !js_ast.IsSuperCall(prevStmt) {
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
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok && !js_ast.IsSuperCall(prevStmt) {
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
				if prevS, ok := prevStmt.Data.(*js_ast.SExpr); ok && !js_ast.IsSuperCall(prevStmt) {
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
						if symbol.Link != js_ast.InvalidRef {
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

					// Do not absorb a "super()" call so that we keep it first
					if js_ast.IsSuperCall(prevStmt) {
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

					// Handle the returned values being the same
					if boolean, ok := checkEqualityIfNoSideEffects(left.Data, right.Data); ok && boolean {
						// "if (a) return b; return b;" => "return a, b;"
						lastReturn = &js_ast.SReturn{ValueOrNil: js_ast.JoinWithComma(prevS.Test, left)}
					} else {
						if comma, ok := prevS.Test.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
							// "if (a, b) return c; return d;" => "return a, b ? c : d;"
							lastReturn = &js_ast.SReturn{ValueOrNil: js_ast.JoinWithComma(comma.Left,
								p.mangleIfExpr(comma.Right.Loc, &js_ast.EIf{Test: comma.Right, Yes: left, No: right}))}
						} else {
							// "if (a) return b; return c;" => "return a ? b : c;"
							lastReturn = &js_ast.SReturn{ValueOrNil: p.mangleIfExpr(
								prevS.Test.Loc, &js_ast.EIf{Test: prevS.Test, Yes: left, No: right})}
						}
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
					// Do not absorb a "super()" call so that we keep it first
					if js_ast.IsSuperCall(prevStmt) {
						break throwLoop
					}

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
						lastThrow = &js_ast.SThrow{Value: js_ast.JoinWithComma(comma.Left, p.mangleIfExpr(comma.Right.Loc, &js_ast.EIf{Test: comma.Right, Yes: left, No: right}))}
					} else {
						// "if (a) return b; return c;" => "return a ? b : c;"
						lastThrow = &js_ast.SThrow{Value: p.mangleIfExpr(prevS.Test.Loc, &js_ast.EIf{Test: prevS.Test, Yes: left, No: right})}
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

func (p *parser) substituteSingleUseSymbolInStmt(stmt js_ast.Stmt, ref js_ast.Ref, replacement js_ast.Expr) bool {
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
		replacementCanBeRemoved := p.exprCanBeRemovedIfUnused(replacement)

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
	ref js_ast.Ref,
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
		if replacementCanBeRemoved && p.exprCanBeRemovedIfUnused(e.Expr) {
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
		} else if !p.exprCanBeRemovedIfUnused(e.Left) {
			// Do not reorder past a side effect
			return expr, substituteFailure
		}

		// Do not substitute our unconditionally-executed value into a branching
		// short-circuit operator unless the value itself has no side effects
		if replacementCanBeRemoved || !e.Op.IsShortCircuit() {
			if value, status := p.substituteSingleUseSymbolInExpr(e.Right, ref, replacement, replacementCanBeRemoved); status != substituteContinue {
				e.Right = value
				return expr, status
			}
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
			if property.IsComputed {
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

				// If we substituted a string, merge the string into the template
				if _, ok := value.Data.(*js_ast.EString); ok {
					expr = p.mangleTemplate(expr.Loc, e)
				}
				return expr, status
			}
		}
	}

	// If both the replacement and this expression have no observable side
	// effects, then we can reorder the replacement past this expression
	if replacementCanBeRemoved && p.exprCanBeRemovedIfUnused(expr) {
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

	return stmtsToSingleStmt(stmt.Loc, stmts)
}

// One statement could potentially expand to several statements
func stmtsToSingleStmt(loc logger.Loc, stmts []js_ast.Stmt) js_ast.Stmt {
	if len(stmts) == 0 {
		return js_ast.Stmt{Loc: loc, Data: &js_ast.SEmpty{}}
	}
	if len(stmts) == 1 {
		// "let" and "const" must be put in a block when in a single-statement context
		if s, ok := stmts[0].Data.(*js_ast.SLocal); !ok || s.Kind == js_ast.LocalVar {
			return stmts[0]
		}
	}
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SBlock{Stmts: stmts}}
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

func (p *parser) recordDeclaredSymbol(ref js_ast.Ref) {
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
				p.log.AddWithNotes(logger.Error, &p.tracker, r,
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
				wasAnonymousNamedExpr := p.isAnonymousNamedExpr(item.DefaultValueOrNil)
				item.DefaultValueOrNil = p.visitExpr(item.DefaultValueOrNil)

				// Optionally preserve the name
				if id, ok := item.Binding.Data.(*js_ast.BIdentifier); ok {
					item.DefaultValueOrNil = p.maybeKeepExprSymbolName(
						item.DefaultValueOrNil, p.symbols[id.Ref.InnerIndex].OriginalName, wasAnonymousNamedExpr)
				}
			}
		}

	case *js_ast.BObject:
		for i, property := range b.Properties {
			if !property.IsSpread {
				property.Key = p.visitExpr(property.Key)
			}
			p.visitBinding(property.Value, opts)
			if property.DefaultValueOrNil.Data != nil {
				wasAnonymousNamedExpr := p.isAnonymousNamedExpr(property.DefaultValueOrNil)
				property.DefaultValueOrNil = p.visitExpr(property.DefaultValueOrNil)

				// Optionally preserve the name
				if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok {
					property.DefaultValueOrNil = p.maybeKeepExprSymbolName(
						property.DefaultValueOrNil, p.symbols[id.Ref.InnerIndex].OriginalName, wasAnonymousNamedExpr)
				}
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
		*js_ast.SBreak, *js_ast.SContinue, *js_ast.SDirective:
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
	return js_ast.Stmt{Loc: body.Loc, Data: &js_ast.SEmpty{}}
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

func appendIfBodyPreservingScope(stmts []js_ast.Stmt, body js_ast.Stmt) []js_ast.Stmt {
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
	if boolean, sideEffects, ok := toBooleanWithSideEffects(s.Test.Data); ok {
		if boolean {
			// The test is truthy
			if s.NoOrNil.Data == nil || !shouldKeepStmtInDeadControlFlow(s.NoOrNil) {
				// We can drop the "no" branch
				if sideEffects == couldHaveSideEffects {
					// Keep the condition if it could have side effects (but is still known to be truthy)
					if test := p.simplifyUnusedExpr(s.Test); test.Data != nil {
						stmts = append(stmts, js_ast.Stmt{Loc: s.Test.Loc, Data: &js_ast.SExpr{Value: test}})
					}
				}
				return appendIfBodyPreservingScope(stmts, s.Yes)
			} else {
				// We have to keep the "no" branch
			}
		} else {
			// The test is falsy
			if !shouldKeepStmtInDeadControlFlow(s.Yes) {
				// We can drop the "yes" branch
				if sideEffects == couldHaveSideEffects {
					// Keep the condition if it could have side effects (but is still known to be falsy)
					if test := p.simplifyUnusedExpr(s.Test); test.Data != nil {
						stmts = append(stmts, js_ast.Stmt{Loc: s.Test.Loc, Data: &js_ast.SExpr{Value: test}})
					}
				}
				if s.NoOrNil.Data == nil {
					return stmts
				}
				return appendIfBodyPreservingScope(stmts, s.NoOrNil)
			} else {
				// We have to keep the "yes" branch
			}
		}
	}

	if yes, ok := s.Yes.Data.(*js_ast.SExpr); ok {
		// "yes" is an expression
		if s.NoOrNil.Data == nil {
			if not, ok := s.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
				// "if (!a) b();" => "a || b();"
				return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{
					Value: js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, not.Value, yes.Value)}})
			} else {
				// "if (a) b();" => "a && b();"
				return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{
					Value: js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, s.Test, yes.Value)}})
			}
		} else if no, ok := s.NoOrNil.Data.(*js_ast.SExpr); ok {
			// "if (a) b(); else c();" => "a ? b() : c();"
			return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: p.mangleIfExpr(loc, &js_ast.EIf{
				Test: s.Test,
				Yes:  yes.Value,
				No:   no.Value,
			})}})
		}
	} else if _, ok := s.Yes.Data.(*js_ast.SEmpty); ok {
		// "yes" is missing
		if s.NoOrNil.Data == nil {
			// "yes" and "no" are both missing
			if p.exprCanBeRemovedIfUnused(s.Test) {
				// "if (1) {}" => ""
				return stmts
			} else {
				// "if (a) {}" => "a;"
				return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: s.Test}})
			}
		} else if no, ok := s.NoOrNil.Data.(*js_ast.SExpr); ok {
			if not, ok := s.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
				// "if (!a) {} else b();" => "a && b();"
				return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{
					Value: js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, not.Value, no.Value)}})
			} else {
				// "if (a) {} else b();" => "a || b();"
				return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{
					Value: js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, s.Test, no.Value)}})
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

	return append(stmts, js_ast.Stmt{Loc: loc, Data: s})
}

func (p *parser) mangleIfExpr(loc logger.Loc, e *js_ast.EIf) js_ast.Expr {
	// "(a, b) ? c : d" => "a, b ? c : d"
	if comma, ok := e.Test.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
		return js_ast.JoinWithComma(comma.Left, p.mangleIfExpr(comma.Right.Loc, &js_ast.EIf{
			Test: comma.Right,
			Yes:  e.Yes,
			No:   e.No,
		}))
	}

	// "!a ? b : c" => "a ? c : b"
	if not, ok := e.Test.Data.(*js_ast.EUnary); ok && not.Op == js_ast.UnOpNot {
		e.Test = not.Value
		e.Yes, e.No = e.No, e.Yes
	}

	if valuesLookTheSame(e.Yes.Data, e.No.Data) {
		// "/* @__PURE__ */ a() ? b : b" => "b"
		if p.exprCanBeRemovedIfUnused(e.Test) {
			return e.Yes
		}

		// "a ? b : b" => "a, b"
		return js_ast.JoinWithComma(e.Test, e.Yes)
	}

	// "a ? true : false" => "!!a"
	// "a ? false : true" => "!a"
	if yes, ok := e.Yes.Data.(*js_ast.EBoolean); ok {
		if no, ok := e.No.Data.(*js_ast.EBoolean); ok {
			if yes.Value && !no.Value {
				return js_ast.Not(js_ast.Not(e.Test))
			}
			if !yes.Value && no.Value {
				return js_ast.Not(e.Test)
			}
		}
	}

	if id, ok := e.Test.Data.(*js_ast.EIdentifier); ok {
		// "a ? a : b" => "a || b"
		if id2, ok := e.Yes.Data.(*js_ast.EIdentifier); ok && id.Ref == id2.Ref {
			return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Test, e.No)
		}

		// "a ? b : a" => "a && b"
		if id2, ok := e.No.Data.(*js_ast.EIdentifier); ok && id.Ref == id2.Ref {
			return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Test, e.Yes)
		}
	}

	// "a ? b ? c : d : d" => "a && b ? c : d"
	if yesIf, ok := e.Yes.Data.(*js_ast.EIf); ok && valuesLookTheSame(yesIf.No.Data, e.No.Data) {
		e.Test = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Test, yesIf.Test)
		e.Yes = yesIf.Yes
		return js_ast.Expr{Loc: loc, Data: e}
	}

	// "a ? b : c ? b : d" => "a || c ? b : d"
	if noIf, ok := e.No.Data.(*js_ast.EIf); ok && valuesLookTheSame(e.Yes.Data, noIf.Yes.Data) {
		e.Test = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Test, noIf.Test)
		e.No = noIf.No
		return js_ast.Expr{Loc: loc, Data: e}
	}

	// "a ? c : (b, c)" => "(a || b), c"
	if comma, ok := e.No.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma && valuesLookTheSame(e.Yes.Data, comma.Right.Data) {
		return js_ast.JoinWithComma(
			js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Test, comma.Left),
			comma.Right,
		)
	}

	// "a ? (b, c) : c" => "(a && b), c"
	if comma, ok := e.Yes.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma && valuesLookTheSame(comma.Right.Data, e.No.Data) {
		return js_ast.JoinWithComma(
			js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Test, comma.Left),
			comma.Right,
		)
	}

	// "a ? b || c : c" => "(a && b) || c"
	if binary, ok := e.Yes.Data.(*js_ast.EBinary); ok && binary.Op == js_ast.BinOpLogicalOr &&
		valuesLookTheSame(binary.Right.Data, e.No.Data) {
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    js_ast.BinOpLogicalOr,
			Left:  js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Test, binary.Left),
			Right: binary.Right,
		}}
	}

	// "a ? c : b && c" => "(a || b) && c"
	if binary, ok := e.No.Data.(*js_ast.EBinary); ok && binary.Op == js_ast.BinOpLogicalAnd &&
		valuesLookTheSame(e.Yes.Data, binary.Right.Data) {
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    js_ast.BinOpLogicalAnd,
			Left:  js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Test, binary.Left),
			Right: binary.Right,
		}}
	}

	// "a ? b(c, d) : b(e, d)" => "b(a ? c : e, d)"
	if y, ok := e.Yes.Data.(*js_ast.ECall); ok && len(y.Args) > 0 {
		if n, ok := e.No.Data.(*js_ast.ECall); ok && len(n.Args) == len(y.Args) &&
			y.HasSameFlagsAs(n) && valuesLookTheSame(y.Target.Data, n.Target.Data) {
			// Only do this if the condition can be reordered past the call target
			// without side effects. For example, if the test or the call target is
			// an unbound identifier, reordering could potentially mean evaluating
			// the code could throw a different ReferenceError.
			if p.exprCanBeRemovedIfUnused(e.Test) && p.exprCanBeRemovedIfUnused(y.Target) {
				sameTailArgs := true
				for i, count := 1, len(y.Args); i < count; i++ {
					if !valuesLookTheSame(y.Args[i].Data, n.Args[i].Data) {
						sameTailArgs = false
						break
					}
				}
				if sameTailArgs {
					yesSpread, yesIsSpread := y.Args[0].Data.(*js_ast.ESpread)
					noSpread, noIsSpread := n.Args[0].Data.(*js_ast.ESpread)

					// "a ? b(...c) : b(...e)" => "b(...a ? c : e)"
					if yesIsSpread && noIsSpread {
						e.Yes = yesSpread.Value
						e.No = noSpread.Value
						y.Args[0] = js_ast.Expr{Loc: loc, Data: &js_ast.ESpread{Value: p.mangleIfExpr(loc, e)}}
						return js_ast.Expr{Loc: loc, Data: y}
					}

					// "a ? b(c) : b(e)" => "b(a ? c : e)"
					if !yesIsSpread && !noIsSpread {
						e.Yes = y.Args[0]
						e.No = n.Args[0]
						y.Args[0] = p.mangleIfExpr(loc, e)
						return js_ast.Expr{Loc: loc, Data: y}
					}
				}
			}
		}
	}

	// Try using the "??" operator, but only if it's supported
	if !p.options.unsupportedJSFeatures.Has(compat.NullishCoalescing) {
		if binary, ok := e.Test.Data.(*js_ast.EBinary); ok {
			switch binary.Op {
			case js_ast.BinOpLooseEq:
				// "a == null ? b : a" => "a ?? b"
				if _, ok := binary.Right.Data.(*js_ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Left) && valuesLookTheSame(binary.Left.Data, e.No.Data) {
					return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpNullishCoalescing, binary.Left, e.Yes)
				}

				// "null == a ? b : a" => "a ?? b"
				if _, ok := binary.Left.Data.(*js_ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Right) && valuesLookTheSame(binary.Right.Data, e.No.Data) {
					return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpNullishCoalescing, binary.Right, e.Yes)
				}

			case js_ast.BinOpLooseNe:
				// "a != null ? a : b" => "a ?? b"
				if _, ok := binary.Right.Data.(*js_ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Left) && valuesLookTheSame(binary.Left.Data, e.Yes.Data) {
					return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpNullishCoalescing, binary.Left, e.No)
				}

				// "null != a ? a : b" => "a ?? b"
				if _, ok := binary.Left.Data.(*js_ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Right) && valuesLookTheSame(binary.Right.Data, e.Yes.Data) {
					return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpNullishCoalescing, binary.Right, e.No)
				}
			}
		}
	}

	return js_ast.Expr{Loc: loc, Data: e}
}

func (p *parser) isAnonymousNamedExpr(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EArrow:
		return true
	case *js_ast.EFunction:
		return e.Fn.Name == nil
	case *js_ast.EClass:
		return e.Class.Name == nil
	}
	return false
}

func (p *parser) maybeKeepExprSymbolName(value js_ast.Expr, name string, wasAnonymousNamedExpr bool) js_ast.Expr {
	if p.options.keepNames && wasAnonymousNamedExpr {
		return p.keepExprSymbolName(value, name)
	}
	return value
}

func (p *parser) keepExprSymbolName(value js_ast.Expr, name string) js_ast.Expr {
	value = p.callRuntime(value.Loc, "__name", []js_ast.Expr{value,
		{Loc: value.Loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(name)}},
	})

	// Make sure tree shaking removes this if the function is never used
	value.Data.(*js_ast.ECall).CanBeUnwrappedIfUnused = true
	return value
}

func (p *parser) keepStmtSymbolName(loc logger.Loc, ref js_ast.Ref, name string) js_ast.Stmt {
	p.symbols[ref.InnerIndex].DidKeepName = true

	return js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{
		Value: p.callRuntime(loc, "__name", []js_ast.Expr{
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}},
			{Loc: loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(name)}},
		}),

		// Make sure tree shaking removes this if the function is never used
		DoesNotAffectTreeShaking: true,
	}}
}

func (p *parser) visitAndAppendStmt(stmts []js_ast.Stmt, stmt js_ast.Stmt) []js_ast.Stmt {
	switch s := stmt.Data.(type) {
	case *js_ast.SDebugger, *js_ast.SEmpty, *js_ast.SComment:
		// These don't contain anything to traverse

	case *js_ast.STypeScript:
		// Erase TypeScript constructs from the output completely
		return stmts

	case *js_ast.SDirective:
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

			if p.symbols[ref.InnerIndex].Kind == js_ast.SymbolUnbound {
				// Silently strip exports of non-local symbols in TypeScript, since
				// those likely correspond to type-only exports. But report exports of
				// non-local symbols as errors in JavaScript.
				if !p.options.ts.Parse {
					r := js_lexer.RangeOfIdentifier(p.source, item.Name.Loc)
					p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("%q is not declared in this file", name))
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
		s.NamespaceRef = p.newSymbol(js_ast.SymbolOther, name)
		p.currentScope.Generated = append(p.currentScope.Generated, s.NamespaceRef)
		p.recordDeclaredSymbol(s.NamespaceRef)

		// This is a re-export and the symbols created here are used to reference
		// names in another file. This means the symbols are really aliases.
		for i, item := range s.Items {
			name := p.loadNameFromRef(item.Name.Ref)
			ref := p.newSymbol(js_ast.SymbolOther, name)
			p.currentScope.Generated = append(p.currentScope.Generated, ref)
			p.recordDeclaredSymbol(ref)
			s.Items[i].Name.Ref = ref
		}

	case *js_ast.SExportStar:
		// "export * from 'path'"
		// "export * as ns from 'path'"
		name := p.loadNameFromRef(s.NamespaceRef)
		s.NamespaceRef = p.newSymbol(js_ast.SymbolOther, name)
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
							Name:         js_ast.LocRef{Loc: s.Alias.Loc, Ref: s.NamespaceRef},
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
			wasAnonymousNamedExpr := p.isAnonymousNamedExpr(s2.Value)
			s2.Value = p.visitExpr(s2.Value)

			// Optionally preserve the name
			s2.Value = p.maybeKeepExprSymbolName(s2.Value, "default", wasAnonymousNamedExpr)

			// Discard type-only export default statements
			if p.options.ts.Parse {
				if id, ok := s2.Value.Data.(*js_ast.EIdentifier); ok {
					symbol := p.symbols[id.Ref.InnerIndex]
					if symbol.Kind == js_ast.SymbolUnbound && p.localTypeNames[symbol.OriginalName] {
						return stmts
					}
				}
			}

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

			p.visitFn(&s2.Fn, s2.Fn.OpenParenLoc)
			stmts = append(stmts, stmt)

			// Optionally preserve the name
			if p.options.keepNames && s2.Fn.Name != nil {
				stmts = append(stmts, p.keepStmtSymbolName(s2.Fn.Name.Loc, s2.Fn.Name.Ref, name))
			}

			return stmts

		case *js_ast.SClass:
			shadowRef := p.visitClass(s.Value.Loc, &s2.Class)

			// Lower class field syntax for browsers that don't support it
			classStmts, _ := p.lowerClass(stmt, js_ast.Expr{}, shadowRef)
			return append(stmts, classStmts...)

		default:
			panic("Internal error")
		}

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
			p.log.Add(logger.Error, &p.tracker, r, "Cannot use \"break\" here:")
		}

	case *js_ast.SContinue:
		if s.Label != nil {
			name := p.loadNameFromRef(s.Label.Ref)
			var isLoop, ok bool
			s.Label.Ref, isLoop, ok = p.findLabelSymbol(s.Label.Loc, name)
			if ok && !isLoop {
				r := js_lexer.RangeOfIdentifier(p.source, s.Label.Loc)
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Cannot continue to label \"%s\"", name))
			}
		} else if !p.fnOrArrowDataVisit.isInsideLoop {
			r := js_lexer.RangeOfIdentifier(p.source, stmt.Loc)
			p.log.Add(logger.Error, &p.tracker, r, "Cannot use \"continue\" here:")
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
		ref := p.newSymbol(js_ast.SymbolLabel, name)
		s.Name.Ref = ref

		// Duplicate labels are an error
		for scope := p.currentScope.Parent; scope != nil; scope = scope.Parent {
			if scope.Label.Ref != js_ast.InvalidRef && name == p.symbols[scope.Label.Ref.InnerIndex].OriginalName {
				p.log.AddWithNotes(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, s.Name.Loc),
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

		p.currentScope.Label = js_ast.LocRef{Loc: s.Name.Loc, Ref: ref}
		switch s.Stmt.Data.(type) {
		case *js_ast.SFor, *js_ast.SForIn, *js_ast.SForOf, *js_ast.SWhile, *js_ast.SDoWhile:
			p.currentScope.LabelStmtIsLoop = true
		}
		s.Stmt = p.visitSingleStmt(s.Stmt, stmtsNormal)
		p.popScope()

	case *js_ast.SLocal:
		for i := range s.Decls {
			d := &s.Decls[i]
			p.visitBinding(d.Binding, bindingOpts{})
			if d.ValueOrNil.Data != nil {
				wasAnonymousNamedExpr := p.isAnonymousNamedExpr(d.ValueOrNil)
				d.ValueOrNil = p.visitExpr(d.ValueOrNil)

				// Optionally preserve the name
				if id, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
					d.ValueOrNil = p.maybeKeepExprSymbolName(
						d.ValueOrNil, p.symbols[id.Ref.InnerIndex].OriginalName, wasAnonymousNamedExpr)
				}

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
				if p.options.mangleSyntax && s.Kind == js_ast.LocalLet {
					if _, ok := d.Binding.Data.(*js_ast.BIdentifier); ok {
						if _, ok := d.ValueOrNil.Data.(*js_ast.EUndefined); ok {
							d.ValueOrNil = js_ast.Expr{}
						}
					}
				}
			}
		}

		// Handle being exported inside a namespace
		if s.IsExport && p.enclosingNamespaceArgRef != nil {
			wrapIdentifier := func(loc logger.Loc, ref js_ast.Ref) js_ast.Expr {
				p.recordUsage(*p.enclosingNamespaceArgRef)
				return js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
					Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					Name:    p.symbols[ref.InnerIndex].OriginalName,
					NameLoc: loc,
				}}
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
		p.stmtExprValue = s.Value.Data
		s.Value = p.visitExpr(s.Value)

		// Trim expressions without side effects
		if p.options.mangleSyntax {
			s.Value = p.simplifyUnusedExpr(s.Value)
			if s.Value.Data == nil {
				stmt = js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SEmpty{}}
			}
		}

	case *js_ast.SThrow:
		s.Value = p.visitExpr(s.Value)

	case *js_ast.SReturn:
		// Forbid top-level return inside modules with ECMAScript syntax
		if p.fnOrArrowDataVisit.isOutsideFnOrArrow {
			if p.hasESModuleSyntax {
				p.log.AddWithNotes(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, stmt.Loc),
					"Top-level return cannot be used inside an ECMAScript module", p.whyESModule())
			} else {
				p.hasTopLevelReturn = true
			}
		}

		if s.ValueOrNil.Data != nil {
			s.ValueOrNil = p.visitExpr(s.ValueOrNil)

			// Returning undefined is implicit except when inside an async generator
			// function, where "return undefined" behaves like "return await undefined"
			// but just "return" has no "await".
			if p.options.mangleSyntax && (!p.fnOrArrowDataVisit.isAsync || !p.fnOrArrowDataVisit.isGenerator) {
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

		if p.options.mangleSyntax {
			if len(s.Stmts) == 1 && !statementCaresAboutScope(s.Stmts[0]) {
				// Unwrap blocks containing a single statement
				stmt = s.Stmts[0]
			} else if len(s.Stmts) == 0 {
				// Trim empty blocks
				stmt = js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SEmpty{}}
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

		if p.options.mangleSyntax {
			s.Test = p.simplifyBooleanExpr(s.Test)

			// A true value is implied
			testOrNil := s.Test
			if boolean, sideEffects, ok := toBooleanWithSideEffects(s.Test.Data); ok && boolean && sideEffects == noSideEffects {
				testOrNil = js_ast.Expr{}
			}

			// "while (a) {}" => "for (;a;) {}"
			forS := &js_ast.SFor{TestOrNil: testOrNil, Body: s.Body}
			mangleFor(forS)
			stmt = js_ast.Stmt{Loc: stmt.Loc, Data: forS}
		}

	case *js_ast.SDoWhile:
		s.Body = p.visitLoopBody(s.Body)
		s.Test = p.visitExpr(s.Test)

		if p.options.mangleSyntax {
			s.Test = p.simplifyBooleanExpr(s.Test)
		}

	case *js_ast.SIf:
		s.Test = p.visitExpr(s.Test)

		if p.options.mangleSyntax {
			s.Test = p.simplifyBooleanExpr(s.Test)
		}

		// Fold constants
		boolean, _, ok := toBooleanWithSideEffects(s.Test.Data)

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
			if p.options.mangleSyntax {
				if _, ok := s.NoOrNil.Data.(*js_ast.SEmpty); ok {
					s.NoOrNil = js_ast.Stmt{}
				}
			}
		}

		if p.options.mangleSyntax {
			return p.mangleIf(stmts, stmt.Loc, s)
		}

	case *js_ast.SFor:
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		if s.InitOrNil.Data != nil {
			p.visitForLoopInit(s.InitOrNil, false)
		}

		if s.TestOrNil.Data != nil {
			s.TestOrNil = p.visitExpr(s.TestOrNil)

			if p.options.mangleSyntax {
				s.TestOrNil = p.simplifyBooleanExpr(s.TestOrNil)

				// A true value is implied
				if boolean, sideEffects, ok := toBooleanWithSideEffects(s.TestOrNil.Data); ok && boolean && sideEffects == noSideEffects {
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

		if p.options.mangleSyntax {
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

		p.popScope()

		p.lowerObjectRestInForLoopInit(s.Init, &s.Body)

	case *js_ast.STry:
		p.pushScopeForVisitPass(js_ast.ScopeBlock, stmt.Loc)
		p.fnOrArrowDataVisit.tryBodyCount++
		s.Body = p.visitStmts(s.Body, stmtsNormal)
		p.fnOrArrowDataVisit.tryBodyCount--
		p.popScope()

		if s.Catch != nil {
			p.pushScopeForVisitPass(js_ast.ScopeCatchBinding, s.Catch.Loc)
			if s.Catch.BindingOrNil.Data != nil {
				p.visitBinding(s.Catch.BindingOrNil, bindingOpts{})
			}

			p.pushScopeForVisitPass(js_ast.ScopeBlock, s.Catch.BodyLoc)
			s.Catch.Body = p.visitStmts(s.Catch.Body, stmtsNormal)
			p.popScope()

			p.lowerObjectRestInCatchBinding(s.Catch)
			p.popScope()
		}

		if s.Finally != nil {
			p.pushScopeForVisitPass(js_ast.ScopeBlock, s.Finally.Loc)
			s.Finally.Stmts = p.visitStmts(s.Finally.Stmts, stmtsNormal)
			p.popScope()
		}

	case *js_ast.SSwitch:
		s.Test = p.visitExpr(s.Test)
		p.pushScopeForVisitPass(js_ast.ScopeBlock, s.BodyLoc)
		oldIsInsideSwitch := p.fnOrArrowDataVisit.isInsideSwitch
		p.fnOrArrowDataVisit.isInsideSwitch = true
		for i, c := range s.Cases {
			if c.ValueOrNil.Data != nil {
				c.ValueOrNil = p.visitExpr(c.ValueOrNil)
				p.warnAboutEqualityCheck("case", c.ValueOrNil, c.ValueOrNil.Loc)
				p.warnAboutTypeofAndString(s.Test, c.ValueOrNil)
			}
			c.Body = p.visitStmts(c.Body, stmtsNormal)

			// Make sure the assignment to the body above is preserved
			s.Cases[i] = c
		}
		p.fnOrArrowDataVisit.isInsideSwitch = oldIsInsideSwitch
		p.popScope()

		// Check for duplicate case values
		p.duplicateCaseChecker.reset()
		for _, c := range s.Cases {
			if c.ValueOrNil.Data != nil {
				p.duplicateCaseChecker.check(p, c.ValueOrNil)
			}
		}

	case *js_ast.SFunction:
		p.visitFn(&s.Fn, s.Fn.OpenParenLoc)

		// Handle exporting this function from a namespace
		if s.IsExport && p.enclosingNamespaceArgRef != nil {
			s.IsExport = false
			stmts = append(stmts, stmt, js_ast.AssignStmt(
				js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EDot{
					Target:  js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					Name:    p.symbols[s.Fn.Name.Ref.InnerIndex].OriginalName,
					NameLoc: s.Fn.Name.Loc,
				}},
				js_ast.Expr{Loc: s.Fn.Name.Loc, Data: &js_ast.EIdentifier{Ref: s.Fn.Name.Ref}},
			))
		} else {
			stmts = append(stmts, stmt)
		}

		// Optionally preserve the name
		if p.options.keepNames {
			stmts = append(stmts, p.keepStmtSymbolName(s.Fn.Name.Loc, s.Fn.Name.Ref, p.symbols[s.Fn.Name.Ref.InnerIndex].OriginalName))
		}
		return stmts

	case *js_ast.SClass:
		shadowRef := p.visitClass(stmt.Loc, &s.Class)

		// Remove the export flag inside a namespace
		wasExportInsideNamespace := s.IsExport && p.enclosingNamespaceArgRef != nil
		if wasExportInsideNamespace {
			s.IsExport = false
		}

		// Lower class field syntax for browsers that don't support it
		classStmts, _ := p.lowerClass(stmt, js_ast.Expr{}, shadowRef)
		stmts = append(stmts, classStmts...)

		// Handle exporting this class from a namespace
		if wasExportInsideNamespace {
			stmts = append(stmts, js_ast.AssignStmt(
				js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EDot{
					Target:  js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					Name:    p.symbols[s.Class.Name.Ref.InnerIndex].OriginalName,
					NameLoc: s.Class.Name.Loc,
				}},
				js_ast.Expr{Loc: s.Class.Name.Loc, Data: &js_ast.EIdentifier{Ref: s.Class.Name.Ref}},
			))
		}

		return stmts

	case *js_ast.SEnum:
		p.recordDeclaredSymbol(s.Name.Ref)
		p.pushScopeForVisitPass(js_ast.ScopeEntry, stmt.Loc)
		p.recordDeclaredSymbol(s.Arg)

		// Scan ahead for any variables inside this namespace. This must be done
		// ahead of time before visiting any statements inside the namespace
		// because we may end up visiting the uses before the declarations.
		// We need to convert the uses into property accesses on the namespace.
		for _, value := range s.Values {
			if value.Ref != js_ast.InvalidRef {
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
		oldShouldFoldNumericConstants := p.shouldFoldNumericConstants
		p.shouldFoldNumericConstants = true

		// Create an assignment for each enum value
		for _, value := range s.Values {
			name := js_lexer.UTF16ToString(value.Name)
			var assignTarget js_ast.Expr
			hasStringValue := false

			if value.ValueOrNil.Data != nil {
				value.ValueOrNil = p.visitExpr(value.ValueOrNil)
				hasNumericValue = false

				switch e := value.ValueOrNil.Data.(type) {
				case *js_ast.ENumber:
					member := exportedMembers[name]
					member.Data = &js_ast.TSNamespaceMemberEnumNumber{Value: e.Value}
					exportedMembers[name] = member
					p.refToTSNamespaceMemberData[value.Ref] = member.Data
					hasNumericValue = true
					nextNumericValue = e.Value + 1

				case *js_ast.EString:
					member := exportedMembers[name]
					member.Data = &js_ast.TSNamespaceMemberEnumString{Value: e.Value}
					exportedMembers[name] = member
					p.refToTSNamespaceMemberData[value.Ref] = member.Data
					hasStringValue = true

				default:
					if !p.exprCanBeRemovedIfUnused(value.ValueOrNil) {
						allValuesArePure = false
					}
				}
			} else if hasNumericValue {
				member := exportedMembers[name]
				member.Data = &js_ast.TSNamespaceMemberEnumNumber{Value: nextNumericValue}
				exportedMembers[name] = member
				p.refToTSNamespaceMemberData[value.Ref] = member.Data
				value.ValueOrNil = js_ast.Expr{Loc: value.Loc, Data: &js_ast.ENumber{Value: nextNumericValue}}
				nextNumericValue++
			} else {
				value.ValueOrNil = js_ast.Expr{Loc: value.Loc, Data: js_ast.EUndefinedShared}
			}

			if p.options.mangleSyntax && js_lexer.IsIdentifier(name) {
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
		p.shouldFoldNumericConstants = oldShouldFoldNumericConstants

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
					p.markExportedDeclsInsideNamespace(s.Arg, local.Decls)
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
	wrapIdentifier := func(loc logger.Loc, ref js_ast.Ref) js_ast.Expr {
		p.relocatedTopLevelVars = append(p.relocatedTopLevelVars, js_ast.LocRef{Loc: loc, Ref: ref})
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

func (p *parser) markExprAsParenthesized(value js_ast.Expr) {
	switch e := value.Data.(type) {
	case *js_ast.EArray:
		e.IsParenthesized = true
	case *js_ast.EObject:
		e.IsParenthesized = true
	}
}

func (p *parser) markExportedDeclsInsideNamespace(nsRef js_ast.Ref, decls []js_ast.Decl) {
	for _, decl := range decls {
		p.markExportedBindingInsideNamespace(nsRef, decl.Binding)
	}
}

func (p *parser) markExportedBindingInsideNamespace(nsRef js_ast.Ref, binding js_ast.Binding) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		p.isExportedInsideNamespace[b.Ref] = nsRef

	case *js_ast.BArray:
		for _, item := range b.Items {
			p.markExportedBindingInsideNamespace(nsRef, item.Binding)
		}

	case *js_ast.BObject:
		for _, property := range b.Properties {
			p.markExportedBindingInsideNamespace(nsRef, property.Value)
		}

	default:
		panic("Internal error")
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
//   // "value" => "value + value"
//   // "value()" => "(_a = value(), _a + _a)"
//   valueFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, value)
//   return wrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
//     Op: js_ast.BinOpAdd,
//     Left: valueFunc(),
//     Right: valueFunc(),
//   }})
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
	tempRef := js_ast.InvalidRef

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
				if tempRef == js_ast.InvalidRef {
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
						Body:       js_ast.FnBody{Loc: loc, Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SReturn{ValueOrNil: expr}}}},
					}},
					Args: []js_ast.Expr{},
				}}
			}
	}

	return func() js_ast.Expr {
		if tempRef == js_ast.InvalidRef {
			tempRef = p.generateTempRef(tempRefNeedsDeclare, "")
			p.recordUsage(tempRef)
			return js_ast.Assign(js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}, value)
		}
		p.recordUsage(tempRef)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}
	}, wrapFunc
}

func (p *parser) visitTSDecorators(tsDecorators []js_ast.Expr) []js_ast.Expr {
	for i, decorator := range tsDecorators {
		tsDecorators[i] = p.visitExpr(decorator)
	}
	return tsDecorators
}

func (p *parser) visitClass(nameScopeLoc logger.Loc, class *js_ast.Class) js_ast.Ref {
	class.TSDecorators = p.visitTSDecorators(class.TSDecorators)

	if class.Name != nil {
		p.recordDeclaredSymbol(class.Name.Ref)
	}

	// Replace "this" with a reference to the class inside static field
	// initializers if static fields are being lowered, since that relocates the
	// field initializers outside of the class body and "this" will no longer
	// reference the same thing.
	classLoweringInfo := p.computeClassLoweringInfo(class)

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
				p.symbols[private.Ref.InnerIndex].PrivateSymbolMustBeLowered = true
			}
		}
	}

	// Conservatively lower all private names that have been used in a private
	// brand check anywhere in the file. See the comment on this map for details.
	if p.classPrivateBrandChecksToLower != nil {
		for _, prop := range class.Properties {
			if private, ok := prop.Key.Data.(*js_ast.EPrivateIdentifier); ok {
				if symbol := &p.symbols[private.Ref.InnerIndex]; p.classPrivateBrandChecksToLower[symbol.OriginalName] {
					symbol.PrivateSymbolMustBeLowered = true
				}
			}
		}
	}

	p.pushScopeForVisitPass(js_ast.ScopeClassName, nameScopeLoc)
	oldEnclosingClassKeyword := p.enclosingClassKeyword
	p.enclosingClassKeyword = class.ClassKeyword
	p.currentScope.RecursiveSetStrictMode(js_ast.ImplicitStrictModeClass)
	if class.Name != nil {
		p.validateDeclaredSymbolName(class.Name.Loc, p.symbols[class.Name.Ref.InnerIndex].OriginalName)
	}

	classNameRef := js_ast.InvalidRef
	if class.Name != nil {
		classNameRef = class.Name.Ref
	} else if classLoweringInfo.lowerAllStaticFields {
		// Generate a name if one doesn't already exist. This is necessary for
		// handling "this" in static class property initializers.
		classNameRef = p.newSymbol(js_ast.SymbolOther, "this")
	}

	// Insert a shadowing name that spans the whole class, which matches
	// JavaScript's semantics. The class body (and extends clause) "captures" the
	// original value of the name. This matters for class statements because the
	// symbol can be re-assigned to something else later. The captured values
	// must be the original value of the name, not the re-assigned value.
	shadowRef := js_ast.InvalidRef
	if classNameRef != js_ast.InvalidRef {
		// Use "const" for this symbol to match JavaScript run-time semantics. You
		// are not allowed to assign to this symbol (it throws a TypeError).
		name := p.symbols[classNameRef.InnerIndex].OriginalName
		shadowRef = p.newSymbol(js_ast.SymbolConst, "_"+name)
		p.recordDeclaredSymbol(shadowRef)
		if class.Name != nil {
			p.currentScope.Members[name] = js_ast.ScopeMember{Loc: class.Name.Loc, Ref: shadowRef}
		}
	}

	if class.ExtendsOrNil.Data != nil {
		class.ExtendsOrNil = p.visitExpr(class.ExtendsOrNil)
	}

	// A scope is needed for private identifiers
	p.pushScopeForVisitPass(js_ast.ScopeClassBody, class.BodyLoc)
	defer p.popScope()

	end := 0

	for i := range class.Properties {
		property := &class.Properties[i]

		if property.Kind == js_ast.PropertyClassStaticBlock {
			oldFnOrArrowData := p.fnOrArrowDataVisit
			oldFnOnlyDataVisit := p.fnOnlyDataVisit

			p.fnOrArrowDataVisit = fnOrArrowDataVisit{}
			p.fnOnlyDataVisit = fnOnlyDataVisit{
				isThisNested:       true,
				isNewTargetAllowed: true,
			}

			if classLoweringInfo.lowerAllStaticFields {
				// Replace "this" with the class name inside static class blocks
				p.fnOnlyDataVisit.thisClassStaticRef = &shadowRef

				// Need to lower "super" since it won't be valid outside the class body
				p.fnOrArrowDataVisit.shouldLowerSuper = true
			}

			p.pushScopeForVisitPass(js_ast.ScopeClassStaticInit, property.ClassStaticBlock.Loc)

			// Make it an error to use "arguments" in a static class block
			p.currentScope.ForbidArguments = true

			property.ClassStaticBlock.Stmts = p.visitStmts(property.ClassStaticBlock.Stmts, stmtsFnBody)
			p.popScope()

			p.fnOrArrowDataVisit = oldFnOrArrowData
			p.fnOnlyDataVisit = oldFnOnlyDataVisit

			// "class { static {} }" => "class {}"
			if p.options.mangleSyntax && len(property.ClassStaticBlock.Stmts) == 0 {
				continue
			}

			// Keep this property
			class.Properties[end] = *property
			end++
			continue
		}

		property.TSDecorators = p.visitTSDecorators(property.TSDecorators)
		private, isPrivate := property.Key.Data.(*js_ast.EPrivateIdentifier)

		// Special-case EPrivateIdentifier to allow it here
		if isPrivate {
			p.recordDeclaredSymbol(private.Ref)
		} else {
			key := p.visitExpr(property.Key)
			property.Key = key

			// "class {['x'] = y}" => "class {x = y}"
			if p.options.mangleSyntax && property.IsComputed {
				if str, ok := key.Data.(*js_ast.EString); ok && js_lexer.IsIdentifierUTF16(str.Value) {
					isInvalidConstructor := false
					if js_lexer.UTF16EqualsString(str.Value, "constructor") {
						if !property.IsMethod {
							// "constructor" is an invalid name for both instance and static fields
							isInvalidConstructor = true
						} else if !property.IsStatic {
							// Calling an instance method "constructor" is problematic so avoid that too
							isInvalidConstructor = true
						}
					}

					// A static property must not be called "prototype"
					isInvalidPrototype := property.IsStatic && js_lexer.UTF16EqualsString(str.Value, "prototype")

					if !isInvalidConstructor && !isInvalidPrototype {
						property.IsComputed = false
					}
				}
			}
		}

		// Make it an error to use "arguments" in a class body
		p.currentScope.ForbidArguments = true

		// The value of "this" and "super" is shadowed inside property values
		oldIsThisCaptured := p.fnOnlyDataVisit.isThisNested
		oldThis := p.fnOnlyDataVisit.thisClassStaticRef
		oldShouldLowerSuper := p.fnOrArrowDataVisit.shouldLowerSuper
		p.fnOnlyDataVisit.isThisNested = true
		p.fnOnlyDataVisit.isNewTargetAllowed = true
		p.fnOnlyDataVisit.thisClassStaticRef = nil
		p.fnOnlyDataVisit.superHelpers = nil

		// We need to explicitly assign the name to the property initializer if it
		// will be transformed such that it is no longer an inline initializer.
		nameToKeep := ""
		if isPrivate && p.privateSymbolNeedsToBeLowered(private) {
			nameToKeep = p.symbols[private.Ref.InnerIndex].OriginalName
		} else if !property.IsMethod && !property.IsComputed &&
			((!property.IsStatic && p.options.unsupportedJSFeatures.Has(compat.ClassField)) ||
				(property.IsStatic && p.options.unsupportedJSFeatures.Has(compat.ClassStaticField))) {
			if str, ok := property.Key.Data.(*js_ast.EString); ok {
				nameToKeep = js_lexer.UTF16ToString(str.Value)
			}
		}

		if property.ValueOrNil.Data != nil {
			if nameToKeep != "" {
				wasAnonymousNamedExpr := p.isAnonymousNamedExpr(property.ValueOrNil)
				property.ValueOrNil = p.maybeKeepExprSymbolName(p.visitExpr(property.ValueOrNil), nameToKeep, wasAnonymousNamedExpr)
			} else {
				property.ValueOrNil = p.visitExpr(property.ValueOrNil)
			}
		}

		if property.InitializerOrNil.Data != nil {
			if property.IsStatic && classLoweringInfo.lowerAllStaticFields {
				// Replace "this" with the class name inside static property initializers
				p.fnOnlyDataVisit.thisClassStaticRef = &shadowRef

				// Need to lower "super" since it won't be valid outside the class body
				p.fnOrArrowDataVisit.shouldLowerSuper = true
			}
			if nameToKeep != "" {
				wasAnonymousNamedExpr := p.isAnonymousNamedExpr(property.InitializerOrNil)
				property.InitializerOrNil = p.maybeKeepExprSymbolName(p.visitExpr(property.InitializerOrNil), nameToKeep, wasAnonymousNamedExpr)
			} else {
				property.InitializerOrNil = p.visitExpr(property.InitializerOrNil)
			}
		}

		// Restore "this" so it will take the inherited value in property keys
		p.fnOnlyDataVisit.thisClassStaticRef = oldThis
		p.fnOnlyDataVisit.isThisNested = oldIsThisCaptured
		p.fnOrArrowDataVisit.shouldLowerSuper = oldShouldLowerSuper

		// Restore the ability to use "arguments" in decorators and computed properties
		p.currentScope.ForbidArguments = false

		// Keep this property
		class.Properties[end] = *property
		end++
	}

	// Finish the filtering operation
	class.Properties = class.Properties[:end]

	p.enclosingClassKeyword = oldEnclosingClassKeyword
	p.popScope()

	if shadowRef != js_ast.InvalidRef {
		if p.symbols[shadowRef.InnerIndex].UseCountEstimate == 0 {
			// Don't generate a shadowing name if one isn't needed
			shadowRef = js_ast.InvalidRef
		} else if class.Name == nil {
			// If there was originally no class name but something inside needed one
			// (e.g. there was a static property initializer that referenced "this"),
			// store our generated name so the class expression ends up with a name.
			class.Name = &js_ast.LocRef{Loc: nameScopeLoc, Ref: classNameRef}
			p.currentScope.Generated = append(p.currentScope.Generated, classNameRef)
			p.recordDeclaredSymbol(classNameRef)
		}
	}

	return shadowRef
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
			if js_lexer.UTF16EqualsString(s.Value, "use strict") {
				return stmt.Loc, true
			}
		default:
			return logger.Loc{}, false
		}
	}
	return logger.Loc{}, false
}

type visitArgsOpts struct {
	body       []js_ast.Stmt
	hasRestArg bool

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
		p.log.Add(logger.Error, &p.tracker, p.source.RangeOfString(useStrictLoc),
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
		arg.TSDecorators = p.visitTSDecorators(arg.TSDecorators)
		p.visitBinding(arg.Binding, bindingOpts{
			duplicateArgCheck: duplicateArgCheck,
		})
		if arg.DefaultOrNil.Data != nil {
			arg.DefaultOrNil = p.visitExpr(arg.DefaultOrNil)
		}
	}
}

func (p *parser) isDotDefineMatch(expr js_ast.Expr, parts []string) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EDot:
		if len(parts) > 1 {
			// Intermediates must be dot expressions
			last := len(parts) - 1
			return parts[last] == e.Name && e.OptionalChain == js_ast.OptionalChainNone &&
				p.isDotDefineMatch(e.Target, parts[:last])
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

func (p *parser) jsxStringsToMemberExpression(loc logger.Loc, parts []string) js_ast.Expr {
	// Check both user-specified defines and known globals
	if defines, ok := p.options.defines.DotDefines[parts[len(parts)-1]]; ok {
	next:
		for _, define := range defines {
			if len(define.Parts) == len(parts) {
				for i := range parts {
					if parts[i] != define.Parts[i] {
						continue next
					}
				}
			}

			// Substitute user-specified defines
			if define.Data.DefineFunc != nil {
				return p.valueForDefine(loc, define.Data.DefineFunc, identifierOpts{})
			}
		}
	}

	// Generate an identifier for the first part
	result := p.findSymbol(loc, parts[0])
	value := p.handleIdentifier(loc, &js_ast.EIdentifier{
		Ref:                   result.ref,
		MustKeepDueToWithStmt: result.isInsideWithScope,

		// Enable tree shaking
		CanBeRemovedIfUnused: true,
	}, identifierOpts{
		wasOriginallyIdentifier: true,
	})

	// Build up a chain of property access expressions for subsequent parts
	for i := 1; i < len(parts); i++ {
		if expr, ok := p.maybeRewritePropertyAccess(loc, js_ast.AssignTargetNone, false, value, parts[i], loc, false, false); ok {
			value = expr
		} else {
			value = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
				Target:  value,
				Name:    parts[i],
				NameLoc: loc,

				// Enable tree shaking
				CanBeRemovedIfUnused: true,
			}}
		}
	}

	return value
}

func (p *parser) checkForUnrepresentableIdentifier(loc logger.Loc, name string) {
	if p.options.asciiOnly && p.options.unsupportedJSFeatures.Has(compat.UnicodeEscapes) &&
		js_lexer.ContainsNonBMPCodePoint(name) {
		if p.unrepresentableIdentifiers == nil {
			p.unrepresentableIdentifiers = make(map[string]bool)
		}
		if !p.unrepresentableIdentifiers[name] {
			p.unrepresentableIdentifiers[name] = true
			where, notes := p.prettyPrintTargetEnvironment(compat.UnicodeEscapes)
			r := js_lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddWithNotes(logger.Error, &p.tracker, r, fmt.Sprintf("%q cannot be escaped in %s but you "+
				"can set the charset to \"utf8\" to allow unescaped Unicode characters", name, where), notes)
		}
	}
}

func (p *parser) warnAboutTypeofAndString(a js_ast.Expr, b js_ast.Expr) {
	if typeof, ok := a.Data.(*js_ast.EUnary); ok && typeof.Op == js_ast.UnOpTypeof {
		if str, ok := b.Data.(*js_ast.EString); ok {
			value := js_lexer.UTF16ToString(str.Value)
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
				p.log.AddWithNotes(kind, &p.tracker, r, text, notes)
			}
		}
	}
}

func canChangeStrictToLoose(a js_ast.Expr, b js_ast.Expr) bool {
	return (js_ast.IsBooleanValue(a) && js_ast.IsBooleanValue(b)) ||
		(js_ast.IsNumericValue(a) && js_ast.IsNumericValue(b)) ||
		(js_ast.IsStringValue(a) && js_ast.IsStringValue(b))
}

func maybeSimplifyEqualityComparison(e *js_ast.EBinary, isNotEqual bool) (js_ast.Expr, bool) {
	// "!x === true" => "!x"
	// "!x === false" => "!!x"
	// "!x !== true" => "!!x"
	// "!x !== false" => "!x"
	if boolean, ok := e.Right.Data.(*js_ast.EBoolean); ok && js_ast.IsBooleanValue(e.Left) {
		if boolean.Value == isNotEqual {
			return js_ast.Not(e.Left), true
		} else {
			return e.Left, true
		}
	}

	return js_ast.Expr{}, false
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
			p.log.AddWithNotes(kind, &p.tracker, r, text,
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
			p.log.AddWithNotes(kind, &p.tracker, r, text,
				[]logger.MsgData{{Text: "Floating-point equality is defined such that NaN is never equal to anything, so \"x === NaN\" always returns false. " +
					"You need to use \"isNaN(x)\" instead to test for NaN."}})
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
			p.log.AddWithNotes(kind, &p.tracker, r, text,
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
				item, ok := importItems[name]
				if !ok {
					// Generate a new import item symbol in the module scope
					item = js_ast.LocRef{Loc: nameLoc, Ref: p.newSymbol(js_ast.SymbolImport, name)}
					p.moduleScope.Generated = append(p.moduleScope.Generated, item.Ref)

					// Link the namespace import and the import item together
					importItems[name] = item
					p.isImportItem[item.Ref] = true

					symbol := &p.symbols[item.Ref.InnerIndex]
					if p.options.mode == config.ModePassThrough {
						// Make sure the printer prints this as a property access
						symbol.NamespaceAlias = &js_ast.NamespaceAlias{
							NamespaceRef: id.Ref,
							Alias:        name,
						}
					} else {
						// Mark this as generated in case it's missing. We don't want to
						// generate errors for missing import items that are automatically
						// generated.
						symbol.ImportItemStatus = js_ast.ImportItemGenerated
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
	if !isCallTarget && p.options.mangleSyntax && assignTarget == js_ast.AssignTargetNone {
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
				if prop.Kind == js_ast.PropertySpread || prop.IsComputed || prop.IsMethod {
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
				if js_lexer.UTF16EqualsString(key.Value, "__proto__") {
					if _, ok := prop.ValueOrNil.Data.(*js_ast.ENull); ok {
						// Replacing "{__proto__: null}.a" with undefined should be safe
						hasProtoNull = true
					}
				}

				// This entire object literal must have no side effects
				if !p.exprCanBeRemovedIfUnused(prop.ValueOrNil) {
					isUnsafe = true
					break
				}

				// Note that we need to take the last value if there are duplicate keys
				if js_lexer.UTF16EqualsString(key.Value, name) {
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
					if preferQuotedKey || !js_lexer.IsIdentifier(name) {
						p.tsNamespaceTarget = &js_ast.EIndex{
							Target: target,
							Index:  js_ast.Expr{Loc: nameLoc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(name)}},
						}
					} else {
						p.tsNamespaceTarget = &js_ast.EDot{
							Target:  target,
							Name:    name,
							NameLoc: nameLoc,
						}
					}
					p.tsNamespaceMemberData = member.Data
					return js_ast.Expr{Loc: loc, Data: p.tsNamespaceTarget}, true
				}
			}
		}
	}

	return js_ast.Expr{}, false
}

func joinStrings(a []uint16, b []uint16) []uint16 {
	data := make([]uint16, len(a)+len(b))
	copy(data[:len(a)], a)
	copy(data[len(a):], b)
	return data
}

func foldStringAddition(left js_ast.Expr, right js_ast.Expr) *js_ast.Expr {
	switch l := left.Data.(type) {
	case *js_ast.EString:
		switch r := right.Data.(type) {
		case *js_ast.EString:
			return &js_ast.Expr{Loc: left.Loc, Data: &js_ast.EString{
				Value:          joinStrings(l.Value, r.Value),
				PreferTemplate: l.PreferTemplate || r.PreferTemplate,
			}}

		case *js_ast.ETemplate:
			if r.TagOrNil.Data == nil {
				return &js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
					HeadLoc:    left.Loc,
					HeadCooked: joinStrings(l.Value, r.HeadCooked),
					Parts:      r.Parts,
				}}
			}
		}

	case *js_ast.ETemplate:
		if l.TagOrNil.Data == nil {
			switch r := right.Data.(type) {
			case *js_ast.EString:
				n := len(l.Parts)
				head := l.HeadCooked
				parts := make([]js_ast.TemplatePart, n)
				if n == 0 {
					head = joinStrings(head, r.Value)
				} else {
					copy(parts, l.Parts)
					parts[n-1].TailCooked = joinStrings(parts[n-1].TailCooked, r.Value)
				}
				return &js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
					HeadLoc:    l.HeadLoc,
					HeadCooked: head,
					Parts:      parts,
				}}

			case *js_ast.ETemplate:
				if r.TagOrNil.Data == nil {
					n := len(l.Parts)
					head := l.HeadCooked
					parts := make([]js_ast.TemplatePart, n+len(r.Parts))
					copy(parts[n:], r.Parts)
					if n == 0 {
						head = joinStrings(head, r.HeadCooked)
					} else {
						copy(parts[:n], l.Parts)
						parts[n-1].TailCooked = joinStrings(parts[n-1].TailCooked, r.HeadCooked)
					}
					return &js_ast.Expr{Loc: left.Loc, Data: &js_ast.ETemplate{
						HeadLoc:    l.HeadLoc,
						HeadCooked: head,
						Parts:      parts,
					}}
				}
			}
		}
	}

	return nil
}

// Simplify syntax when we know it's used inside a boolean context
func (p *parser) simplifyBooleanExpr(expr js_ast.Expr) js_ast.Expr {
	switch e := expr.Data.(type) {
	case *js_ast.EUnary:
		if e.Op == js_ast.UnOpNot {
			// "!!a" => "a"
			if e2, ok2 := e.Value.Data.(*js_ast.EUnary); ok2 && e2.Op == js_ast.UnOpNot {
				return p.simplifyBooleanExpr(e2.Value)
			}

			e.Value = p.simplifyBooleanExpr(e.Value)
		}

	case *js_ast.EBinary:
		switch e.Op {
		case js_ast.BinOpLogicalAnd:
			if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Right.Data); ok && boolean && sideEffects == noSideEffects {
				// "if (anything && truthyNoSideEffects)" => "if (anything)"
				return e.Left
			}

		case js_ast.BinOpLogicalOr:
			if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Right.Data); ok && !boolean && sideEffects == noSideEffects {
				// "if (anything || falsyNoSideEffects)" => "if (anything)"
				return e.Left
			}
		}
	}

	return expr
}

func toInt32(f float64) int32 {
	// The easy way
	i := int32(f)
	if float64(i) == f {
		return i
	}

	// The hard way
	i = int32(uint32(math.Mod(math.Abs(f), 4294967296)))
	if math.Signbit(f) {
		return -i
	}
	return i
}

func toUint32(f float64) uint32 {
	return uint32(toInt32(f))
}

type exprIn struct {
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

	// Certain substitutions of identifiers are disallowed for assignment targets.
	// For example, we shouldn't transform "undefined = 1" into "void 0 = 1". This
	// isn't something real-world code would do but it matters for conformance
	// tests.
	assignTarget js_ast.AssignTarget
}

type exprOut struct {
	// True if the child node is an optional chain node (EDot, EIndex, or ECall
	// with an IsOptionalChain value of true)
	childContainsOptionalChain bool

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
}

func (p *parser) visitExpr(expr js_ast.Expr) js_ast.Expr {
	expr, _ = p.visitExprInOut(expr, exprIn{})
	return expr
}

func (p *parser) valueForThis(
	loc logger.Loc,
	shouldWarn bool,
	assignTarget js_ast.AssignTarget,
	isCallTarget bool,
	isDeleteTarget bool,
) (js_ast.Expr, bool) {
	// Substitute "this" if we're inside a static class property initializer
	if p.fnOnlyDataVisit.thisClassStaticRef != nil {
		p.recordUsage(*p.fnOnlyDataVisit.thisClassStaticRef)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: *p.fnOnlyDataVisit.thisClassStaticRef}}, true
	}

	// Is this a top-level use of "this"?
	if !p.fnOnlyDataVisit.isThisNested {
		// Substitute user-specified defines
		if data, ok := p.options.defines.IdentifierDefines["this"]; ok {
			if data.DefineFunc != nil {
				return p.valueForDefine(loc, data.DefineFunc, identifierOpts{
					assignTarget:   assignTarget,
					isCallTarget:   isCallTarget,
					isDeleteTarget: isDeleteTarget,
				}), true
			}
		}

		// Otherwise, replace top-level "this" with either "undefined" or "exports"
		if p.options.mode != config.ModePassThrough {
			if p.hasESModuleSyntax {
				// Warn about "this" becoming undefined, but only once per file
				if shouldWarn && !p.warnedThisIsUndefined {
					p.warnedThisIsUndefined = true

					// Show the warning as a debug message if we're in "node_modules"
					kind := logger.Warning
					if p.suppressWarningsAboutWeirdCode {
						kind = logger.Debug
					}
					data := p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, loc),
						"Top-level \"this\" will be replaced with undefined since this file is an ECMAScript module")
					data.Location.Suggestion = "undefined"
					p.log.AddMsg(logger.Msg{Kind: kind, Data: data, Notes: p.whyESModule()})
				}

				// In an ES6 module, "this" is supposed to be undefined. Instead of
				// doing this at runtime using "fn.call(undefined)", we do it at
				// compile time using expression substitution here.
				return js_ast.Expr{Loc: loc, Data: js_ast.EUndefinedShared}, true
			} else {
				// In a CommonJS module, "this" is supposed to be the same as "exports".
				// Instead of doing this at runtime using "fn.call(module.exports)", we
				// do it at compile time using expression substitution here.
				p.recordUsage(p.exportsRef)
				return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.exportsRef}}, true
			}
		}
	}

	return js_ast.Expr{}, false
}

func isBinaryNullAndUndefined(left js_ast.Expr, right js_ast.Expr, op js_ast.OpCode) (js_ast.Expr, js_ast.Expr, bool) {
	if a, ok := left.Data.(*js_ast.EBinary); ok && a.Op == op {
		if b, ok := right.Data.(*js_ast.EBinary); ok && b.Op == op {
			if idA, ok := a.Left.Data.(*js_ast.EIdentifier); ok {
				if idB, ok := b.Left.Data.(*js_ast.EIdentifier); ok && idA.Ref == idB.Ref {
					// "a === null || a === void 0"
					if _, ok := a.Right.Data.(*js_ast.ENull); ok {
						if _, ok := b.Right.Data.(*js_ast.EUndefined); ok {
							return a.Left, a.Right, true
						}
					}

					// "a === void 0 || a === null"
					if _, ok := a.Right.Data.(*js_ast.EUndefined); ok {
						if _, ok := b.Right.Data.(*js_ast.ENull); ok {
							return b.Left, b.Right, true
						}
					}
				}
			}
		}
	}

	return js_ast.Expr{}, js_ast.Expr{}, false
}

func inlineSpreadsOfArrayLiterals(values []js_ast.Expr) (results []js_ast.Expr) {
	for _, value := range values {
		if spread, ok := value.Data.(*js_ast.ESpread); ok {
			if array, ok := spread.Value.Data.(*js_ast.EArray); ok {
				for _, item := range array.Items {
					if _, ok := item.Data.(*js_ast.EMissing); ok {
						results = append(results, js_ast.Expr{Loc: item.Loc, Data: js_ast.EUndefinedShared})
					} else {
						results = append(results, item)
					}
				}
				continue
			}
		}
		results = append(results, value)
	}
	return
}

func locAfterOp(e *js_ast.EBinary) logger.Loc {
	if e.Left.Loc.Start < e.Right.Loc.Start {
		return e.Right.Loc
	} else {
		// Handle the case when we have transposed the operands
		return e.Left.Loc
	}
}

func canBeDeleted(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EIdentifier, *js_ast.EDot, *js_ast.EIndex:
		return true
	case *js_ast.ENumber:
		return math.IsInf(e.Value, 1) || math.IsNaN(e.Value)
	}
	return false
}

// This function exists to tie all of these checks together in one place
func isEvalOrArguments(name string) bool {
	return name == "eval" || name == "arguments"
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

// "`a${'b'}c`" => "`abc`"
func (p *parser) mangleTemplate(loc logger.Loc, e *js_ast.ETemplate) js_ast.Expr {
	// Can't inline strings if there's a custom template tag
	if e.TagOrNil.Data == nil {
		end := 0
		for _, part := range e.Parts {
			if str, ok := part.Value.Data.(*js_ast.EString); ok {
				if end == 0 {
					e.HeadCooked = append(append(e.HeadCooked, str.Value...), part.TailCooked...)
				} else {
					prevPart := &e.Parts[end-1]
					prevPart.TailCooked = append(append(prevPart.TailCooked, str.Value...), part.TailCooked...)
				}
			} else {
				e.Parts[end] = part
				end++
			}
		}
		e.Parts = e.Parts[:end]

		// Become a plain string if there are no substitutions
		if len(e.Parts) == 0 {
			return js_ast.Expr{Loc: loc, Data: &js_ast.EString{
				Value:          e.HeadCooked,
				PreferTemplate: true,
			}}
		}
	}

	return js_ast.Expr{Loc: loc, Data: e}
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

// This function takes "exprIn" as input from the caller and produces "exprOut"
// for the caller to pass along extra data. This is mostly for optional chaining.
func (p *parser) visitExprInOut(expr js_ast.Expr, in exprIn) (js_ast.Expr, exprOut) {
	if in.assignTarget != js_ast.AssignTargetNone && !p.isValidAssignmentTarget(expr) {
		p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: expr.Loc}, "Invalid assignment target")
	}

	switch e := expr.Data.(type) {
	case *js_ast.ENull, *js_ast.ESuper,
		*js_ast.EBoolean, *js_ast.EBigInt,
		*js_ast.ERegExp, *js_ast.EUndefined:

	case *js_ast.ENewTarget:
		if !p.fnOnlyDataVisit.isNewTargetAllowed {
			p.log.Add(logger.Error, &p.tracker, e.Range, "Cannot use \"new.target\" here:")
		}

	case *js_ast.EString:
		if e.LegacyOctalLoc.Start > 0 {
			if e.PreferTemplate {
				p.log.Add(logger.Error, &p.tracker, p.source.RangeOfLegacyOctalEscape(e.LegacyOctalLoc),
					"Legacy octal escape sequences cannot be used in template literals")
			} else if p.isStrictMode() {
				p.markStrictModeFeature(legacyOctalEscape, p.source.RangeOfLegacyOctalEscape(e.LegacyOctalLoc), "")
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

		if value, ok := p.valueForThis(expr.Loc, true /* shouldWarn */, in.assignTarget, isDeleteTarget, isCallTarget); ok {
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
				if p.isDotDefineMatch(expr, define.Parts) {
					// Substitute user-specified defines
					if define.Data.DefineFunc != nil {
						return p.valueForDefine(expr.Loc, define.Data.DefineFunc, identifierOpts{
							assignTarget:   in.assignTarget,
							isCallTarget:   isCallTarget,
							isDeleteTarget: isDeleteTarget,
						}), exprOut{}
					}
				}
			}
		}

		if p.importMetaRef != js_ast.InvalidRef {
			// Replace "import.meta" with a reference to the symbol
			p.recordUsage(p.importMetaRef)
			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: p.importMetaRef}}, exprOut{}
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

		// Handle assigning to a constant
		if in.assignTarget != js_ast.AssignTargetNone {
			switch p.symbols[result.ref.InnerIndex].Kind {
			case js_ast.SymbolConst:
				r := js_lexer.RangeOfIdentifier(p.source, expr.Loc)
				notes := []logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, result.declareLoc),
					fmt.Sprintf("The symbol %q was declared a constant here:", name))}

				// Make this an error when bundling because we may need to convert this
				// "const" into a "var" during bundling.
				if p.options.mode == config.ModeBundle {
					p.log.AddWithNotes(logger.Error, &p.tracker, r,
						fmt.Sprintf("Cannot assign to %q because it is a constant", name), notes)
				} else {
					p.log.AddWithNotes(logger.Warning, &p.tracker, r,
						fmt.Sprintf("This assignment will throw because %q is a constant", name), notes)
				}

			case js_ast.SymbolInjected:
				if where, ok := p.injectedSymbolSources[result.ref]; ok {
					r := js_lexer.RangeOfIdentifier(p.source, expr.Loc)
					tracker := logger.MakeLineColumnTracker(&where.source)
					p.log.AddWithNotes(logger.Error, &p.tracker, r,
						fmt.Sprintf("Cannot assign to %q because it's an import from an injected file", name),
						[]logger.MsgData{tracker.MsgData(js_lexer.RangeOfIdentifier(where.source, where.loc),
							fmt.Sprintf("The symbol %q was exported from %q here:", name, where.source.PrettyPath))})
				}
			}
		}

		// Substitute user-specified defines for unbound or injected symbols
		if p.symbols[e.Ref.InnerIndex].Kind.IsUnboundOrInjected() && !result.isInsideWithScope && e != p.deleteTarget {
			if data, ok := p.options.defines.IdentifierDefines[name]; ok {
				if data.DefineFunc != nil {
					new := p.valueForDefine(expr.Loc, data.DefineFunc, identifierOpts{
						assignTarget:   in.assignTarget,
						isCallTarget:   isCallTarget,
						isDeleteTarget: isDeleteTarget,
					})

					// Don't substitute an identifier for a non-identifier if this is an
					// assignment target, since it'll cause a syntax error
					if _, ok := new.Data.(*js_ast.EIdentifier); in.assignTarget == js_ast.AssignTargetNone || ok {
						p.ignoreUsage(e.Ref)
						return new, exprOut{}
					}
				}

				// Copy the side effect flags over in case this expression is unused
				if data.CanBeRemovedIfUnused {
					e.CanBeRemovedIfUnused = true
				}
				if data.CallCanBeUnwrappedIfUnused && !p.options.ignoreDCEAnnotations {
					e.CallCanBeUnwrappedIfUnused = true
				}
			}
		}

		return p.handleIdentifier(expr.Loc, e, identifierOpts{
			assignTarget:            in.assignTarget,
			isCallTarget:            isCallTarget,
			isDeleteTarget:          isDeleteTarget,
			wasOriginallyIdentifier: true,
		}), exprOut{}

	case *js_ast.EPrivateIdentifier:
		// We should never get here
		panic("Internal error")

	case *js_ast.EJSXElement:
		if e.TagOrNil.Data != nil {
			e.TagOrNil = p.visitExpr(e.TagOrNil)
			p.warnAboutImportNamespaceCall(e.TagOrNil, exprKindJSXTag)
		}

		// Visit properties
		for i, property := range e.Properties {
			if property.Kind != js_ast.PropertySpread {
				property.Key = p.visitExpr(property.Key)
			}
			if property.ValueOrNil.Data != nil {
				property.ValueOrNil = p.visitExpr(property.ValueOrNil)
			}
			if property.InitializerOrNil.Data != nil {
				property.InitializerOrNil = p.visitExpr(property.InitializerOrNil)
			}
			e.Properties[i] = property
		}

		// Visit children
		if len(e.Children) > 0 {
			for i, child := range e.Children {
				e.Children[i] = p.visitExpr(child)
			}
		}

		if p.options.jsx.Preserve {
			// If the tag is an identifier, mark it as needing to be upper-case
			switch tag := e.TagOrNil.Data.(type) {
			case *js_ast.EIdentifier:
				p.symbols[tag.Ref.InnerIndex].MustStartWithCapitalLetterForJSX = true

			case *js_ast.EImportIdentifier:
				p.symbols[tag.Ref.InnerIndex].MustStartWithCapitalLetterForJSX = true
			}
		} else {
			// A missing tag is a fragment
			if e.TagOrNil.Data == nil {
				var value js_ast.Expr
				if len(p.options.jsx.Fragment.Parts) > 0 {
					value = p.jsxStringsToMemberExpression(expr.Loc, p.options.jsx.Fragment.Parts)
				} else if constant := p.options.jsx.Fragment.Constant; constant != nil {
					value = js_ast.Expr{Loc: expr.Loc, Data: constant}
				}
				e.TagOrNil = value
			}

			// Arguments to createElement()
			args := []js_ast.Expr{e.TagOrNil}
			if len(e.Properties) > 0 {
				args = append(args, p.lowerObjectSpread(expr.Loc, &js_ast.EObject{
					Properties: e.Properties,
				}))
			} else {
				args = append(args, js_ast.Expr{Loc: expr.Loc, Data: js_ast.ENullShared})
			}
			if len(e.Children) > 0 {
				args = append(args, e.Children...)
			}

			// Call createElement()
			target := p.jsxStringsToMemberExpression(expr.Loc, p.options.jsx.Factory.Parts)
			p.warnAboutImportNamespaceCall(target, exprKindCall)
			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
				Target: target,
				Args:   args,

				// Enable tree shaking
				CanBeUnwrappedIfUnused: !p.options.ignoreDCEAnnotations,
			}}, exprOut{}
		}

	case *js_ast.ETemplate:
		if e.LegacyOctalLoc.Start > 0 {
			p.log.Add(logger.Error, &p.tracker, p.source.RangeOfLegacyOctalEscape(e.LegacyOctalLoc),
				"Legacy octal escape sequences cannot be used in template literals")
		}

		if e.TagOrNil.Data != nil {
			p.templateTag = e.TagOrNil.Data
			e.TagOrNil = p.visitExpr(e.TagOrNil)

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
				}})
			}
		}

		for i, part := range e.Parts {
			e.Parts[i].Value = p.visitExpr(part.Value)
		}

		// When mangling, inline string values into the template literal. Note that
		// it may no longer be a template literal after this point (it may turn into
		// a plain string literal instead).
		if p.options.mangleSyntax {
			expr = p.mangleTemplate(expr.Loc, e)
		}

		shouldLowerTemplateLiteral := p.options.unsupportedJSFeatures.Has(compat.TemplateLiteral)

		// Lower tagged template literals that include "</script"
		// since we won't be able to escape it without lowering it
		if !shouldLowerTemplateLiteral && e.TagOrNil.Data != nil {
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
				return p.lowerTemplateLiteral(expr.Loc, e), exprOut{}
			}
		}

	case *js_ast.EBinary:
		// Special-case EPrivateIdentifier to allow it here
		if private, ok := e.Left.Data.(*js_ast.EPrivateIdentifier); ok && e.Op == js_ast.BinOpIn {
			name := p.loadNameFromRef(private.Ref)
			result := p.findSymbol(e.Left.Loc, name)
			private.Ref = result.ref

			// Unlike regular identifiers, there are no unbound private identifiers
			symbol := &p.symbols[result.ref.InnerIndex]
			if !symbol.Kind.IsPrivate() {
				r := logger.Range{Loc: e.Left.Loc, Len: int32(len(name))}
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Private name %q must be declared in an enclosing class", name))
			}

			e.Right = p.visitExpr(e.Right)

			if p.privateSymbolNeedsToBeLowered(private) {
				return p.lowerPrivateBrandCheck(e.Right, expr.Loc, private), exprOut{}
			}
			break
		}

		isCallTarget := e == p.callTarget
		isTemplateTag := e == p.templateTag
		isStmtExpr := e == p.stmtExprValue
		wasAnonymousNamedExpr := p.isAnonymousNamedExpr(e.Right)
		e.Left, _ = p.visitExprInOut(e.Left, exprIn{assignTarget: e.Op.BinaryAssignTarget()})

		// Mark the control flow as dead if the branch is never taken
		switch e.Op {
		case js_ast.BinOpLogicalOr:
			if boolean, _, ok := toBooleanWithSideEffects(e.Left.Data); ok && boolean {
				// "true || dead"
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.Right = p.visitExpr(e.Right)
				p.isControlFlowDead = old
			} else {
				e.Right = p.visitExpr(e.Right)
			}

		case js_ast.BinOpLogicalAnd:
			if boolean, _, ok := toBooleanWithSideEffects(e.Left.Data); ok && !boolean {
				// "false && dead"
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.Right = p.visitExpr(e.Right)
				p.isControlFlowDead = old
			} else {
				e.Right = p.visitExpr(e.Right)
			}

		case js_ast.BinOpNullishCoalescing:
			if isNullOrUndefined, _, ok := toNullOrUndefinedWithSideEffects(e.Left.Data); ok && !isNullOrUndefined {
				// "notNullOrUndefined ?? dead"
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.Right = p.visitExpr(e.Right)
				p.isControlFlowDead = old
			} else {
				e.Right = p.visitExpr(e.Right)
			}

		default:
			e.Right = p.visitExpr(e.Right)
		}

		// Always put constants on the right for equality comparisons to help
		// reduce the number of cases we have to check during pattern matching. We
		// can only reorder expressions that do not have any side effects.
		switch e.Op {
		case js_ast.BinOpLooseEq, js_ast.BinOpLooseNe, js_ast.BinOpStrictEq, js_ast.BinOpStrictNe:
			if isPrimitiveToReorder(e.Left.Data) && !isPrimitiveToReorder(e.Right.Data) {
				e.Left, e.Right = e.Right, e.Left
			}
		}

		// Post-process the binary expression
		switch e.Op {
		case js_ast.BinOpComma:
			// "(1, 2)" => "2"
			// "(sideEffects(), 2)" => "(sideEffects(), 2)"
			if p.options.mangleSyntax {
				e.Left = p.simplifyUnusedExpr(e.Left)
				if e.Left.Data == nil {
					// "(1, fn)()" => "fn()"
					// "(1, this.fn)" => "this.fn"
					// "(1, this.fn)()" => "(0, this.fn)()"
					if (isCallTarget || isTemplateTag) && hasValueForThisInCall(e.Right) {
						return js_ast.JoinWithComma(js_ast.Expr{Loc: e.Left.Loc, Data: &js_ast.ENumber{}}, e.Right), exprOut{}
					}
					return e.Right, exprOut{}
				}
			}

		case js_ast.BinOpLooseEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("==", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("==", e.Right, afterOpLoc)
			}
			p.warnAboutTypeofAndString(e.Left, e.Right)

			if p.options.mangleSyntax {
				// "x == void 0" => "x == null"
				if _, ok := e.Right.Data.(*js_ast.EUndefined); ok {
					e.Right.Data = js_ast.ENullShared
				}

				if result, ok := maybeSimplifyEqualityComparison(e, false /* isNotEqual */); ok {
					return result, exprOut{}
				}
			}

		case js_ast.BinOpStrictEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("===", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("===", e.Right, afterOpLoc)
			}
			p.warnAboutTypeofAndString(e.Left, e.Right)

			if p.options.mangleSyntax {
				// "typeof x === 'undefined'" => "typeof x == 'undefined'"
				if canChangeStrictToLoose(e.Left, e.Right) {
					e.Op = js_ast.BinOpLooseEq
				}

				if result, ok := maybeSimplifyEqualityComparison(e, false /* isNotEqual */); ok {
					return result, exprOut{}
				}
			}

		case js_ast.BinOpLooseNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: !result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("!=", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("!=", e.Right, afterOpLoc)
			}
			p.warnAboutTypeofAndString(e.Left, e.Right)

			if p.options.mangleSyntax {
				// "x != void 0" => "x != null"
				if _, ok := e.Right.Data.(*js_ast.EUndefined); ok {
					e.Right.Data = js_ast.ENullShared
				}

				if result, ok := maybeSimplifyEqualityComparison(e, true /* isNotEqual */); ok {
					return result, exprOut{}
				}
			}

		case js_ast.BinOpStrictNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: !result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("!==", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("!==", e.Right, afterOpLoc)
			}
			p.warnAboutTypeofAndString(e.Left, e.Right)

			if p.options.mangleSyntax {
				// "typeof x !== 'undefined'" => "typeof x != 'undefined'"
				if canChangeStrictToLoose(e.Left, e.Right) {
					e.Op = js_ast.BinOpLooseNe
				}

				if result, ok := maybeSimplifyEqualityComparison(e, true /* isNotEqual */); ok {
					return result, exprOut{}
				}
			}

		case js_ast.BinOpNullishCoalescing:
			if isNullOrUndefined, sideEffects, ok := toNullOrUndefinedWithSideEffects(e.Left.Data); ok {
				if !isNullOrUndefined {
					return e.Left, exprOut{}
				} else if sideEffects == noSideEffects {
					// "(null ?? fn)()" => "fn()"
					// "(null ?? this.fn)" => "this.fn"
					// "(null ?? this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Right) {
						return js_ast.JoinWithComma(js_ast.Expr{Loc: e.Left.Loc, Data: &js_ast.ENumber{}}, e.Right), exprOut{}
					}

					return e.Right, exprOut{}
				}
			}

			if p.options.mangleSyntax {
				// "a ?? (b ?? c)" => "a ?? b ?? c"
				if right, ok := e.Right.Data.(*js_ast.EBinary); ok && right.Op == js_ast.BinOpNullishCoalescing {
					e.Left = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpNullishCoalescing, e.Left, right.Left)
					e.Right = right.Right
				}
			}

			if p.options.unsupportedJSFeatures.Has(compat.NullishCoalescing) {
				return p.lowerNullishCoalescing(expr.Loc, e.Left, e.Right), exprOut{}
			}

		case js_ast.BinOpLogicalOr:
			if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Left.Data); ok {
				if boolean {
					return e.Left, exprOut{}
				} else if sideEffects == noSideEffects {
					// "(0 || fn)()" => "fn()"
					// "(0 || this.fn)" => "this.fn"
					// "(0 || this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Right) {
						return js_ast.JoinWithComma(js_ast.Expr{Loc: e.Left.Loc, Data: &js_ast.ENumber{}}, e.Right), exprOut{}
					}
					return e.Right, exprOut{}
				}
			}

			if p.options.mangleSyntax {
				// "a || (b || c)" => "a || b || c"
				if right, ok := e.Right.Data.(*js_ast.EBinary); ok && right.Op == js_ast.BinOpLogicalOr {
					e.Left = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Left, right.Left)
					e.Right = right.Right
				}

				// "a === null || a === undefined" => "a == null"
				if left, right, ok := isBinaryNullAndUndefined(e.Left, e.Right, js_ast.BinOpStrictEq); ok {
					e.Op = js_ast.BinOpLooseEq
					e.Left = left
					e.Right = right
				}
			}

		case js_ast.BinOpLogicalAnd:
			if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Left.Data); ok {
				if !boolean {
					return e.Left, exprOut{}
				} else if sideEffects == noSideEffects {
					// "(1 && fn)()" => "fn()"
					// "(1 && this.fn)" => "this.fn"
					// "(1 && this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Right) {
						return js_ast.JoinWithComma(js_ast.Expr{Loc: e.Left.Loc, Data: &js_ast.ENumber{}}, e.Right), exprOut{}
					}
					return e.Right, exprOut{}
				}
			}

			if p.options.mangleSyntax {
				// "a && (b && c)" => "a && b && c"
				if right, ok := e.Right.Data.(*js_ast.EBinary); ok && right.Op == js_ast.BinOpLogicalAnd {
					e.Left = js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Left, right.Left)
					e.Right = right.Right
				}

				// "a !== null && a !== undefined" => "a != null"
				if left, right, ok := isBinaryNullAndUndefined(e.Left, e.Right, js_ast.BinOpStrictNe); ok {
					e.Op = js_ast.BinOpLooseNe
					e.Left = left
					e.Right = right
				}
			}

		case js_ast.BinOpAdd:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: left + right}}, exprOut{}
				}
			}

			// "'abc' + 'xyz'" => "'abcxyz'"
			if result := foldStringAddition(e.Left, e.Right); result != nil {
				return *result, exprOut{}
			}

			if left, ok := e.Left.Data.(*js_ast.EBinary); ok && left.Op == js_ast.BinOpAdd {
				// "x + 'abc' + 'xyz'" => "x + 'abcxyz'"
				if result := foldStringAddition(left.Right, e.Right); result != nil {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBinary{Op: left.Op, Left: left.Left, Right: *result}}, exprOut{}
				}
			}

		case js_ast.BinOpSub:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: left - right}}, exprOut{}
				}
			}

		case js_ast.BinOpMul:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: left * right}}, exprOut{}
				}
			}

		case js_ast.BinOpDiv:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: left / right}}, exprOut{}
				}
			}

		case js_ast.BinOpRem:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: math.Mod(left, right)}}, exprOut{}
				}
			}

		case js_ast.BinOpPow:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: math.Pow(left, right)}}, exprOut{}
				}
			}

			// Lower the exponentiation operator for browsers that don't support it
			if p.options.unsupportedJSFeatures.Has(compat.ExponentOperator) {
				return p.callRuntime(expr.Loc, "__pow", []js_ast.Expr{e.Left, e.Right}), exprOut{}
			}

		case js_ast.BinOpShl:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(toInt32(left) << (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case js_ast.BinOpShr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(toInt32(left) >> (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case js_ast.BinOpUShr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(toUint32(left) >> (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case js_ast.BinOpBitwiseAnd:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(toInt32(left) & toInt32(right))}}, exprOut{}
				}
			}

		case js_ast.BinOpBitwiseOr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(toInt32(left) | toInt32(right))}}, exprOut{}
				}
			}

		case js_ast.BinOpBitwiseXor:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(toInt32(left) ^ toInt32(right))}}, exprOut{}
				}
			}

			////////////////////////////////////////////////////////////////////////////////
			// All assignment operators below here

		case js_ast.BinOpAssign:
			// Optionally preserve the name
			if id, ok := e.Left.Data.(*js_ast.EIdentifier); ok {
				e.Right = p.maybeKeepExprSymbolName(e.Right, p.symbols[id.Ref.InnerIndex].OriginalName, wasAnonymousNamedExpr)
			}

			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSet(target, loc, private, e.Right), exprOut{}
			}

			if property := p.extractSuperProperty(e.Left); property.Data != nil {
				return p.lowerSuperPropertySet(e.Left.Loc, property, e.Right), exprOut{}
			}

			// Lower assignment destructuring patterns for browsers that don't
			// support them. Note that assignment expressions are used to represent
			// initializers in binding patterns, so only do this if we're not
			// ourselves the target of an assignment. Example: "[a = b] = c"
			if in.assignTarget == js_ast.AssignTargetNone {
				mode := objRestMustReturnInitExpr
				if isStmtExpr {
					mode = objRestReturnValueIsUnused
				}
				if result, ok := p.lowerAssign(e.Left, e.Right, mode); ok {
					return result, exprOut{}
				}
			}

		case js_ast.BinOpAddAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpAdd, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpSubAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpSub, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpMulAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpMul, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpDivAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpDiv, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpRemAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpRem, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpPowAssign:
			// Lower the exponentiation operator for browsers that don't support it
			if p.options.unsupportedJSFeatures.Has(compat.ExponentOperator) {
				return p.lowerExponentiationAssignmentOperator(expr.Loc, e), exprOut{}
			}

			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpPow, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpShlAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpShl, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpShrAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpShr, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpUShrAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpUShr, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpBitwiseOrAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpBitwiseOr, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpBitwiseAndAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpBitwiseAnd, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpBitwiseXorAssign:
			if result := p.maybeLowerSetBinOp(e.Left, js_ast.BinOpBitwiseXor, e.Right); result.Data != nil {
				return result, exprOut{}
			}

		case js_ast.BinOpNullishCoalescingAssign:
			if value, ok := p.lowerNullishCoalescingAssignmentOperator(expr.Loc, e); ok {
				return value, exprOut{}
			}

		case js_ast.BinOpLogicalAndAssign:
			if value, ok := p.lowerLogicalAssignmentOperator(expr.Loc, e, js_ast.BinOpLogicalAnd); ok {
				return value, exprOut{}
			}

		case js_ast.BinOpLogicalOrAssign:
			if value, ok := p.lowerLogicalAssignmentOperator(expr.Loc, e, js_ast.BinOpLogicalOr); ok {
				return value, exprOut{}
			}
		}

		// "(a, b) + c" => "a, b + c"
		if p.options.mangleSyntax && e.Op != js_ast.BinOpComma {
			if comma, ok := e.Left.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
				return js_ast.JoinWithComma(comma.Left, js_ast.Expr{
					Loc: comma.Right.Loc,
					Data: &js_ast.EBinary{
						Op:    e.Op,
						Left:  comma.Right,
						Right: e.Right,
					},
				}), exprOut{}
			}
		}

	case *js_ast.EIndex:
		isCallTarget := e == p.callTarget
		isTemplateTag := e == p.templateTag
		isDeleteTarget := e == p.deleteTarget

		// "a['b']" => "a.b"
		if p.options.mangleSyntax {
			if str, ok := e.Index.Data.(*js_ast.EString); ok && js_lexer.IsIdentifierUTF16(str.Value) {
				dot := &js_ast.EDot{
					Target:        e.Target,
					Name:          js_lexer.UTF16ToString(str.Value),
					NameLoc:       e.Index.Loc,
					OptionalChain: e.OptionalChain,
				}
				if isCallTarget {
					p.callTarget = dot
				}
				if isDeleteTarget {
					p.deleteTarget = dot
				}
				return p.visitExprInOut(js_ast.Expr{Loc: expr.Loc, Data: dot}, in)
			}
		}

		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == js_ast.OptionalChainContinue,
		})
		e.Target = target

		// Special-case EPrivateIdentifier to allow it here
		if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok {
			name := p.loadNameFromRef(private.Ref)
			result := p.findSymbol(e.Index.Loc, name)
			private.Ref = result.ref

			// Unlike regular identifiers, there are no unbound private identifiers
			kind := p.symbols[result.ref.InnerIndex].Kind
			if !kind.IsPrivate() {
				r := logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
				p.log.Add(logger.Error, &p.tracker, r, fmt.Sprintf("Private name %q must be declared in an enclosing class", name))
			} else {
				var r logger.Range
				var text string
				if in.assignTarget != js_ast.AssignTargetNone && (kind == js_ast.SymbolPrivateMethod || kind == js_ast.SymbolPrivateStaticMethod) {
					r = logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
					text = fmt.Sprintf("Writing to read-only method %q will throw", name)
				} else if in.assignTarget != js_ast.AssignTargetNone && (kind == js_ast.SymbolPrivateGet || kind == js_ast.SymbolPrivateStaticGet) {
					r = logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
					text = fmt.Sprintf("Writing to getter-only property %q will throw", name)
				} else if in.assignTarget != js_ast.AssignTargetReplace && (kind == js_ast.SymbolPrivateSet || kind == js_ast.SymbolPrivateStaticSet) {
					r = logger.Range{Loc: e.Index.Loc, Len: int32(len(name))}
					text = fmt.Sprintf("Reading from setter-only property %q will throw", name)
				}
				if text != "" {
					if !p.suppressWarningsAboutWeirdCode {
						p.log.Add(logger.Warning, &p.tracker, r, text)
					} else {
						p.log.Add(logger.Debug, &p.tracker, r, text)
					}
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
			e.Index = p.visitExpr(e.Index)
		}

		// Lower "super[prop]" if necessary
		if e.OptionalChain == js_ast.OptionalChainNone && in.assignTarget == js_ast.AssignTargetNone &&
			!isCallTarget && p.shouldLowerSuperPropertyAccess(e.Target) {
			// "super[foo]" => "__superGet(foo)"
			return p.lowerSuperPropertyGet(expr.Loc, e.Index), exprOut{}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != js_ast.OptionalChainNone
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out)
		}

		// Potentially rewrite this property access
		out = exprOut{
			childContainsOptionalChain: containsOptionalChain,
			thisArgFunc:                out.thisArgFunc,
			thisArgWrapFunc:            out.thisArgWrapFunc,
		}
		if !in.hasChainParent {
			out.thisArgFunc = nil
			out.thisArgWrapFunc = nil
		}
		if str, ok := e.Index.Data.(*js_ast.EString); ok && e.OptionalChain == js_ast.OptionalChainNone {
			preferQuotedKey := !p.options.mangleSyntax
			if value, ok := p.maybeRewritePropertyAccess(expr.Loc, in.assignTarget, isDeleteTarget,
				e.Target, js_lexer.UTF16ToString(str.Value), e.Index.Loc, isCallTarget, preferQuotedKey); ok {
				return value, out
			}
		}

		// Create an error for assigning to an import namespace when bundling. Even
		// though this is a run-time error, we make it a compile-time error when
		// bundling because scope hoisting means these will no longer be run-time
		// errors.
		if p.options.mode == config.ModeBundle && (in.assignTarget != js_ast.AssignTargetNone || isDeleteTarget) {
			if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok && p.symbols[id.Ref.InnerIndex].Kind == js_ast.SymbolImport {
				r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
				p.log.AddWithNotes(logger.Error, &p.tracker, r,
					fmt.Sprintf("Cannot assign to property on import %q", p.symbols[id.Ref.InnerIndex].OriginalName),
					[]logger.MsgData{{Text: "Imports are immutable in JavaScript. " +
						"To modify the value of this import, you must export a setter function in the " +
						"imported file and then import and call that function here instead."}})

			}
		}

		return js_ast.Expr{Loc: expr.Loc, Data: e}, out

	case *js_ast.EUnary:
		switch e.Op {
		case js_ast.UnOpTypeof:
			_, idBefore := e.Value.Data.(*js_ast.EIdentifier)
			e.Value, _ = p.visitExprInOut(e.Value, exprIn{assignTarget: e.Op.UnaryAssignTarget()})
			id, idAfter := e.Value.Data.(*js_ast.EIdentifier)

			// The expression "typeof (0, x)" must not become "typeof x" if "x"
			// is unbound because that could suppress a ReferenceError from "x"
			if !idBefore && idAfter && p.symbols[id.Ref.InnerIndex].Kind == js_ast.SymbolUnbound {
				e.Value = js_ast.JoinWithComma(js_ast.Expr{Loc: e.Value.Loc, Data: &js_ast.ENumber{}}, e.Value)
			}

			if typeof, ok := typeofWithoutSideEffects(e.Value.Data); ok {
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(typeof)}}, exprOut{}
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
				p.log.Add(kind, &p.tracker, r, text)
			}

			p.deleteTarget = e.Value.Data
			canBeDeletedBefore := canBeDeleted(e.Value)
			value, out := p.visitExprInOut(e.Value, exprIn{hasChainParent: true})
			e.Value = value
			canBeDeletedAfter := canBeDeleted(e.Value)

			// Lower optional chaining if present since we're guaranteed to be the
			// end of the chain
			if out.childContainsOptionalChain {
				return p.lowerOptionalChain(expr, in, out)
			}

			// Make sure we don't accidentally change the return value
			//
			//   Returns false:
			//     "var a; delete (a)"
			//     "var a = Object.freeze({b: 1}); delete (a.b)"
			//     "var a = Object.freeze({b: 1}); delete (a?.b)"
			//     "var a = Object.freeze({b: 1}); delete (a['b'])"
			//     "var a = Object.freeze({b: 1}); delete (a?.['b'])"
			//
			//   Returns true:
			//     "var a; delete (0, a)"
			//     "var a = Object.freeze({b: 1}); delete (true && a.b)"
			//     "var a = Object.freeze({b: 1}); delete (false || a?.b)"
			//     "var a = Object.freeze({b: 1}); delete (null ?? a?.['b'])"
			//     "var a = Object.freeze({b: 1}); delete (true ? a['b'] : a['b'])"
			//
			if canBeDeletedAfter && !canBeDeletedBefore {
				e.Value = js_ast.JoinWithComma(js_ast.Expr{Loc: e.Value.Loc, Data: &js_ast.ENumber{}}, e.Value)
			}

		default:
			e.Value, _ = p.visitExprInOut(e.Value, exprIn{assignTarget: e.Op.UnaryAssignTarget()})

			// Post-process the unary expression
			switch e.Op {
			case js_ast.UnOpNot:
				if p.options.mangleSyntax {
					e.Value = p.simplifyBooleanExpr(e.Value)
				}

				if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Value.Data); ok && sideEffects == noSideEffects {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EBoolean{Value: !boolean}}, exprOut{}
				}

				if p.options.mangleSyntax {
					if result, ok := js_ast.MaybeSimplifyNot(e.Value); ok {
						return result, exprOut{}
					}
				}

			case js_ast.UnOpVoid:
				if p.exprCanBeRemovedIfUnused(e.Value) {
					return js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared}, exprOut{}
				}

			case js_ast.UnOpPos:
				if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: number}}, exprOut{}
				}

			case js_ast.UnOpNeg:
				if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: -number}}, exprOut{}
				}

				////////////////////////////////////////////////////////////////////////////////
				// All assignment operators below here

			case js_ast.UnOpPreDec, js_ast.UnOpPreInc, js_ast.UnOpPostDec, js_ast.UnOpPostInc:
				if target, loc, private := p.extractPrivateIndex(e.Value); private != nil {
					return p.lowerPrivateSetUnOp(target, loc, private, e.Op), exprOut{}
				}
				if property := p.extractSuperProperty(e.Value); property.Data != nil {
					e.Value = p.callSuperPropertyWrapper(expr.Loc, property, true /* includeGet */)
				}
			}
		}

		// "-(a, b)" => "a, -b"
		if p.options.mangleSyntax && e.Op != js_ast.UnOpDelete && e.Op != js_ast.UnOpTypeof {
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

	case *js_ast.EDot:
		isDeleteTarget := e == p.deleteTarget
		isCallTarget := e == p.callTarget

		// Check both user-specified defines and known globals
		if defines, ok := p.options.defines.DotDefines[e.Name]; ok {
			for _, define := range defines {
				if p.isDotDefineMatch(expr, define.Parts) {
					// Substitute user-specified defines
					if define.Data.DefineFunc != nil {
						return p.valueForDefine(expr.Loc, define.Data.DefineFunc, identifierOpts{
							assignTarget:   in.assignTarget,
							isCallTarget:   isCallTarget,
							isDeleteTarget: isDeleteTarget,
						}), exprOut{}
					}

					// Copy the side effect flags over in case this expression is unused
					if define.Data.CanBeRemovedIfUnused {
						e.CanBeRemovedIfUnused = true
					}
					if define.Data.CallCanBeUnwrappedIfUnused && !p.options.ignoreDCEAnnotations {
						e.CallCanBeUnwrappedIfUnused = true
					}
					break
				}
			}
		}

		// Track ".then().catch()" chains
		if isCallTarget && p.thenCatchChain.nextTarget == e {
			if e.Name == "catch" {
				p.thenCatchChain = thenCatchChain{
					nextTarget: e.Target.Data,
					hasCatch:   true,
				}
			} else if e.Name == "then" {
				p.thenCatchChain = thenCatchChain{
					nextTarget: e.Target.Data,
					hasCatch:   p.thenCatchChain.hasCatch || p.thenCatchChain.hasMultipleArgs,
				}
			}
		}

		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == js_ast.OptionalChainContinue,
		})
		e.Target = target

		// Lower "super.prop" if necessary
		if e.OptionalChain == js_ast.OptionalChainNone && in.assignTarget == js_ast.AssignTargetNone &&
			!isCallTarget && p.shouldLowerSuperPropertyAccess(e.Target) {
			// "super.foo" => "__superGet('foo')"
			key := js_ast.Expr{Loc: e.NameLoc, Data: &js_ast.EString{Value: js_lexer.StringToUTF16(e.Name)}}
			return p.lowerSuperPropertyGet(expr.Loc, key), exprOut{}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != js_ast.OptionalChainNone
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out)
		}

		// Potentially rewrite this property access
		out = exprOut{
			childContainsOptionalChain: containsOptionalChain,
			thisArgFunc:                out.thisArgFunc,
			thisArgWrapFunc:            out.thisArgWrapFunc,
		}
		if !in.hasChainParent {
			out.thisArgFunc = nil
			out.thisArgWrapFunc = nil
		}
		if e.OptionalChain == js_ast.OptionalChainNone {
			if value, ok := p.maybeRewritePropertyAccess(expr.Loc, in.assignTarget,
				isDeleteTarget, e.Target, e.Name, e.NameLoc, isCallTarget, false); ok {
				return value, out
			}
		}
		return js_ast.Expr{Loc: expr.Loc, Data: e}, out

	case *js_ast.EIf:
		isCallTarget := e == p.callTarget
		e.Test = p.visitExpr(e.Test)

		if p.options.mangleSyntax {
			e.Test = p.simplifyBooleanExpr(e.Test)
		}

		// Fold constants
		if boolean, sideEffects, ok := toBooleanWithSideEffects(e.Test.Data); !ok {
			e.Yes = p.visitExpr(e.Yes)
			e.No = p.visitExpr(e.No)
		} else {
			// Mark the control flow as dead if the branch is never taken
			if boolean {
				// "true ? live : dead"
				e.Yes = p.visitExpr(e.Yes)
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.No = p.visitExpr(e.No)
				p.isControlFlowDead = old

				if p.options.mangleSyntax {
					// "(a, true) ? b : c" => "a, b"
					if sideEffects == couldHaveSideEffects {
						return js_ast.JoinWithComma(p.simplifyUnusedExpr(e.Test), e.Yes), exprOut{}
					}

					// "(1 ? fn : 2)()" => "fn()"
					// "(1 ? this.fn : 2)" => "this.fn"
					// "(1 ? this.fn : 2)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Yes) {
						return js_ast.JoinWithComma(js_ast.Expr{Loc: e.Test.Loc, Data: &js_ast.ENumber{}}, e.Yes), exprOut{}
					}

					return e.Yes, exprOut{}
				}
			} else {
				// "false ? dead : live"
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				e.Yes = p.visitExpr(e.Yes)
				p.isControlFlowDead = old
				e.No = p.visitExpr(e.No)

				if p.options.mangleSyntax {
					// "(a, false) ? b : c" => "a, c"
					if sideEffects == couldHaveSideEffects {
						return js_ast.JoinWithComma(p.simplifyUnusedExpr(e.Test), e.No), exprOut{}
					}

					// "(0 ? 1 : fn)()" => "fn()"
					// "(0 ? 1 : this.fn)" => "this.fn"
					// "(0 ? 1 : this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.No) {
						return js_ast.JoinWithComma(js_ast.Expr{Loc: e.Test.Loc, Data: &js_ast.ENumber{}}, e.No), exprOut{}
					}

					return e.No, exprOut{}
				}
			}
		}

		if p.options.mangleSyntax {
			return p.mangleIfExpr(expr.Loc, e), exprOut{}
		}

	case *js_ast.EAwait:
		p.awaitTarget = e.Value.Data
		e.Value = p.visitExpr(e.Value)

		// "await" expressions turn into "yield" expressions when lowering
		if p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) {
			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EYield{ValueOrNil: e.Value}}, exprOut{}
		}

	case *js_ast.EYield:
		if e.ValueOrNil.Data != nil {
			e.ValueOrNil = p.visitExpr(e.ValueOrNil)
		}

	case *js_ast.EArray:
		if in.assignTarget != js_ast.AssignTargetNone {
			if e.CommaAfterSpread.Start != 0 {
				p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: e.CommaAfterSpread, Len: 1}, "Unexpected \",\" after rest pattern")
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
					wasAnonymousNamedExpr := p.isAnonymousNamedExpr(e2.Right)
					e2.Left, _ = p.visitExprInOut(e2.Left, exprIn{assignTarget: js_ast.AssignTargetReplace})
					e2.Right = p.visitExpr(e2.Right)

					// Optionally preserve the name
					if id, ok := e2.Left.Data.(*js_ast.EIdentifier); ok {
						e2.Right = p.maybeKeepExprSymbolName(
							e2.Right, p.symbols[id.Ref.InnerIndex].OriginalName, wasAnonymousNamedExpr)
					}
				} else {
					item, _ = p.visitExprInOut(item, exprIn{assignTarget: in.assignTarget})
				}
			default:
				item, _ = p.visitExprInOut(item, exprIn{assignTarget: in.assignTarget})
			}
			e.Items[i] = item
		}

		// "[1, ...[2, 3], 4]" => "[1, 2, 3, 4]"
		if p.options.mangleSyntax && hasSpread && in.assignTarget == js_ast.AssignTargetNone {
			e.Items = inlineSpreadsOfArrayLiterals(e.Items)
		}

	case *js_ast.EObject:
		if in.assignTarget != js_ast.AssignTargetNone {
			if e.CommaAfterSpread.Start != 0 {
				p.log.Add(logger.Error, &p.tracker, logger.Range{Loc: e.CommaAfterSpread, Len: 1}, "Unexpected \",\" after rest pattern")
			}
			p.markSyntaxFeature(compat.Destructuring, logger.Range{Loc: expr.Loc, Len: 1})
		}
		hasSpread := false
		protoRange := logger.Range{}
		for i := range e.Properties {
			property := &e.Properties[i]

			if property.Kind != js_ast.PropertySpread {
				key := p.visitExpr(property.Key)
				e.Properties[i].Key = key

				// Forbid duplicate "__proto__" properties according to the specification
				if !property.IsComputed && !property.WasShorthand && !property.IsMethod && in.assignTarget == js_ast.AssignTargetNone {
					if str, ok := key.Data.(*js_ast.EString); ok && js_lexer.UTF16EqualsString(str.Value, "__proto__") {
						r := js_lexer.RangeOfIdentifier(p.source, key.Loc)
						if protoRange.Len > 0 {
							p.log.AddWithNotes(logger.Error, &p.tracker, r,
								"Cannot specify the \"__proto__\" property more than once per object",
								[]logger.MsgData{p.tracker.MsgData(protoRange, "The earlier \"__proto__\" property is here:")})
						} else {
							protoRange = r
						}
					}
				}

				// "{['x']: y}" => "{x: y}"
				if p.options.mangleSyntax && property.IsComputed {
					if str, ok := key.Data.(*js_ast.EString); ok && js_lexer.IsIdentifierUTF16(str.Value) && !js_lexer.UTF16EqualsString(str.Value, "__proto__") {
						property.IsComputed = false
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
				property.ValueOrNil, _ = p.visitExprInOut(property.ValueOrNil, exprIn{assignTarget: in.assignTarget})
			}
			if property.InitializerOrNil.Data != nil {
				wasAnonymousNamedExpr := p.isAnonymousNamedExpr(property.InitializerOrNil)
				property.InitializerOrNil = p.visitExpr(property.InitializerOrNil)

				// Optionally preserve the name
				if property.ValueOrNil.Data != nil {
					if id, ok := property.ValueOrNil.Data.(*js_ast.EIdentifier); ok {
						property.InitializerOrNil = p.maybeKeepExprSymbolName(
							property.InitializerOrNil, p.symbols[id.Ref.InnerIndex].OriginalName, wasAnonymousNamedExpr)
					}
				}
			}
		}

		// Check for and warn about duplicate keys in object literals
		if len(e.Properties) > 1 && !p.suppressWarningsAboutWeirdCode {
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
			keys := make(map[string]existingKey)
			for _, property := range e.Properties {
				if property.Kind != js_ast.PropertySpread {
					if str, ok := property.Key.Data.(*js_ast.EString); ok {
						key := js_lexer.UTF16ToString(str.Value)
						prevKey := keys[key]
						nextKey := existingKey{kind: keyNormal, loc: property.Key.Loc}
						if property.Kind == js_ast.PropertyGet {
							nextKey.kind = keyGet
						} else if property.Kind == js_ast.PropertySet {
							nextKey.kind = keySet
						}
						if prevKey.kind != keyMissing && key != "__proto__" {
							if (prevKey.kind == keyGet && nextKey.kind == keySet) || (prevKey.kind == keySet && nextKey.kind == keyGet) {
								nextKey.kind = keyGetAndSet
							} else {
								r := js_lexer.RangeOfIdentifier(p.source, property.Key.Loc)
								p.log.AddWithNotes(logger.Warning, &p.tracker, r, fmt.Sprintf("Duplicate key %q in object literal", key),
									[]logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, prevKey.loc),
										fmt.Sprintf("The original key %q is here:", key))})
							}
						}
						keys[key] = nextKey
					}
				}
			}
		}

		if in.assignTarget == js_ast.AssignTargetNone {
			// "{a, ...{b, c}, d}" => "{a, b, c, d}"
			if p.options.mangleSyntax && hasSpread {
				var properties []js_ast.Property
				for _, property := range e.Properties {
					if property.Kind == js_ast.PropertySpread {
						switch v := property.ValueOrNil.Data.(type) {
						case *js_ast.EBoolean, *js_ast.ENull, *js_ast.EUndefined, *js_ast.ENumber,
							*js_ast.EBigInt, *js_ast.ERegExp, *js_ast.EFunction, *js_ast.EArrow:
							// This value is ignored because it doesn't have any of its own properties
							continue

						case *js_ast.EObject:
							for i, p := range v.Properties {
								// Getters are evaluated at iteration time. The property
								// descriptor is not inlined into the caller. Since we are not
								// evaluating code at compile time, just bail if we hit one
								// and preserve the spread with the remaining properties.
								if p.Kind == js_ast.PropertyGet || p.Kind == js_ast.PropertySet {
									v.Properties = v.Properties[i:]
									properties = append(properties, property)
									break
								}

								// Also bail if we hit a verbatim "__proto__" key. This will
								// actually set the prototype of the object being spread so
								// inlining it is not correct.
								if p.Kind == js_ast.PropertyNormal && !p.IsComputed && !p.IsMethod {
									if str, ok := p.Key.Data.(*js_ast.EString); ok && js_lexer.UTF16EqualsString(str.Value, "__proto__") {
										v.Properties = v.Properties[i:]
										properties = append(properties, property)
										break
									}
								}

								properties = append(properties, p)
							}
							continue
						}
					}
					properties = append(properties, property)
				}
				e.Properties = properties
			}

			// Object expressions represent both object literals and binding patterns.
			// Only lower object spread if we're an object literal, not a binding pattern.
			return p.lowerObjectSpread(expr.Loc, e), exprOut{}
		}

	case *js_ast.EImportCall:
		isAwaitTarget := e == p.awaitTarget
		isThenCatchTarget := e == p.thenCatchChain.nextTarget && p.thenCatchChain.hasCatch
		e.Expr = p.visitExpr(e.Expr)

		var assertions *[]ast.AssertEntry
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
			// only an "assert" clause. In that case we can split this AST node.
			if object, ok := e.OptionsOrNil.Data.(*js_ast.EObject); ok {
				if len(object.Properties) == 1 {
					if prop := object.Properties[0]; prop.Kind == js_ast.PropertyNormal && !prop.IsComputed && !prop.IsMethod {
						if str, ok := prop.Key.Data.(*js_ast.EString); ok && js_lexer.UTF16EqualsString(str.Value, "assert") {
							if value, ok := prop.ValueOrNil.Data.(*js_ast.EObject); ok {
								entries := []ast.AssertEntry{}
								for _, p := range value.Properties {
									if p.Kind == js_ast.PropertyNormal && !p.IsComputed && !p.IsMethod {
										if key, ok := p.Key.Data.(*js_ast.EString); ok {
											if value, ok := p.ValueOrNil.Data.(*js_ast.EString); ok {
												entries = append(entries, ast.AssertEntry{
													Key:             key.Value,
													KeyLoc:          p.Key.Loc,
													Value:           value.Value,
													ValueLoc:        p.ValueOrNil.Loc,
													PreferQuotedKey: p.PreferQuotedKey,
												})
												continue
											} else {
												why = fmt.Sprintf("the value for the property %q was not a string literal",
													js_lexer.UTF16ToString(key.Value))
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
									assertions = &entries
									why = ""
								}
							} else {
								why = "the value for \"assert\" was not an object literal"
								whyLoc = prop.ValueOrNil.Loc
							}
						} else {
							why = "this property was not called \"assert\""
							whyLoc = prop.Key.Loc
						}
					} else {
						why = "this property was invalid"
						whyLoc = prop.Key.Loc
					}
				} else {
					why = "the second argument was not an object literal with a single property called \"assert\""
					whyLoc = e.OptionsOrNil.Loc
				}
			}

			// Handle the case that isn't just an import assertion clause
			if why != "" {
				// Only warn when bundling
				if p.options.mode == config.ModeBundle {
					text := "This \"import()\" was not recognized because " + why
					kind := logger.Warning
					if p.suppressWarningsAboutWeirdCode {
						kind = logger.Debug
					}
					p.log.Add(kind, &p.tracker, logger.Range{Loc: whyLoc}, text)
				}

				// If import assertions aren't supported in the target platform, keeping
				// them would be a syntax error so we need to get rid of them. We can't
				// just not print them because they may have important side effects.
				// Attempt to discard them without changing side effects and generate an
				// error if that isn't possible.
				if p.options.unsupportedJSFeatures.Has(compat.ImportAssertions) {
					if p.exprCanBeRemovedIfUnused(e.OptionsOrNil) {
						e.OptionsOrNil = js_ast.Expr{}
					} else {
						p.markSyntaxFeature(compat.ImportAssertions, logger.Range{Loc: e.OptionsOrNil.Loc})
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

				importRecordIndex := p.addImportRecord(ast.ImportDynamic, arg.Loc, js_lexer.UTF16ToString(str.Value), assertions)
				p.importRecords[importRecordIndex].HandlesImportErrors = (isAwaitTarget && p.fnOrArrowDataVisit.tryBodyCount != 0) || isThenCatchTarget
				p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)
				return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EImportString{
					ImportRecordIndex:       importRecordIndex,
					LeadingInteriorComments: e.LeadingInteriorComments,
				}}
			}

			// Use a debug log so people can see this if they want to
			r := js_lexer.RangeOfIdentifier(p.source, expr.Loc)
			p.log.Add(logger.Debug, &p.tracker, r,
				"This \"import\" expression will not be bundled because the argument is not a string literal")

			// We need to convert this into a call to "require()" if ES6 syntax is
			// not supported in the current output format. The full conversion:
			//
			//   Before:
			//     import(foo)
			//
			//   After:
			//     Promise.resolve().then(() => require(foo))
			//
			// This is normally done by the printer since we don't know during the
			// parsing stage whether this module is external or not. However, it's
			// guaranteed to be external if the argument isn't a string. We handle
			// this case here instead of in the printer because both the printer
			// and the linker currently need an import record to handle this case
			// correctly, and you need a string literal to get an import record.
			if p.options.unsupportedJSFeatures.Has(compat.DynamicImport) {
				var then js_ast.Expr
				value := p.callRuntime(arg.Loc, "__toModule", []js_ast.Expr{{Loc: expr.Loc, Data: &js_ast.ECall{
					Target: p.valueToSubstituteForRequire(expr.Loc),
					Args:   []js_ast.Expr{arg},
				}}})
				body := js_ast.FnBody{Loc: expr.Loc, Stmts: []js_ast.Stmt{{Loc: expr.Loc, Data: &js_ast.SReturn{ValueOrNil: value}}}}
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
						}},
						Name:    "then",
						NameLoc: expr.Loc,
					}},
					Args: []js_ast.Expr{then},
				}}
			}

			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EImportCall{
				Expr:                    arg,
				OptionsOrNil:            e.OptionsOrNil,
				LeadingInteriorComments: e.LeadingInteriorComments,
			}}
		}), exprOut{}

	case *js_ast.ECall:
		p.callTarget = e.Target.Data

		// Track ".then().catch()" chains
		p.thenCatchChain = thenCatchChain{
			nextTarget:      e.Target.Data,
			hasMultipleArgs: len(e.Args) >= 2,
			hasCatch:        p.thenCatchChain.nextTarget == e && p.thenCatchChain.hasCatch,
		}

		// Prepare to recognize "require.resolve()" calls
		couldBeRequireResolve := false
		if len(e.Args) == 1 && p.options.mode != config.ModePassThrough {
			if dot, ok := e.Target.Data.(*js_ast.EDot); ok && dot.OptionalChain == js_ast.OptionalChainNone && dot.Name == "resolve" {
				couldBeRequireResolve = true
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
		for i, arg := range e.Args {
			arg = p.visitExpr(arg)
			if _, ok := arg.Data.(*js_ast.ESpread); ok {
				hasSpread = true
			}
			e.Args[i] = arg
		}

		// Recognize "require.resolve()" calls
		if couldBeRequireResolve {
			if dot, ok := e.Target.Data.(*js_ast.EDot); ok {
				if id, ok := dot.Target.Data.(*js_ast.EIdentifier); ok {
					if id.Ref == p.requireRef {
						p.ignoreUsage(p.requireRef)
						return p.maybeTransposeIfExprChain(e.Args[0], func(arg js_ast.Expr) js_ast.Expr {
							if str, ok := e.Args[0].Data.(*js_ast.EString); ok {
								// Ignore calls to require.resolve() if the control flow is provably
								// dead here. We don't want to spend time scanning the required files
								// if they will never be used.
								if p.isControlFlowDead {
									return js_ast.Expr{Loc: arg.Loc, Data: js_ast.ENullShared}
								}

								importRecordIndex := p.addImportRecord(ast.ImportRequireResolve, e.Args[0].Loc, js_lexer.UTF16ToString(str.Value), nil)
								p.importRecords[importRecordIndex].HandlesImportErrors = p.fnOrArrowDataVisit.tryBodyCount != 0
								p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)

								// Create a new expression to represent the operation
								return js_ast.Expr{Loc: arg.Loc, Data: &js_ast.ERequireResolveString{
									ImportRecordIndex: importRecordIndex,
								}}
							}

							// Otherwise just return a clone of the "require.resolve()" call
							return js_ast.Expr{Loc: arg.Loc, Data: &js_ast.ECall{
								Target: js_ast.Expr{Loc: e.Target.Loc, Data: &js_ast.EDot{
									Target:  p.valueToSubstituteForRequire(dot.Target.Loc),
									Name:    dot.Name,
									NameLoc: dot.NameLoc,
								}},
								Args: []js_ast.Expr{arg},
							}}
						}), exprOut{}
					}
				}
			}
		}

		// "foo(1, ...[2, 3], 4)" => "foo(1, 2, 3, 4)"
		if p.options.mangleSyntax && hasSpread && in.assignTarget == js_ast.AssignTargetNone {
			e.Args = inlineSpreadsOfArrayLiterals(e.Args)
		}

		// Detect if this is a direct eval. Note that "(1 ? eval : 0)(x)" will
		// become "eval(x)" after we visit the target due to dead code elimination,
		// but that doesn't mean it should become a direct eval.
		if wasIdentifierBeforeVisit {
			if id, ok := e.Target.Data.(*js_ast.EIdentifier); ok {
				if symbol := p.symbols[id.Ref.InnerIndex]; symbol.OriginalName == "eval" {
					e.IsDirectEval = true

					// Pessimistically assume that if this looks like a CommonJS module
					// (no "import" or "export" keywords), a direct call to "eval" means
					// that code could potentially access "module" or "exports".
					if p.options.mode == config.ModeBundle && !p.hasESModuleSyntax {
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
					if p.options.mode == config.ModeBundle {
						text := "Using direct eval with a bundler is not recommended and may cause problems"
						kind := logger.Debug
						if p.hasESModuleSyntax && !p.suppressWarningsAboutWeirdCode {
							kind = logger.Warning
						}
						p.log.AddWithNotes(kind, &p.tracker, js_lexer.RangeOfIdentifier(p.source, e.Target.Loc), text,
							[]logger.MsgData{{Text: "You can read more about direct eval and bundling here: https://esbuild.github.io/link/direct-eval"}})
					}
				}
			}
		}

		// Copy the call side effect flag over if this is a known target
		switch t := target.Data.(type) {
		case *js_ast.EIdentifier:
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}
		case *js_ast.EDot:
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}
		}

		// Handle parenthesized optional chains
		if isParenthesizedOptionalChain && out.thisArgFunc != nil && out.thisArgWrapFunc != nil {
			return p.lowerParenthesizedOptionalChain(expr.Loc, e, out), exprOut{}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != js_ast.OptionalChainNone
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

				if p.options.mode == config.ModeBundle {
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

								importRecordIndex := p.addImportRecord(ast.ImportRequire, arg.Loc, js_lexer.UTF16ToString(str.Value), nil)
								p.importRecords[importRecordIndex].HandlesImportErrors = p.fnOrArrowDataVisit.tryBodyCount != 0
								p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)

								// Create a new expression to represent the operation
								return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ERequireString{
									ImportRecordIndex: importRecordIndex,
								}}
							}

							// Use a debug log so people can see this if they want to
							r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
							p.log.Add(logger.Debug, &p.tracker, r,
								"This call to \"require\" will not be bundled because the argument is not a string literal")

							// Otherwise just return a clone of the "require()" call
							return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
								Target: p.valueToSubstituteForRequire(e.Target.Loc),
								Args:   []js_ast.Expr{arg},
							}}
						}), exprOut{}
					} else {
						// Use a debug log so people can see this if they want to
						r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
						p.log.AddWithNotes(logger.Debug, &p.tracker, r,
							fmt.Sprintf("This call to \"require\" will not be bundled because it has %d arguments", len(e.Args)),
							[]logger.MsgData{{Text: "To be bundled by esbuild, a \"require\" call must have exactly 1 argument."}})
					}

					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ECall{
						Target: p.valueToSubstituteForRequire(e.Target.Loc),
						Args:   e.Args,
					}}, exprOut{}
				} else if p.options.outputFormat == config.FormatESModule && !omitWarnings {
					r := js_lexer.RangeOfIdentifier(p.source, e.Target.Loc)
					p.log.Add(logger.Warning, &p.tracker, r, "Converting \"require\" to \"esm\" is currently not supported")
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
		e.Target = p.visitExpr(e.Target)
		p.warnAboutImportNamespaceCall(e.Target, exprKindNew)

		for i, arg := range e.Args {
			e.Args[i] = p.visitExpr(arg)
		}

		p.maybeMarkKnownGlobalConstructorAsPure(e)

	case *js_ast.EArrow:
		asyncArrowNeedsToBeLowered := e.IsAsync && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait)
		oldFnOrArrowData := p.fnOrArrowDataVisit
		p.fnOrArrowDataVisit = fnOrArrowDataVisit{
			isArrow:          true,
			isAsync:          e.IsAsync,
			shouldLowerSuper: oldFnOrArrowData.shouldLowerSuper || asyncArrowNeedsToBeLowered,
		}

		// Mark if we're inside an async arrow function. This value should be true
		// even if we're inside multiple arrow functions and the closest inclosing
		// arrow function isn't async, as long as at least one enclosing arrow
		// function within the current enclosing function is async.
		oldInsideAsyncArrowFn := p.fnOnlyDataVisit.isInsideAsyncArrowFn
		if e.IsAsync {
			p.fnOnlyDataVisit.isInsideAsyncArrowFn = true
		}

		// If this function is "async" and we need to lower it, also lower any
		// "super" property accesses within this function. This object will be
		// populated if "super" is used, and then any necessary helper functions
		// will be placed in the function body by "lowerFunction" below.
		var superHelpersOrNil *superHelpers
		if asyncArrowNeedsToBeLowered && p.fnOnlyDataVisit.superHelpers == nil {
			superHelpersOrNil = &superHelpers{
				getRef: js_ast.InvalidRef,
				setRef: js_ast.InvalidRef,
			}
			p.fnOnlyDataVisit.superHelpers = superHelpersOrNil
		}

		p.pushScopeForVisitPass(js_ast.ScopeFunctionArgs, expr.Loc)
		p.visitArgs(e.Args, visitArgsOpts{
			hasRestArg:               e.HasRestArg,
			body:                     e.Body.Stmts,
			isUniqueFormalParameters: true,
		})
		p.pushScopeForVisitPass(js_ast.ScopeFunctionBody, e.Body.Loc)
		e.Body.Stmts = p.visitStmtsAndPrependTempRefs(e.Body.Stmts, prependTempRefsOpts{kind: stmtsFnBody})
		p.popScope()
		p.lowerFunction(&e.IsAsync, &e.Args, e.Body.Loc, &e.Body.Stmts, &e.PreferExpr, &e.HasRestArg, true /* isArrow */, superHelpersOrNil)
		p.popScope()

		// If we intercepted "super" accesses above, clear out the helpers so they
		// don't propagate into other function calls later on.
		if superHelpersOrNil != nil {
			p.fnOnlyDataVisit.superHelpers = nil
		}

		if p.options.mangleSyntax && len(e.Body.Stmts) == 1 {
			if s, ok := e.Body.Stmts[0].Data.(*js_ast.SReturn); ok {
				if s.ValueOrNil.Data == nil {
					// "() => { return }" => "() => {}"
					e.Body.Stmts = []js_ast.Stmt{}
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
			return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EFunction{Fn: js_ast.Fn{
				Args:         e.Args,
				Body:         e.Body,
				ArgumentsRef: js_ast.InvalidRef,
				IsAsync:      e.IsAsync,
				HasRestArg:   e.HasRestArg,
			}}}, exprOut{}
		}

	case *js_ast.EFunction:
		p.visitFn(&e.Fn, expr.Loc)
		name := e.Fn.Name

		// Remove unused function names when minifying
		if p.options.mangleSyntax && !p.currentScope.ContainsDirectEval &&
			name != nil && p.symbols[name.Ref.InnerIndex].UseCountEstimate == 0 {
			e.Fn.Name = nil
		}

		// Optionally preserve the name
		if p.options.keepNames && name != nil {
			expr = p.keepExprSymbolName(expr, p.symbols[name.Ref.InnerIndex].OriginalName)
		}

	case *js_ast.EClass:
		shadowRef := p.visitClass(expr.Loc, &e.Class)

		// Lower class field syntax for browsers that don't support it
		_, expr = p.lowerClass(js_ast.Stmt{}, expr, shadowRef)

	default:
		panic("Internal error")
	}

	return expr, exprOut{}
}

func (p *parser) warnAboutImportNamespaceCall(target js_ast.Expr, kind importNamespaceCallKind) {
	if p.options.outputFormat != config.FormatPreserve {
		if id, ok := target.Data.(*js_ast.EIdentifier); ok && p.importItemsForNamespace[id.Ref] != nil {
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

			p.log.AddWithNotes(logger.Warning, &p.tracker, r, fmt.Sprintf(
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
		if symbol := p.symbols[id.Ref.InnerIndex]; symbol.Kind == js_ast.SymbolUnbound {
			switch symbol.OriginalName {
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

func (p *parser) valueForDefine(loc logger.Loc, defineFunc config.DefineFunc, opts identifierOpts) js_ast.Expr {
	expr := js_ast.Expr{Loc: loc, Data: defineFunc(config.DefineArgs{
		Loc:             loc,
		FindSymbol:      p.findSymbolHelper,
		SymbolForDefine: p.symbolForDefineHelper,
	})}
	if id, ok := expr.Data.(*js_ast.EIdentifier); ok {
		opts.wasOriginallyIdentifier = true
		return p.handleIdentifier(loc, id, opts)
	}
	return expr
}

type identifierOpts struct {
	assignTarget            js_ast.AssignTarget
	isCallTarget            bool
	isDeleteTarget          bool
	preferQuotedKey         bool
	wasOriginallyIdentifier bool
}

func (p *parser) handleIdentifier(loc logger.Loc, e *js_ast.EIdentifier, opts identifierOpts) js_ast.Expr {
	ref := e.Ref

	// Capture the "arguments" variable if necessary
	if p.fnOnlyDataVisit.argumentsRef != nil && ref == *p.fnOnlyDataVisit.argumentsRef {
		isInsideUnsupportedArrow := p.fnOrArrowDataVisit.isArrow && p.options.unsupportedJSFeatures.Has(compat.Arrow)
		isInsideUnsupportedAsyncArrow := p.fnOnlyDataVisit.isInsideAsyncArrowFn && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait)
		if isInsideUnsupportedArrow || isInsideUnsupportedAsyncArrow {
			return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: p.captureArguments()}}
		}
	}

	// Create an error for assigning to an import namespace
	if p.options.mode == config.ModeBundle &&
		(opts.assignTarget != js_ast.AssignTargetNone || opts.isDeleteTarget) &&
		p.symbols[ref.InnerIndex].Kind == js_ast.SymbolImport {
		r := js_lexer.RangeOfIdentifier(p.source, loc)

		// Try to come up with a setter name to try to make this message more understandable
		var setterHint string
		originalName := p.symbols[ref.InnerIndex].OriginalName
		if js_lexer.IsIdentifier(originalName) && originalName != "_" {
			if len(originalName) == 1 || (len(originalName) > 1 && originalName[0] < utf8.RuneSelf) {
				setterHint = fmt.Sprintf(" (e.g. \"set%s%s\")", strings.ToUpper(originalName[:1]), originalName[1:])
			} else {
				setterHint = fmt.Sprintf(" (e.g. \"set_%s\")", originalName)
			}
		}

		p.log.AddWithNotes(logger.Error, &p.tracker, r, fmt.Sprintf("Cannot assign to import %q", originalName),
			[]logger.MsgData{{Text: "Imports are immutable in JavaScript. " +
				fmt.Sprintf("To modify the value of this import, you must export a setter function in the "+
					"imported file%s and then import and call that function here instead.", setterHint)}})
	}

	// Substitute an EImportIdentifier now if this has a namespace alias
	if opts.assignTarget == js_ast.AssignTargetNone && !opts.isDeleteTarget {
		if nsAlias := p.symbols[ref.InnerIndex].NamespaceAlias; nsAlias != nil {
			data := &js_ast.EImportIdentifier{
				Ref:                     ref,
				PreferQuotedKey:         opts.preferQuotedKey,
				WasOriginallyIdentifier: opts.wasOriginallyIdentifier,
			}

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
			propertyAccess := &js_ast.EDot{
				Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: nsRef}},
				Name:    name,
				NameLoc: loc,
			}
			if p.tsNamespaceTarget == e {
				p.tsNamespaceTarget = propertyAccess
			}
			return js_ast.Expr{Loc: loc, Data: propertyAccess}
		}
	}

	// Swap references to the global "require" function with our "__require" stub
	if ref == p.requireRef && !opts.isCallTarget {
		return p.valueToSubstituteForRequire(loc)
	}

	return js_ast.Expr{Loc: loc, Data: e}
}

func extractNumericValues(left js_ast.Expr, right js_ast.Expr) (float64, float64, bool) {
	if a, ok := left.Data.(*js_ast.ENumber); ok {
		if b, ok := right.Data.(*js_ast.ENumber); ok {
			return a.Value, b.Value, true
		}
	}
	return 0, 0, false
}

func (p *parser) visitFn(fn *js_ast.Fn, scopeLoc logger.Loc) {
	oldFnOrArrowData := p.fnOrArrowDataVisit
	oldFnOnlyData := p.fnOnlyDataVisit
	p.fnOrArrowDataVisit = fnOrArrowDataVisit{
		isAsync:          fn.IsAsync,
		isGenerator:      fn.IsGenerator,
		shouldLowerSuper: fn.IsAsync && p.options.unsupportedJSFeatures.Has(compat.AsyncAwait),
	}
	p.fnOnlyDataVisit = fnOnlyDataVisit{
		isThisNested:       true,
		isNewTargetAllowed: true,
		argumentsRef:       &fn.ArgumentsRef,
	}

	// If this function is "async" and we need to lower it, also lower any
	// "super" property accesses within this function. This object will be
	// populated if "super" is used, and then any necessary helper functions
	// will be placed in the function body by "lowerFunction" below.
	if p.fnOrArrowDataVisit.shouldLowerSuper {
		p.fnOnlyDataVisit.superHelpers = &superHelpers{
			getRef: js_ast.InvalidRef,
			setRef: js_ast.InvalidRef,
		}
	}

	if fn.Name != nil {
		p.recordDeclaredSymbol(fn.Name.Ref)
	}

	p.pushScopeForVisitPass(js_ast.ScopeFunctionArgs, scopeLoc)
	p.visitArgs(fn.Args, visitArgsOpts{
		hasRestArg:               fn.HasRestArg,
		body:                     fn.Body.Stmts,
		isUniqueFormalParameters: fn.IsUniqueFormalParameters,
	})
	p.pushScopeForVisitPass(js_ast.ScopeFunctionBody, fn.Body.Loc)
	if fn.Name != nil {
		p.validateDeclaredSymbolName(fn.Name.Loc, p.symbols[fn.Name.Ref.InnerIndex].OriginalName)
	}
	fn.Body.Stmts = p.visitStmtsAndPrependTempRefs(fn.Body.Stmts, prependTempRefsOpts{fnBodyLoc: &fn.Body.Loc, kind: stmtsFnBody})
	p.popScope()
	p.lowerFunction(&fn.IsAsync, &fn.Args, fn.Body.Loc, &fn.Body.Stmts, nil, &fn.HasRestArg, false /* isArrow */, p.fnOnlyDataVisit.superHelpers)
	p.popScope()

	p.fnOrArrowDataVisit = oldFnOrArrowData
	p.fnOnlyDataVisit = oldFnOnlyData
}

func (p *parser) recordExport(loc logger.Loc, alias string, ref js_ast.Ref) {
	if name, ok := p.namedExports[alias]; ok {
		// Duplicate exports are an error
		p.log.AddWithNotes(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, loc),
			fmt.Sprintf("Multiple exports with the same name %q", alias),
			[]logger.MsgData{p.tracker.MsgData(js_lexer.RangeOfIdentifier(p.source, name.AliasLoc),
				fmt.Sprintf("The name %q was originally exported here:", alias))})
	} else if alias == "__esModule" {
		p.log.Add(logger.Error, &p.tracker, js_lexer.RangeOfIdentifier(p.source, loc),
			"The export name \"__esModule\" is reserved and cannot be used (it's needed as an export marker when converting ES module syntax to CommonJS)")
	} else {
		p.namedExports[alias] = js_ast.NamedExport{AliasLoc: loc, Ref: ref}
	}
}

func (p *parser) recordExportedBinding(binding js_ast.Binding) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		p.recordExport(binding.Loc, p.symbols[b.Ref.InnerIndex].OriginalName, b.Ref)

	case *js_ast.BArray:
		for _, item := range b.Items {
			p.recordExportedBinding(item.Binding)
		}

	case *js_ast.BObject:
		for _, item := range b.Properties {
			p.recordExportedBinding(item.Value)
		}
	default:
		panic("Internal error")
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
		if id, ok := value.Data.(*js_ast.EIdentifier); ok {
			// Is this import statement unused?
			if ref := decl.Binding.Data.(*js_ast.BIdentifier).Ref; p.symbols[ref.InnerIndex].UseCountEstimate == 0 {
				// Also don't count the referenced identifier
				p.ignoreUsage(id.Ref)

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
			keepUnusedImports := p.options.ts.Parse && p.options.unusedImportsTS == config.UnusedImportsKeepValues &&
				p.options.mode != config.ModeBundle && !p.options.minifyIdentifiers

			// TypeScript always trims unused imports. This is important for
			// correctness since some imports might be fake (only in the type
			// system and used for type-only imports).
			if (p.options.mangleSyntax || p.options.ts.Parse) && !keepUnusedImports {
				foundImports := false
				isUnusedInTypeScript := true

				// Remove the default name if it's unused
				if s.DefaultName != nil {
					foundImports = true
					symbol := p.symbols[s.DefaultName.Ref.InnerIndex]

					// TypeScript has a separate definition of unused
					if p.options.ts.Parse && p.tsUseCounts[s.DefaultName.Ref.InnerIndex] != 0 {
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
					if p.options.ts.Parse && p.tsUseCounts[s.NamespaceRef.InnerIndex] != 0 {
						isUnusedInTypeScript = false
					}

					// Remove the symbol if it's never used outside a dead code region
					if symbol.UseCountEstimate == 0 && (p.options.ts.Parse || !p.moduleScope.ContainsDirectEval) {
						// Make sure we don't remove this if it was used for a property
						// access while bundling
						if importItems, ok := p.importItemsForNamespace[s.NamespaceRef]; ok && len(importItems) == 0 {
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
						if p.options.ts.Parse && p.tsUseCounts[item.Name.Ref.InnerIndex] != 0 {
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
				if p.options.ts.Parse && foundImports && isUnusedInTypeScript && p.options.unusedImportsTS == config.UnusedImportsRemoveStmt {
					// Ignore import records with a pre-filled source index. These are
					// for injected files and we definitely do not want to trim these.
					if !record.SourceIndex.IsValid() {
						record.IsUnused = true
						continue
					}
				}
			}

			if p.options.mode != config.ModePassThrough {
				if s.StarNameLoc != nil {
					// "importItemsForNamespace" has property accesses off the namespace
					if importItems, ok := p.importItemsForNamespace[s.NamespaceRef]; ok && len(importItems) > 0 {
						// Sort keys for determinism
						sorted := make([]string, 0, len(importItems))
						for alias := range importItems {
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
							name := importItems[alias]
							p.namedImports[name.Ref] = js_ast.NamedImport{
								Alias:             alias,
								AliasLoc:          name.Loc,
								NamespaceRef:      s.NamespaceRef,
								ImportRecordIndex: s.ImportRecordIndex,
							}

							// Make sure the printer prints this as a property access
							p.symbols[name.Ref.InnerIndex].NamespaceAlias = &js_ast.NamespaceAlias{
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
						NamespaceRef:      js_ast.InvalidRef,
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
				record.ContainsImportStar = true
			}

			if s.DefaultName != nil {
				record.ContainsDefaultAlias = true
			} else if s.Items != nil {
				for _, item := range *s.Items {
					if item.Alias == "default" {
						record.ContainsDefaultAlias = true
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
				for _, decl := range s.Decls {
					p.recordExportedBinding(decl.Binding)
				}
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
			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			if s.Alias != nil {
				// "export * as ns from 'path'"
				p.namedImports[s.NamespaceRef] = js_ast.NamedImport{
					AliasIsStar:       true,
					AliasLoc:          s.Alias.Loc,
					NamespaceRef:      js_ast.InvalidRef,
					ImportRecordIndex: s.ImportRecordIndex,
					IsExported:        true,
				}
				p.recordExport(s.Alias.Loc, s.Alias.OriginalName, s.NamespaceRef)
			} else {
				// "export * from 'path'"
				p.exportStarImportRecords = append(p.exportStarImportRecords, s.ImportRecordIndex)
			}

		case *js_ast.SExportFrom:
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
	p.symbolUses = make(map[js_ast.Ref]js_ast.SymbolUse)
	p.declaredSymbols = nil
	p.importRecordsForCurrentPart = nil
	p.scopesForCurrentPart = nil

	part := js_ast.Part{
		Stmts:      p.visitStmtsAndPrependTempRefs(stmts, prependTempRefsOpts{}),
		SymbolUses: p.symbolUses,
	}

	// Insert any relocated variable statements now
	if len(p.relocatedTopLevelVars) > 0 {
		alreadyDeclared := make(map[js_ast.Ref]bool)
		for _, local := range p.relocatedTopLevelVars {
			// Follow links because "var" declarations may be merged due to hoisting
			for {
				link := p.symbols[local.Ref.InnerIndex].Link
				if link == js_ast.InvalidRef {
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
		part.CanBeRemovedIfUnused = p.stmtsCanBeRemovedIfUnused(part.Stmts)
		part.DeclaredSymbols = p.declaredSymbols
		part.ImportRecordIndices = p.importRecordsForCurrentPart
		part.Scopes = p.scopesForCurrentPart
		parts = append(parts, part)
	}
	return parts
}

func (p *parser) stmtsCanBeRemovedIfUnused(stmts []js_ast.Stmt) bool {
	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SFunction, *js_ast.SEmpty:
			// These never have side effects

		case *js_ast.SImport:
			// Let these be removed if they are unused. Note that we also need to
			// check if the imported file is marked as "sideEffects: false" before we
			// can remove a SImport statement. Otherwise the import must be kept for
			// its side effects.

		case *js_ast.SClass:
			if !p.classCanBeRemovedIfUnused(s.Class) {
				return false
			}

		case *js_ast.SExpr:
			if s.DoesNotAffectTreeShaking {
				// Expressions marked with this are automatically generated and have
				// no side effects by construction.
				break
			}

			if !p.exprCanBeRemovedIfUnused(s.Value) {
				return false
			}

		case *js_ast.SLocal:
			for _, decl := range s.Decls {
				if !p.bindingCanBeRemovedIfUnused(decl.Binding) {
					return false
				}
				if decl.ValueOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(decl.ValueOrNil) {
					return false
				}
			}

		case *js_ast.STry:
			if !p.stmtsCanBeRemovedIfUnused(s.Body) || (s.Finally != nil && !p.stmtsCanBeRemovedIfUnused(s.Finally.Stmts)) {
				return false
			}

		case *js_ast.SExportFrom:
			// Exports are tracked separately, so this isn't necessary

		case *js_ast.SExportClause:
			// Exports are tracked separately, so this isn't necessary. Except we
			// should keep all of these statements if we're not doing any format
			// conversion, because exports are not re-emitted in that case.
			if p.options.mode == config.ModePassThrough {
				return false
			}

		case *js_ast.SExportDefault:
			switch s2 := s.Value.Data.(type) {
			case *js_ast.SExpr:
				if !p.exprCanBeRemovedIfUnused(s2.Value) {
					return false
				}

			case *js_ast.SFunction:
				// These never have side effects

			case *js_ast.SClass:
				if !p.classCanBeRemovedIfUnused(s2.Class) {
					return false
				}

			default:
				panic("Internal error")
			}

		default:
			// Assume that all statements not explicitly special-cased here have side
			// effects, and cannot be removed even if unused
			return false
		}
	}

	return true
}

func (p *parser) classCanBeRemovedIfUnused(class js_ast.Class) bool {
	if class.ExtendsOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(class.ExtendsOrNil) {
		return false
	}

	for _, property := range class.Properties {
		if property.Kind == js_ast.PropertyClassStaticBlock {
			if !p.stmtsCanBeRemovedIfUnused(property.ClassStaticBlock.Stmts) {
				return false
			}
			continue
		}
		if !p.exprCanBeRemovedIfUnused(property.Key) {
			return false
		}
		if property.ValueOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(property.ValueOrNil) {
			return false
		}
		if property.InitializerOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(property.InitializerOrNil) {
			return false
		}
	}

	return true
}

func (p *parser) bindingCanBeRemovedIfUnused(binding js_ast.Binding) bool {
	switch b := binding.Data.(type) {
	case *js_ast.BArray:
		for _, item := range b.Items {
			if !p.bindingCanBeRemovedIfUnused(item.Binding) {
				return false
			}
			if item.DefaultValueOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(item.DefaultValueOrNil) {
				return false
			}
		}

	case *js_ast.BObject:
		for _, property := range b.Properties {
			if !property.IsSpread && !p.exprCanBeRemovedIfUnused(property.Key) {
				return false
			}
			if !p.bindingCanBeRemovedIfUnused(property.Value) {
				return false
			}
			if property.DefaultValueOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(property.DefaultValueOrNil) {
				return false
			}
		}
	}

	return true
}

func (p *parser) exprCanBeRemovedIfUnused(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EInlinedEnum:
		return p.exprCanBeRemovedIfUnused(e.Value)

	case *js_ast.ENull, *js_ast.EUndefined, *js_ast.EMissing, *js_ast.EBoolean, *js_ast.ENumber, *js_ast.EBigInt,
		*js_ast.EString, *js_ast.EThis, *js_ast.ERegExp, *js_ast.EFunction, *js_ast.EArrow, *js_ast.EImportMeta:
		return true

	case *js_ast.EDot:
		return e.CanBeRemovedIfUnused

	case *js_ast.EClass:
		return p.classCanBeRemovedIfUnused(e.Class)

	case *js_ast.EIdentifier:
		if e.MustKeepDueToWithStmt {
			return false
		}

		// Unbound identifiers cannot be removed because they can have side effects.
		// One possible side effect is throwing a ReferenceError if they don't exist.
		// Another one is a getter with side effects on the global object:
		//
		//   Object.defineProperty(globalThis, 'x', {
		//     get() {
		//       sideEffect();
		//     },
		//   });
		//
		// Be very careful about this possibility. It's tempting to treat all
		// identifier expressions as not having side effects but that's wrong. We
		// must make sure they have been declared by the code we are currently
		// compiling before we can tell that they have no side effects.
		//
		// Note that we currently ignore ReferenceErrors due to TDZ access. This is
		// incorrect but proper TDZ analysis is very complicated and would have to
		// be very conservative, which would inhibit a lot of optimizations of code
		// inside closures. This may need to be revisited if it proves problematic.
		if e.CanBeRemovedIfUnused || p.symbols[e.Ref.InnerIndex].Kind != js_ast.SymbolUnbound {
			return true
		}

	case *js_ast.EImportIdentifier:
		// References to an ES6 import item are always side-effect free in an
		// ECMAScript environment.
		//
		// They could technically have side effects if the imported module is a
		// CommonJS module and the import item was translated to a property access
		// (which esbuild's bundler does) and the property has a getter with side
		// effects.
		//
		// But this is very unlikely and respecting this edge case would mean
		// disabling tree shaking of all code that references an export from a
		// CommonJS module. It would also likely violate the expectations of some
		// developers because the code *looks* like it should be able to be tree
		// shaken.
		//
		// So we deliberately ignore this edge case and always treat import item
		// references as being side-effect free.
		return true

	case *js_ast.EIf:
		return p.exprCanBeRemovedIfUnused(e.Test) &&
			((p.isSideEffectFreeUnboundIdentifierRef(e.Yes, e.Test, true) || p.exprCanBeRemovedIfUnused(e.Yes)) &&
				(p.isSideEffectFreeUnboundIdentifierRef(e.No, e.Test, false) || p.exprCanBeRemovedIfUnused(e.No)))

	case *js_ast.EArray:
		for _, item := range e.Items {
			if !p.exprCanBeRemovedIfUnused(item) {
				return false
			}
		}
		return true

	case *js_ast.EObject:
		for _, property := range e.Properties {
			// The key must still be evaluated if it's computed or a spread
			if property.Kind == js_ast.PropertySpread || property.IsComputed {
				return false
			}
			if property.ValueOrNil.Data != nil && !p.exprCanBeRemovedIfUnused(property.ValueOrNil) {
				return false
			}
		}
		return true

	case *js_ast.ECall:
		// A call that has been marked "__PURE__" can be removed if all arguments
		// can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			for _, arg := range e.Args {
				if !p.exprCanBeRemovedIfUnused(arg) {
					return false
				}
			}
			return true
		}

	case *js_ast.ENew:
		// A constructor call that has been marked "__PURE__" can be removed if all
		// arguments can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			for _, arg := range e.Args {
				if !p.exprCanBeRemovedIfUnused(arg) {
					return false
				}
			}
			return true
		}

	case *js_ast.EUnary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case js_ast.UnOpVoid, js_ast.UnOpNot:
			return p.exprCanBeRemovedIfUnused(e.Value)

		// The "typeof" operator doesn't do any type conversions so it can be removed
		// if the result is unused and the operand has no side effects. However, it
		// has a special case where if the operand is an identifier expression such
		// as "typeof x" and "x" doesn't exist, no reference error is thrown so the
		// operation has no side effects.
		//
		// Note that there *is* actually a case where "typeof x" can throw an error:
		// when "x" is being referenced inside of its TDZ (temporal dead zone). TDZ
		// checks are not yet handled correctly by esbuild, so this possibility is
		// currently ignored.
		case js_ast.UnOpTypeof:
			if _, ok := e.Value.Data.(*js_ast.EIdentifier); ok {
				return true
			}
			return p.exprCanBeRemovedIfUnused(e.Value)
		}

	case *js_ast.EBinary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case js_ast.BinOpStrictEq, js_ast.BinOpStrictNe, js_ast.BinOpComma, js_ast.BinOpNullishCoalescing:
			return p.exprCanBeRemovedIfUnused(e.Left) && p.exprCanBeRemovedIfUnused(e.Right)

		// Special-case "||" to make sure "typeof x === 'undefined' || x" can be removed
		case js_ast.BinOpLogicalOr:
			return p.exprCanBeRemovedIfUnused(e.Left) &&
				(p.isSideEffectFreeUnboundIdentifierRef(e.Right, e.Left, false) || p.exprCanBeRemovedIfUnused(e.Right))

		// Special-case "&&" to make sure "typeof x !== 'undefined' && x" can be removed
		case js_ast.BinOpLogicalAnd:
			return p.exprCanBeRemovedIfUnused(e.Left) &&
				(p.isSideEffectFreeUnboundIdentifierRef(e.Right, e.Left, true) || p.exprCanBeRemovedIfUnused(e.Right))

		// For "==" and "!=", pretend the operator was actually "===" or "!==". If
		// we know that we can convert it to "==" or "!=", then we can consider the
		// operator itself to have no side effects. This matters because our mangle
		// logic will convert "typeof x === 'object'" into "typeof x == 'object'"
		// and since "typeof x === 'object'" is considered to be side-effect free,
		// we must also consider "typeof x == 'object'" to be side-effect free.
		case js_ast.BinOpLooseEq, js_ast.BinOpLooseNe:
			return canChangeStrictToLoose(e.Left, e.Right) && p.exprCanBeRemovedIfUnused(e.Left) && p.exprCanBeRemovedIfUnused(e.Right)
		}
	}

	// Assume all other expression types have side effects and cannot be removed
	return false
}

func (p *parser) isSideEffectFreeUnboundIdentifierRef(value js_ast.Expr, guardCondition js_ast.Expr, isYesBranch bool) bool {
	if id, ok := value.Data.(*js_ast.EIdentifier); ok && p.symbols[id.Ref.InnerIndex].Kind == js_ast.SymbolUnbound {
		if binary, ok := guardCondition.Data.(*js_ast.EBinary); ok {
			switch binary.Op {
			case js_ast.BinOpStrictEq, js_ast.BinOpStrictNe, js_ast.BinOpLooseEq, js_ast.BinOpLooseNe:
				// Pattern match for "typeof x !== <string>"
				typeof, string := binary.Left, binary.Right
				if _, ok := typeof.Data.(*js_ast.EString); ok {
					typeof, string = string, typeof
				}
				if typeof, ok := typeof.Data.(*js_ast.EUnary); ok && typeof.Op == js_ast.UnOpTypeof {
					if text, ok := string.Data.(*js_ast.EString); ok {
						// In "typeof x !== 'undefined' ? x : null", the reference to "x" is side-effect free
						// In "typeof x === 'object' ? x : null", the reference to "x" is side-effect free
						if (js_lexer.UTF16EqualsString(text.Value, "undefined") == isYesBranch) ==
							(binary.Op == js_ast.BinOpStrictNe || binary.Op == js_ast.BinOpLooseNe) {
							if id2, ok := typeof.Value.Data.(*js_ast.EIdentifier); ok && id2.Ref == id.Ref {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// This will return a nil expression if the expression can be totally removed
func (p *parser) simplifyUnusedExpr(expr js_ast.Expr) js_ast.Expr {
	switch e := expr.Data.(type) {
	case *js_ast.EInlinedEnum:
		return p.simplifyUnusedExpr(e.Value)

	case *js_ast.ENull, *js_ast.EUndefined, *js_ast.EMissing, *js_ast.EBoolean, *js_ast.ENumber, *js_ast.EBigInt,
		*js_ast.EString, *js_ast.EThis, *js_ast.ERegExp, *js_ast.EFunction, *js_ast.EArrow, *js_ast.EImportMeta:
		return js_ast.Expr{}

	case *js_ast.EDot:
		if e.CanBeRemovedIfUnused {
			return js_ast.Expr{}
		}

	case *js_ast.EIdentifier:
		if e.MustKeepDueToWithStmt {
			break
		}
		if e.CanBeRemovedIfUnused || p.symbols[e.Ref.InnerIndex].Kind != js_ast.SymbolUnbound {
			return js_ast.Expr{}
		}

	case *js_ast.ETemplate:
		if e.TagOrNil.Data == nil {
			var result js_ast.Expr
			for _, part := range e.Parts {
				// Make sure "ToString" is still evaluated on the value
				if result.Data == nil {
					result = js_ast.Expr{Loc: part.Value.Loc, Data: &js_ast.EString{}}
				}
				result = js_ast.Expr{Loc: part.Value.Loc, Data: &js_ast.EBinary{
					Op:    js_ast.BinOpAdd,
					Left:  result,
					Right: part.Value,
				}}
			}
			return result
		}

	case *js_ast.EArray:
		// Arrays with "..." spread expressions can't be unwrapped because the
		// "..." triggers code evaluation via iterators. In that case, just trim
		// the other items instead and leave the array expression there.
		for _, spread := range e.Items {
			if _, ok := spread.Data.(*js_ast.ESpread); ok {
				end := 0
				for _, item := range e.Items {
					item = p.simplifyUnusedExpr(item)
					if item.Data != nil {
						e.Items[end] = item
						end++
					}
				}
				e.Items = e.Items[:end]
				return expr
			}
		}

		// Otherwise, the array can be completely removed. We only need to keep any
		// array items with side effects. Apply this simplification recursively.
		var result js_ast.Expr
		for _, item := range e.Items {
			result = js_ast.JoinWithComma(result, p.simplifyUnusedExpr(item))
		}
		return result

	case *js_ast.EObject:
		// Objects with "..." spread expressions can't be unwrapped because the
		// "..." triggers code evaluation via getters. In that case, just trim
		// the other items instead and leave the object expression there.
		for _, spread := range e.Properties {
			if spread.Kind == js_ast.PropertySpread {
				end := 0
				for _, property := range e.Properties {
					// Spread properties must always be evaluated
					if property.Kind != js_ast.PropertySpread {
						value := p.simplifyUnusedExpr(property.ValueOrNil)
						if value.Data != nil {
							// Keep the value
							property.ValueOrNil = value
						} else if !property.IsComputed {
							// Skip this property if the key doesn't need to be computed
							continue
						} else {
							// Replace values without side effects with "0" because it's short
							property.ValueOrNil.Data = &js_ast.ENumber{}
						}
					}
					e.Properties[end] = property
					end++
				}
				e.Properties = e.Properties[:end]
				return expr
			}
		}

		// Otherwise, the object can be completely removed. We only need to keep any
		// object properties with side effects. Apply this simplification recursively.
		var result js_ast.Expr
		for _, property := range e.Properties {
			if property.IsComputed {
				// Make sure "ToString" is still evaluated on the key
				result = js_ast.JoinWithComma(result, js_ast.Expr{Loc: property.Key.Loc, Data: &js_ast.EBinary{
					Op:    js_ast.BinOpAdd,
					Left:  property.Key,
					Right: js_ast.Expr{Loc: property.Key.Loc, Data: &js_ast.EString{}},
				}})
			}
			result = js_ast.JoinWithComma(result, p.simplifyUnusedExpr(property.ValueOrNil))
		}
		return result

	case *js_ast.EIf:
		e.Yes = p.simplifyUnusedExpr(e.Yes)
		e.No = p.simplifyUnusedExpr(e.No)

		// "foo() ? 1 : 2" => "foo()"
		if e.Yes.Data == nil && e.No.Data == nil {
			return p.simplifyUnusedExpr(e.Test)
		}

		// "foo() ? 1 : bar()" => "foo() || bar()"
		if e.Yes.Data == nil {
			return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalOr, e.Test, e.No)
		}

		// "foo() ? bar() : 2" => "foo() && bar()"
		if e.No.Data == nil {
			return js_ast.JoinWithLeftAssociativeOp(js_ast.BinOpLogicalAnd, e.Test, e.Yes)
		}

	case *js_ast.EUnary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case js_ast.UnOpVoid, js_ast.UnOpNot:
			return p.simplifyUnusedExpr(e.Value)

		case js_ast.UnOpTypeof:
			if _, ok := e.Value.Data.(*js_ast.EIdentifier); ok {
				// "typeof x" must not be transformed into if "x" since doing so could
				// cause an exception to be thrown. Instead we can just remove it since
				// "typeof x" is special-cased in the standard to never throw.
				return js_ast.Expr{}
			}
			return p.simplifyUnusedExpr(e.Value)
		}

	case *js_ast.EBinary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case js_ast.BinOpStrictEq, js_ast.BinOpStrictNe, js_ast.BinOpComma:
			return js_ast.JoinWithComma(p.simplifyUnusedExpr(e.Left), p.simplifyUnusedExpr(e.Right))

		// We can simplify "==" and "!=" even though they can call "toString" and/or
		// "valueOf" if we can statically determine that the types of both sides are
		// primitives. In that case there won't be any chance for user-defined
		// "toString" and/or "valueOf" to be called.
		case js_ast.BinOpLooseEq, js_ast.BinOpLooseNe:
			if isPrimitiveWithSideEffects(e.Left.Data) && isPrimitiveWithSideEffects(e.Right.Data) {
				return js_ast.JoinWithComma(p.simplifyUnusedExpr(e.Left), p.simplifyUnusedExpr(e.Right))
			}

		case js_ast.BinOpLogicalAnd, js_ast.BinOpLogicalOr, js_ast.BinOpNullishCoalescing:
			// Preserve short-circuit behavior: the left expression is only unused if
			// the right expression can be completely removed. Otherwise, the left
			// expression is important for the branch.
			e.Right = p.simplifyUnusedExpr(e.Right)
			if e.Right.Data == nil {
				return p.simplifyUnusedExpr(e.Left)
			}

		case js_ast.BinOpAdd:
			if result, isStringAddition := simplifyUnusedStringAdditionChain(expr); isStringAddition {
				return result
			}
		}

	case *js_ast.ECall:
		// A call that has been marked "__PURE__" can be removed if all arguments
		// can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			expr = js_ast.Expr{}
			for _, arg := range e.Args {
				expr = js_ast.JoinWithComma(expr, p.simplifyUnusedExpr(arg))
			}
		}

	case *js_ast.ENew:
		// A constructor call that has been marked "__PURE__" can be removed if all
		// arguments can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			expr = js_ast.Expr{}
			for _, arg := range e.Args {
				expr = js_ast.JoinWithComma(expr, p.simplifyUnusedExpr(arg))
			}
		}
	}

	return expr
}

func simplifyUnusedStringAdditionChain(expr js_ast.Expr) (js_ast.Expr, bool) {
	switch e := expr.Data.(type) {
	case *js_ast.EString:
		// "'x' + y" => "'' + y"
		return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{}}, true

	case *js_ast.EBinary:
		if e.Op == js_ast.BinOpAdd {
			left, leftIsStringAddition := simplifyUnusedStringAdditionChain(e.Left)
			e.Left = left

			if _, rightIsString := e.Right.Data.(*js_ast.EString); rightIsString {
				// "('' + x) + 'y'" => "'' + x"
				if leftIsStringAddition {
					return left, true
				}

				// "x + 'y'" => "x + ''"
				if !leftIsStringAddition {
					e.Right.Data = &js_ast.EString{}
					return expr, true
				}
			}

			return expr, leftIsStringAddition
		}
	}

	return expr, false
}

func newParser(log logger.Log, source logger.Source, lexer js_lexer.Lexer, options *Options) *parser {
	if options.defines == nil {
		defaultDefines := config.ProcessDefines(nil)
		options.defines = &defaultDefines
	}

	p := &parser{
		log:               log,
		source:            source,
		tracker:           logger.MakeLineColumnTracker(&source),
		lexer:             lexer,
		allowIn:           true,
		options:           *options,
		runtimeImports:    make(map[string]js_ast.Ref),
		promiseRef:        js_ast.InvalidRef,
		afterArrowBodyLoc: logger.Loc{Start: -1},

		// For lowering private methods
		weakMapRef:     js_ast.InvalidRef,
		weakSetRef:     js_ast.InvalidRef,
		privateGetters: make(map[js_ast.Ref]js_ast.Ref),
		privateSetters: make(map[js_ast.Ref]js_ast.Ref),

		// These are for TypeScript
		refToTSNamespaceMemberData: make(map[js_ast.Ref]js_ast.TSNamespaceMemberData),
		emittedNamespaceVars:       make(map[js_ast.Ref]bool),
		isExportedInsideNamespace:  make(map[js_ast.Ref]js_ast.Ref),
		localTypeNames:             make(map[string]bool),

		// These are for handling ES6 imports and exports
		importItemsForNamespace: make(map[js_ast.Ref]map[string]js_ast.LocRef),
		isImportItem:            make(map[js_ast.Ref]bool),
		namedImports:            make(map[js_ast.Ref]js_ast.NamedImport),
		namedExports:            make(map[string]js_ast.NamedExport),

		suppressWarningsAboutWeirdCode: helpers.IsInsideNodeModules(source.KeyPath.Text),
	}

	p.findSymbolHelper = func(loc logger.Loc, name string) js_ast.Ref {
		return p.findSymbol(loc, name).ref
	}

	p.symbolForDefineHelper = func(index int) js_ast.Ref {
		ref := p.injectedDefineSymbols[index]
		p.recordUsage(ref)
		return ref
	}

	p.pushScopeForParsePass(js_ast.ScopeEntry, logger.Loc{Start: locModuleScope})

	return p
}

var defaultJSXFactory = []string{"React", "createElement"}
var defaultJSXFragment = []string{"React", "Fragment"}

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
		options.jsx.Factory = config.JSXExpr{Parts: defaultJSXFactory}
	}
	if len(options.jsx.Fragment.Parts) == 0 && options.jsx.Fragment.Constant == nil {
		options.jsx.Fragment = config.JSXExpr{Parts: defaultJSXFragment}
	}

	if !options.ts.Parse {
		// Non-TypeScript files always get the real JavaScript class field behavior
		options.useDefineForClassFields = config.True
	} else if options.useDefineForClassFields == config.Unspecified {
		// The default behavior for TypeScript files depends on the value of the
		// "target" field and on the version of TypeScript:
		//
		//   * TypeScript ≥4.3 and "target": "ESNext" => "useDefineForClassFields": true
		//   * Otherwise => "useDefineForClassFields": false
		//
		// Context: https://github.com/microsoft/TypeScript/pull/42663. This was
		// silently changed in TypeScript 4.3. It's a breaking change even though
		// it wasn't mentioned in the announcement blog post for TypeScript 4.3:
		// https://devblogs.microsoft.com/typescript/announcing-typescript-4-3/.
		if options.targetFromAPI == config.TargetWasConfiguredIncludingESNext ||
			(options.tsTarget != nil && strings.EqualFold(options.tsTarget.Target, "ESNext")) {
			options.useDefineForClassFields = config.True
		} else {
			options.useDefineForClassFields = config.False
		}
	}

	// If there is no top-level esbuild "target" setting, include unsupported
	// JavaScript features from the TypeScript "target" setting. Otherwise the
	// TypeScript "target" setting is ignored.
	if options.targetFromAPI == config.TargetWasUnconfigured && options.tsTarget != nil {
		options.unsupportedJSFeatures |= options.tsTarget.UnsupportedJSFeatures
	}

	p := newParser(log, source, js_lexer.NewLexer(log, source), &options)

	// Consume a leading hashbang comment
	hashbang := ""
	if p.lexer.Token == js_lexer.THashbang {
		hashbang = p.lexer.Identifier
		p.lexer.Next()
	}

	// Allow top-level await
	p.fnOrArrowDataParse.await = allowExpr
	p.fnOrArrowDataParse.isTopLevel = true

	// Parse the file in the first pass, but do not bind symbols
	stmts := p.parseStmtsUpTo(js_lexer.TEndOfFile, parseStmtOpts{isModuleScope: true})
	p.prepareForVisitPass()

	// Strip off a leading "use strict" directive when not bundling
	directive := ""
	if p.options.mode != config.ModeBundle {
		for i, stmt := range stmts {
			switch s := stmt.Data.(type) {
			case *js_ast.SComment:
				continue
			case *js_ast.SDirective:
				if !isDirectiveSupported(s) {
					continue
				}
				directive = js_lexer.UTF16ToString(s.Value)

				// Remove this directive from the statement list
				copy(stmts[1:], stmts[:i])
				stmts = stmts[1:]
			}
			break
		}
	}

	// Insert a variable for "import.meta" at the top of the file if it was used.
	// We don't need to worry about "use strict" directives because this only
	// happens when bundling, in which case we are flatting the module scopes of
	// all modules together anyway so such directives are meaningless.
	if p.importMetaRef != js_ast.InvalidRef {
		importMetaStmt := js_ast.Stmt{Data: &js_ast.SLocal{
			Kind: p.selectLocalKind(js_ast.LocalConst),
			Decls: []js_ast.Decl{{
				Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: p.importMetaRef}},
				ValueOrNil: js_ast.Expr{Data: &js_ast.EObject{}},
			}},
		}}
		stmts = append(append(make([]js_ast.Stmt, 0, len(stmts)+1), importMetaStmt), stmts...)
	}

	// Add an empty part for the namespace export that we can fill in later
	nsExportPart := js_ast.Part{
		SymbolUses:           make(map[js_ast.Ref]js_ast.SymbolUse),
		CanBeRemovedIfUnused: true,
	}

	var before = []js_ast.Part{nsExportPart}
	var parts []js_ast.Part
	var after []js_ast.Part

	// Insert any injected import statements now that symbols have been declared
	for _, file := range p.options.injectedFiles {
		exportsNoConflict := make([]string, 0, len(file.Exports))
		symbols := make(map[string]js_ast.Ref)
		if file.DefineName != "" {
			ref := p.newSymbol(js_ast.SymbolOther, file.DefineName)
			p.moduleScope.Generated = append(p.moduleScope.Generated, ref)
			symbols["default"] = ref
			exportsNoConflict = append(exportsNoConflict, "default")
			p.injectedDefineSymbols = append(p.injectedDefineSymbols, ref)
		} else {
			for _, export := range file.Exports {
				if _, ok := p.moduleScope.Members[export.Alias]; !ok {
					ref := p.newSymbol(js_ast.SymbolInjected, export.Alias)
					p.moduleScope.Members[export.Alias] = js_ast.ScopeMember{Ref: ref}
					symbols[export.Alias] = ref
					exportsNoConflict = append(exportsNoConflict, export.Alias)
					if p.injectedSymbolSources == nil {
						p.injectedSymbolSources = make(map[js_ast.Ref]injectedSymbolSource)
					}
					p.injectedSymbolSources[ref] = injectedSymbolSource{
						source: file.Source,
						loc:    export.Loc,
					}
				}
			}
		}
		before = p.generateImportStmt(file.Source.KeyPath.Text, exportsNoConflict, file.Source.Index, before, symbols)
	}

	// Bind symbols in a second pass over the AST. I started off doing this in a
	// single pass, but it turns out it's pretty much impossible to do this
	// correctly while handling arrow functions because of the grammar
	// ambiguities.
	if !p.options.treeShaking {
		// When tree shaking is disabled, everything comes in a single part
		parts = p.appendPart(parts, stmts)
	} else {
		// When tree shaking is enabled, each top-level statement is potentially a separate part
		for _, stmt := range stmts {
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

			default:
				parts = p.appendPart(parts, []js_ast.Stmt{stmt})
			}
		}
	}

	// Pop the module scope to apply the "ContainsDirectEval" rules
	p.popScope()

	parts = append(append(before, parts...), after...)
	result = p.toAST(parts, hashbang, directive)
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
		p.symbolUses = make(map[js_ast.Ref]js_ast.SymbolUse)
		expr = p.callRuntime(expr.Loc, apiCall, []js_ast.Expr{expr})
	}

	// Add an empty part for the namespace export that we can fill in later
	nsExportPart := js_ast.Part{
		SymbolUses:           make(map[js_ast.Ref]js_ast.SymbolUse),
		CanBeRemovedIfUnused: true,
	}

	// Defer the actual code generation until linking
	part := js_ast.Part{
		Stmts:      []js_ast.Stmt{{Loc: expr.Loc, Data: &js_ast.SLazyExport{Value: expr}}},
		SymbolUses: p.symbolUses,
	}
	p.symbolUses = nil

	ast := p.toAST([]js_ast.Part{nsExportPart, part}, "", "")
	ast.HasLazyExport = true
	return ast
}

type JSXExprKind uint8

const (
	JSXFactory JSXExprKind = iota
	JSXFragment
)

func ParseJSXExpr(text string, kind JSXExprKind) (config.JSXExpr, bool) {
	if text == "" {
		return config.JSXExpr{}, true
	}

	// Try a property chain
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !js_lexer.IsIdentifier(part) {
			parts = nil
			break
		}
	}
	if parts != nil {
		return config.JSXExpr{Parts: parts}, true
	}

	if kind == JSXFragment {
		// Try a JSON value
		source := logger.Source{Contents: text}
		expr, ok := ParseJSON(logger.NewDeferLog(logger.DeferLogAll), source, JSONOptions{})
		if !ok {
			return config.JSXExpr{}, false
		}

		// Only primitives are supported for now
		switch expr.Data.(type) {
		case *js_ast.ENull, *js_ast.EBoolean, *js_ast.EString, *js_ast.ENumber:
			return config.JSXExpr{Constant: expr.Data}, true
		}
	}

	return config.JSXExpr{}, false
}

// Say why this the current file is being considered an ES module
func (p *parser) whyESModule() (notes []logger.MsgData) {
	var where logger.Range
	switch {
	case p.es6ImportKeyword.Len > 0:
		where = p.es6ImportKeyword
	case p.es6ExportKeyword.Len > 0:
		where = p.es6ExportKeyword
	case p.topLevelAwaitKeyword.Len > 0:
		where = p.topLevelAwaitKeyword
	}
	if where.Len > 0 {
		notes = []logger.MsgData{p.tracker.MsgData(where,
			fmt.Sprintf("This file is considered an ECMAScript module because of the %q keyword here:", p.source.TextForRange(where)))}
	}
	return
}

func (p *parser) prepareForVisitPass() {
	p.pushScopeForVisitPass(js_ast.ScopeEntry, logger.Loc{Start: locModuleScope})
	p.fnOrArrowDataVisit.isOutsideFnOrArrow = true
	p.moduleScope = p.currentScope
	p.hasESModuleSyntax = p.es6ImportKeyword.Len > 0 || p.es6ExportKeyword.Len > 0 || p.topLevelAwaitKeyword.Len > 0

	// Legacy HTML comments are not allowed in ESM files
	if p.hasESModuleSyntax && p.lexer.LegacyHTMLCommentRange.Len > 0 {
		p.log.AddWithNotes(logger.Error, &p.tracker, p.lexer.LegacyHTMLCommentRange,
			"Legacy HTML single-line comments are not allowed in ECMAScript modules", p.whyESModule())
	}

	// ECMAScript modules are always interpreted as strict mode. This has to be
	// done before "hoistSymbols" because strict mode can alter hoisting (!).
	if p.es6ImportKeyword.Len > 0 {
		p.moduleScope.RecursiveSetStrictMode(js_ast.ImplicitStrictModeImport)
	} else if p.es6ExportKeyword.Len > 0 {
		p.moduleScope.RecursiveSetStrictMode(js_ast.ImplicitStrictModeExport)
	} else if p.topLevelAwaitKeyword.Len > 0 {
		p.moduleScope.RecursiveSetStrictMode(js_ast.ImplicitStrictModeTopLevelAwait)
	}

	p.hoistSymbols(p.moduleScope)

	if p.options.mode != config.ModePassThrough {
		p.requireRef = p.declareCommonJSSymbol(js_ast.SymbolUnbound, "require")
	} else {
		p.requireRef = p.newSymbol(js_ast.SymbolUnbound, "require")
	}

	// CommonJS-style exports are only enabled if this isn't using ECMAScript-
	// style exports. You can still use "require" in ESM, just not "module" or
	// "exports". You can also still use "import" in CommonJS.
	if p.options.moduleType != config.ModuleESM && p.options.mode != config.ModePassThrough &&
		p.es6ExportKeyword.Len == 0 && p.topLevelAwaitKeyword.Len == 0 {
		p.exportsRef = p.declareCommonJSSymbol(js_ast.SymbolHoisted, "exports")
		p.moduleRef = p.declareCommonJSSymbol(js_ast.SymbolHoisted, "module")
	} else {
		p.exportsRef = p.newSymbol(js_ast.SymbolHoisted, "exports")
		p.moduleRef = p.newSymbol(js_ast.SymbolHoisted, "module")
	}

	// Convert "import.meta" to a variable if it's not supported in the output format
	if p.hasImportMeta && (p.options.unsupportedJSFeatures.Has(compat.ImportMeta) ||
		(p.options.mode != config.ModePassThrough && !p.options.outputFormat.KeepES6ImportExportSyntax())) {
		p.importMetaRef = p.newSymbol(js_ast.SymbolOther, "import_meta")
		p.moduleScope.Generated = append(p.moduleScope.Generated, p.importMetaRef)
	} else {
		p.importMetaRef = js_ast.InvalidRef
	}

	// Handle "@jsx" and "@jsxFrag" pragmas now that lexing is done
	if p.options.jsx.Parse {
		if expr, ok := ParseJSXExpr(p.lexer.JSXFactoryPragmaComment.Text, JSXFactory); !ok {
			p.log.Add(logger.Warning, &p.tracker, p.lexer.JSXFactoryPragmaComment.Range,
				fmt.Sprintf("Invalid JSX factory: %s", p.lexer.JSXFactoryPragmaComment.Text))
		} else if len(expr.Parts) > 0 {
			p.options.jsx.Factory = expr
		}
		if expr, ok := ParseJSXExpr(p.lexer.JSXFragmentPragmaComment.Text, JSXFragment); !ok {
			p.log.Add(logger.Warning, &p.tracker, p.lexer.JSXFragmentPragmaComment.Range,
				fmt.Sprintf("Invalid JSX fragment: %s", p.lexer.JSXFragmentPragmaComment.Text))
		} else if len(expr.Parts) > 0 || expr.Constant != nil {
			p.options.jsx.Fragment = expr
		}
	}
}

func (p *parser) declareCommonJSSymbol(kind js_ast.SymbolKind, name string) js_ast.Ref {
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
	if ok && p.symbols[member.Ref.InnerIndex].Kind == js_ast.SymbolHoisted &&
		kind == js_ast.SymbolHoisted && !p.hasESModuleSyntax {
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
func (p *parser) computeCharacterFrequency() *js_ast.CharFreq {
	if !p.options.minifyIdentifiers || p.source.Index == runtime.SourceIndex {
		return nil
	}

	// Add everything in the file to the histogram
	charFreq := &js_ast.CharFreq{}
	charFreq.Scan(p.source.Contents, 1)

	// Subtract out all comments
	for _, comment := range p.lexer.AllOriginalComments {
		charFreq.Scan(comment.Text, -1)
	}

	// Subtract out all symbols that will be minified
	var visit func(*js_ast.Scope)
	visit = func(scope *js_ast.Scope) {
		for _, member := range scope.Members {
			symbol := &p.symbols[member.Ref.InnerIndex]
			if symbol.SlotNamespace() != js_ast.SlotMustNotBeRenamed {
				charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate))
			}
		}
		if scope.Label.Ref != js_ast.InvalidRef {
			symbol := &p.symbols[scope.Label.Ref.InnerIndex]
			if symbol.SlotNamespace() != js_ast.SlotMustNotBeRenamed {
				charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate)-1)
			}
		}
		for _, child := range scope.Children {
			visit(child)
		}
	}
	visit(p.moduleScope)

	return charFreq
}

func (p *parser) generateImportStmt(
	path string,
	imports []string,
	sourceIndex uint32,
	parts []js_ast.Part,
	symbols map[string]js_ast.Ref,
) []js_ast.Part {
	namespaceRef := p.newSymbol(js_ast.SymbolOther, "import_"+js_ast.GenerateNonUniqueNameFromPath(path))
	p.moduleScope.Generated = append(p.moduleScope.Generated, namespaceRef)
	declaredSymbols := make([]js_ast.DeclaredSymbol, len(imports))
	clauseItems := make([]js_ast.ClauseItem, len(imports))
	importRecordIndex := p.addImportRecord(ast.ImportStmt, logger.Loc{}, path, nil)
	p.importRecords[importRecordIndex].SourceIndex = ast.MakeIndex32(sourceIndex)

	// Create per-import information
	for i, alias := range imports {
		ref := symbols[alias]
		declaredSymbols[i] = js_ast.DeclaredSymbol{Ref: ref, IsTopLevel: true}
		clauseItems[i] = js_ast.ClauseItem{Alias: alias, Name: js_ast.LocRef{Ref: ref}}
		p.isImportItem[ref] = true
		p.namedImports[ref] = js_ast.NamedImport{
			Alias:             alias,
			NamespaceRef:      namespaceRef,
			ImportRecordIndex: importRecordIndex,
		}
	}

	// Append a single import to the end of the file (ES6 imports are hoisted
	// so we don't need to worry about where the import statement goes)
	return append(parts, js_ast.Part{
		DeclaredSymbols:     declaredSymbols,
		ImportRecordIndices: []uint32{importRecordIndex},
		Stmts: []js_ast.Stmt{{Data: &js_ast.SImport{
			NamespaceRef:      namespaceRef,
			Items:             &clauseItems,
			ImportRecordIndex: importRecordIndex,
		}}},
	})
}

func (p *parser) toAST(parts []js_ast.Part, hashbang string, directive string) js_ast.AST {
	// Insert an import statement for any runtime imports we generated
	if len(p.runtimeImports) > 0 && !p.options.omitRuntimeForTests {
		// Sort the imports for determinism
		keys := make([]string, 0, len(p.runtimeImports))
		for key := range p.runtimeImports {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts = p.generateImportStmt("<runtime>", keys, runtime.SourceIndex, parts, p.runtimeImports)
	}

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
	// We only want to repeat the the code that eliminates TypeScript import-equals
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
		p.topLevelSymbolToParts = make(map[js_ast.Ref][]uint32)
		for partIndex, part := range parts {
			for _, declared := range part.DeclaredSymbols {
				if declared.IsTopLevel {
					p.topLevelSymbolToParts[declared.Ref] = append(
						p.topLevelSymbolToParts[declared.Ref], uint32(partIndex))
				}
			}
		}

		// Pulling in the exports of this module always pulls in the export part
		p.topLevelSymbolToParts[p.exportsRef] = append(p.topLevelSymbolToParts[p.exportsRef], js_ast.NSExportPartIndex)

		// Each part tracks the other parts it depends on within this file
		localDependencies := make(map[uint32]uint32)
		for partIndex := range parts {
			part := &parts[partIndex]
			for ref := range part.SymbolUses {
				for _, otherPartIndex := range p.topLevelSymbolToParts[ref] {
					if oldPartIndex, ok := localDependencies[otherPartIndex]; !ok || oldPartIndex != uint32(partIndex) {
						localDependencies[otherPartIndex] = uint32(partIndex)
						part.Dependencies = append(part.Dependencies, js_ast.Dependency{
							SourceIndex: p.source.Index,
							PartIndex:   otherPartIndex,
						})
					}
				}

				// Also map from imports to parts that use them
				if namedImport, ok := p.namedImports[ref]; ok {
					namedImport.LocalPartsWithUses = append(namedImport.LocalPartsWithUses, uint32(partIndex))
					p.namedImports[ref] = namedImport
				}
			}
		}
	}

	// Make a wrapper symbol in case we need to be wrapped in a closure
	wrapperRef := p.newSymbol(js_ast.SymbolOther, "require_"+p.source.IdentifierName)

	// Assign slots to symbols in nested scopes. This is some precomputation for
	// the symbol renaming pass that will happen later in the linker. It's done
	// now in the parser because we want it to be done in parallel per file and
	// we're already executing code in a dedicated goroutine for this file.
	var nestedScopeSlotCounts js_ast.SlotCounts
	if p.options.minifyIdentifiers {
		nestedScopeSlotCounts = renamer.AssignNestedScopeSlots(p.moduleScope, p.symbols)
	}

	exportsKind := js_ast.ExportsNone
	usesExportsRef := p.symbols[p.exportsRef.InnerIndex].UseCountEstimate > 0
	usesModuleRef := p.symbols[p.moduleRef.InnerIndex].UseCountEstimate > 0

	if p.es6ExportKeyword.Len > 0 || p.topLevelAwaitKeyword.Len > 0 {
		exportsKind = js_ast.ExportsESM
	} else if usesExportsRef || usesModuleRef || p.hasTopLevelReturn {
		exportsKind = js_ast.ExportsCommonJS
	} else {
		// If this module has no exports, try to determine what kind of module it
		// is by looking at node's "type" field in "package.json" and/or whether
		// the file extension is ".mjs"/".mts" or ".cjs"/".cts".
		switch p.options.moduleType {
		case config.ModuleCommonJS:
			// "type: commonjs" or ".cjs" or ".cts"
			exportsKind = js_ast.ExportsCommonJS

		case config.ModuleESM:
			// "type: module" or ".mjs" or ".mts"
			exportsKind = js_ast.ExportsESM

		case config.ModuleUnknown:
			// Treat unknown modules containing an import statement as ESM. Otherwise
			// the bundler will treat this file as CommonJS if it's imported and ESM
			// if it's not imported.
			if p.es6ImportKeyword.Len > 0 {
				exportsKind = js_ast.ExportsESM
			}
		}
	}

	return js_ast.AST{
		Parts:                           parts,
		ModuleScope:                     p.moduleScope,
		CharFreq:                        p.computeCharacterFrequency(),
		Symbols:                         p.symbols,
		ExportsRef:                      p.exportsRef,
		ModuleRef:                       p.moduleRef,
		WrapperRef:                      wrapperRef,
		Hashbang:                        hashbang,
		Directive:                       directive,
		NamedImports:                    p.namedImports,
		NamedExports:                    p.namedExports,
		NestedScopeSlotCounts:           nestedScopeSlotCounts,
		TopLevelSymbolToPartsFromParser: p.topLevelSymbolToParts,
		ExportStarImportRecords:         p.exportStarImportRecords,
		ImportRecords:                   p.importRecords,
		ApproximateLineCount:            int32(p.lexer.ApproximateNewlineCount) + 1,

		// CommonJS features
		UsesExportsRef: usesExportsRef,
		UsesModuleRef:  usesModuleRef,
		ExportsKind:    exportsKind,

		// ES6 features
		ImportKeyword:        p.es6ImportKeyword,
		ExportKeyword:        p.es6ExportKeyword,
		TopLevelAwaitKeyword: p.topLevelAwaitKeyword,
	}
}
