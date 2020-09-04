package parser

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
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
	config.Options
	log                      logging.Log
	source                   logging.Source
	lexer                    lexer.Lexer
	allowIn                  bool
	allowPrivateIdentifiers  bool
	hasTopLevelReturn        bool
	fnOptsParse              fnOptsParse
	fnOptsVisit              fnOptsVisit
	latestReturnHadSemicolon bool
	hasImportMeta            bool
	allocatedNames           []string
	latestArrowArgLoc        ast.Loc
	currentScope             *ast.Scope
	scopesForCurrentPart     []*ast.Scope
	symbols                  []ast.Symbol
	tsUseCounts              []uint32
	exportsRef               ast.Ref
	requireRef               ast.Ref
	moduleRef                ast.Ref
	importMetaRef            ast.Ref
	findSymbolHelper         config.FindSymbol
	symbolUses               map[ast.Ref]ast.SymbolUse
	declaredSymbols          []ast.DeclaredSymbol
	runtimeImports           map[string]ast.Ref
	duplicateCaseChecker     duplicateCaseChecker

	// For lowering private methods
	weakMapRef     ast.Ref
	weakSetRef     ast.Ref
	privateGetters map[ast.Ref]ast.Ref
	privateSetters map[ast.Ref]ast.Ref

	// These are for TypeScript
	shouldFoldNumericConstants bool
	enclosingNamespaceRef      *ast.Ref
	emittedNamespaceVars       map[ast.Ref]bool
	isExportedInsideNamespace  map[ast.Ref]ast.Ref
	knownEnumValues            map[ast.Ref]map[string]float64
	localTypeNames             map[string]bool

	// Imports (both ES6 and CommonJS) are tracked at the top level
	importRecords               []ast.ImportRecord
	importRecordsForCurrentPart []uint32
	exportStarImportRecords     []uint32

	// These are for handling ES6 imports and exports
	hasES6ImportSyntax      bool
	hasES6ExportSyntax      bool
	importItemsForNamespace map[ast.Ref]map[string]ast.LocRef
	isImportItem            map[ast.Ref]bool
	namedImports            map[ast.Ref]ast.NamedImport
	namedExports            map[string]ast.Ref
	topLevelSymbolToParts   map[ast.Ref][]uint32

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
	isThisCaptured    bool
	argumentsRef      *ast.Ref
	callTarget        ast.E
	deleteTarget      ast.E
	moduleScope       *ast.Scope
	isControlFlowDead bool

	// These are for recognizing "typeof require == 'function' && require". This
	// is a workaround for code that browserify generates that looks like this:
	//
	//   (function e(t, n, r) {
	//     function s(o2, u) {
	//       if (!n[o2]) {
	//         if (!t[o2]) {
	//           var a = typeof require == "function" && require;
	//           if (!u && a)
	//             return a(o2, true);
	//           if (i)
	//             return i(o2, true);
	//           throw new Error("Cannot find module '" + o2 + "'");
	//         }
	//         var f = n[o2] = {exports: {}};
	//         t[o2][0].call(f.exports, function(e2) {
	//           var n2 = t[o2][1][e2];
	//           return s(n2 ? n2 : e2);
	//         }, f, f.exports, e, t, n, r);
	//       }
	//       return n[o2].exports;
	//     }
	//     var i = typeof require == "function" && require;
	//     for (var o = 0; o < r.length; o++)
	//       s(r[o]);
	//     return s;
	//   });
	//
	// It's checking to see if the environment it's running in has a "require"
	// function before calling it. However, esbuild's bundling environment has a
	// bundle-time require function because it's a bundler. So in this case
	// "typeof require == 'function'" is true and the "&&" expression just
	// becomes a single "require" identifier, which will then crash at run time.
	//
	// The workaround is to explicitly pattern-match for the exact expression
	// "typeof require == 'function' && require" and replace it with "false" if
	// we're targeting the browser.
	//
	// Note that we can't just leave "typeof require == 'function'" alone because
	// there is other code in the wild that legitimately does need it to become
	// "true" when bundling. Specifically, the package "@dagrejs/graphlib" has
	// code that looks like this:
	//
	//   if (typeof require === "function") {
	//     try {
	//       lodash = {
	//         clone: require("lodash/clone"),
	//         constant: require("lodash/constant"),
	//         each: require("lodash/each"),
	//         // ... more calls to require() here ...
	//       };
	//     } catch (e) {
	//       // continue regardless of error
	//     }
	//   }
	//
	// That library will crash later on during startup if that branch isn't
	// taken because "typeof require === 'function'" is false at run time.
	typeofTarget                ast.E
	typeofRequire               ast.E
	typeofRequireEqualsFn       ast.E
	typeofRequireEqualsFnTarget ast.E

	// Temporary variables used for lowering
	tempRefsToDeclare []ast.Ref
	tempRefCount      int
}

const (
	locModuleScope = -1
)

type scopeOrder struct {
	loc   ast.Loc
	scope *ast.Scope
}

// This is function-specific information used during parsing. It is saved and
// restored on the call stack around code that parses nested functions.
type fnOptsParse struct {
	asyncRange     ast.Range
	arrowArgErrors *deferredArrowArgErrors
	isOutsideFn    bool
	allowAwait     bool
	allowYield     bool
	allowSuperCall bool
	isTopLevel     bool
	isConstructor  bool

	// In TypeScript, forward declarations of functions have no bodies
	allowMissingBodyForTypeScript bool

	// Allow TypeScript decorators in function arguments
	allowTSDecorators bool
}

// This is function-specific information used during visiting. It is saved and
// restored on the call stack around code that parses nested functions.
type fnOptsVisit struct {
	isInsideLoop   bool
	isInsideSwitch bool

	// This is used to silence references to "require" inside a try/catch
	// statement. The assumption is that the try/catch statement is there to
	// handle the case where the reference to "require" crashes. Specifically,
	// the workaround handles the "moment" library which contains code that
	// looks like this:
	//
	//   try {
	//     oldLocale = globalLocale._abbr;
	//     var aliasedRequire = require;
	//     aliasedRequire('./locale/' + name);
	//     getSetGlobalLocale(oldLocale);
	//   } catch (e) {}
	//
	tryBodyCount int
}

const bloomFilterSize = 251

type duplicateCaseValue struct {
	hash  uint32
	value ast.Expr
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

func (dc *duplicateCaseChecker) check(p *parser, expr ast.Expr) {
	if hash, ok := duplicateCaseHash(expr); ok {
		bucket := hash % bloomFilterSize
		entry := &dc.bloomFilter[bucket/8]
		mask := byte(1) << (bucket % 8)

		// Check for collisions
		if (*entry & mask) != 0 {
			for _, c := range dc.cases {
				if c.hash == hash {
					if equals, couldBeIncorrect := duplicateCaseEquals(c.value, expr); equals {
						index := strings.LastIndex(p.source.Contents[:expr.Loc.Start], "case")
						r := lexer.RangeOfIdentifier(p.source, ast.Loc{Start: int32(index)})
						if couldBeIncorrect {
							p.log.AddRangeWarning(&p.source, r,
								"This case clause may never be evaluated because it likely duplicates an earlier case clause")
						} else {
							p.log.AddRangeWarning(&p.source, r,
								"This case clause will never be evaluated because it duplicates an earlier case clause")
						}
					}
					return
				}
			}
		}

		*entry |= mask
		dc.cases = append(dc.cases, duplicateCaseValue{hash: hash, value: expr})
	}
}

func hashCombine(seed uint32, hash uint32) uint32 {
	return seed ^ (hash + 0x9e3779b9 + (seed << 6) + (seed >> 2))
}

func duplicateCaseHash(expr ast.Expr) (uint32, bool) {
	switch e := expr.Data.(type) {
	case *ast.ENull:
		return 0, true

	case *ast.EUndefined:
		return 1, true

	case *ast.EBoolean:
		if e.Value {
			return hashCombine(2, 1), true
		}
		return hashCombine(2, 0), true

	case *ast.ENumber:
		bits := math.Float64bits(e.Value)
		return hashCombine(hashCombine(3, uint32(bits)), uint32(bits>>32)), true

	case *ast.EString:
		hash := uint32(4)
		for _, c := range e.Value {
			hash = hashCombine(hash, uint32(c))
		}
		return hash, true

	case *ast.EBigInt:
		hash := uint32(5)
		for _, c := range e.Value {
			hash = hashCombine(hash, uint32(c))
		}
		return hash, true

	case *ast.EIdentifier:
		return hashCombine(6, e.Ref.InnerIndex), true

	case *ast.EDot:
		if target, ok := duplicateCaseHash(e.Target); ok {
			hash := hashCombine(7, target)
			for _, c := range e.Name {
				hash = hashCombine(hash, uint32(c))
			}
			return hash, true
		}

	case *ast.EIndex:
		if target, ok := duplicateCaseHash(e.Target); ok {
			if index, ok := duplicateCaseHash(e.Index); ok {
				return hashCombine(hashCombine(8, target), index), true
			}
		}
	}

	return 0, false
}

func duplicateCaseEquals(left ast.Expr, right ast.Expr) (equals bool, couldBeIncorrect bool) {
	switch a := left.Data.(type) {
	case *ast.ENull:
		_, ok := right.Data.(*ast.ENull)
		return ok, false

	case *ast.EUndefined:
		_, ok := right.Data.(*ast.EUndefined)
		return ok, false

	case *ast.EBoolean:
		b, ok := right.Data.(*ast.EBoolean)
		return ok && a.Value == b.Value, false

	case *ast.ENumber:
		b, ok := right.Data.(*ast.ENumber)
		return ok && a.Value == b.Value, false

	case *ast.EString:
		b, ok := right.Data.(*ast.EString)
		return ok && lexer.UTF16EqualsUTF16(a.Value, b.Value), false

	case *ast.EBigInt:
		b, ok := right.Data.(*ast.EBigInt)
		return ok && a.Value == b.Value, false

	case *ast.EIdentifier:
		b, ok := right.Data.(*ast.EIdentifier)
		return ok && a.Ref == b.Ref, false

	case *ast.EDot:
		if b, ok := right.Data.(*ast.EDot); ok && a.OptionalChain == b.OptionalChain && a.Name == b.Name {
			equals, _ := duplicateCaseEquals(a.Target, b.Target)
			return equals, true
		}

	case *ast.EIndex:
		if b, ok := right.Data.(*ast.EIndex); ok && a.OptionalChain == b.OptionalChain {
			if equals, _ := duplicateCaseEquals(a.Index, b.Index); equals {
				equals, _ := duplicateCaseEquals(a.Target, b.Target)
				return equals, true
			}
		}
	}

	return false, false
}

func isJumpStatement(data ast.S) bool {
	switch data.(type) {
	case *ast.SBreak, *ast.SContinue, *ast.SReturn, *ast.SThrow:
		return true
	}

	return false
}

func isPrimitiveToReorder(e ast.E) bool {
	switch e.(type) {
	case *ast.ENull, *ast.EUndefined, *ast.EString, *ast.EBoolean, *ast.ENumber, *ast.EBigInt:
		return true
	}
	return false
}

func toBooleanWithoutSideEffects(data ast.E) (bool, bool) {
	switch e := data.(type) {
	case *ast.ENull, *ast.EUndefined:
		return false, true

	case *ast.EBoolean:
		return e.Value, true

	case *ast.ENumber:
		return e.Value != 0 && !math.IsNaN(e.Value), true

	case *ast.EBigInt:
		return e.Value != "0", true

	case *ast.EString:
		return len(e.Value) > 0, true

	case *ast.EFunction, *ast.EArrow:
		return true, true
	}

	return false, false
}

func toNumberWithoutSideEffects(data ast.E) (float64, bool) {
	switch e := data.(type) {
	case *ast.ENull:
		return 0, true

	case *ast.EUndefined:
		return math.NaN(), true

	case *ast.EBoolean:
		if e.Value {
			return 1, true
		} else {
			return 0, true
		}

	case *ast.ENumber:
		return e.Value, true
	}

	return 0, false
}

func typeofWithoutSideEffects(data ast.E) (string, bool) {
	switch data.(type) {
	case *ast.ENull:
		return "object", true

	case *ast.EUndefined:
		return "undefined", true

	case *ast.EBoolean:
		return "boolean", true

	case *ast.ENumber:
		return "number", true

	case *ast.EBigInt:
		return "bigint", true

	case *ast.EString:
		return "string", true

	case *ast.EFunction, *ast.EArrow:
		return "function", true
	}

	return "", false
}

// Returns "equal, ok". If "ok" is false, then nothing is known about the two
// values. If "ok" is true, the equality or inequality of the two values is
// stored in "equal".
func checkEqualityIfNoSideEffects(left ast.E, right ast.E) (bool, bool) {
	switch l := left.(type) {
	case *ast.ENull:
		_, ok := right.(*ast.ENull)
		return ok, ok

	case *ast.EUndefined:
		_, ok := right.(*ast.EUndefined)
		return ok, ok

	case *ast.EBoolean:
		r, ok := right.(*ast.EBoolean)
		return ok && l.Value == r.Value, ok

	case *ast.ENumber:
		r, ok := right.(*ast.ENumber)
		return ok && l.Value == r.Value, ok

	case *ast.EBigInt:
		r, ok := right.(*ast.EBigInt)
		return ok && l.Value == r.Value, ok

	case *ast.EString:
		r, ok := right.(*ast.EString)
		return ok && lexer.UTF16EqualsUTF16(l.Value, r.Value), ok
	}

	return false, false
}

func valuesLookTheSame(left ast.E, right ast.E) bool {
	switch a := left.(type) {
	case *ast.EIdentifier:
		if b, ok := right.(*ast.EIdentifier); ok && a.Ref == b.Ref {
			return true
		}

	case *ast.EDot:
		if b, ok := right.(*ast.EDot); ok && a.HasSameFlagsAs(b) &&
			a.Name == b.Name && valuesLookTheSame(a.Target.Data, b.Target.Data) {
			return true
		}

	case *ast.EIndex:
		if b, ok := right.(*ast.EIndex); ok && a.HasSameFlagsAs(b) &&
			valuesLookTheSame(a.Target.Data, b.Target.Data) && valuesLookTheSame(a.Index.Data, b.Index.Data) {
			return true
		}

	case *ast.EIf:
		if b, ok := right.(*ast.EIf); ok && valuesLookTheSame(a.Test.Data, b.Test.Data) &&
			valuesLookTheSame(a.Yes.Data, b.Yes.Data) && valuesLookTheSame(a.No.Data, b.No.Data) {
			return true
		}

	case *ast.EUnary:
		if b, ok := right.(*ast.EUnary); ok && a.Op == b.Op && valuesLookTheSame(a.Value.Data, b.Value.Data) {
			return true
		}

	case *ast.EBinary:
		if b, ok := right.(*ast.EBinary); ok && a.Op == b.Op && valuesLookTheSame(a.Left.Data, b.Left.Data) &&
			valuesLookTheSame(a.Right.Data, b.Right.Data) {
			return true
		}

	case *ast.ECall:
		if b, ok := right.(*ast.ECall); ok && a.HasSameFlagsAs(b) &&
			len(a.Args) == len(b.Args) && valuesLookTheSame(a.Target.Data, b.Target.Data) {
			for i := range a.Args {
				if !valuesLookTheSame(a.Args[i].Data, b.Args[i].Data) {
					return false
				}
			}
			return true
		}
	}

	equal, ok := checkEqualityIfNoSideEffects(left, right)
	return ok && equal
}

func hasValueForThisInCall(expr ast.Expr) bool {
	switch expr.Data.(type) {
	case *ast.EDot, *ast.EIndex:
		return true

	default:
		return false
	}
}

func (p *parser) pushScopeForParsePass(kind ast.ScopeKind, loc ast.Loc) int {
	parent := p.currentScope
	scope := &ast.Scope{
		Kind:     kind,
		Parent:   parent,
		Members:  make(map[string]ast.ScopeMember),
		LabelRef: ast.InvalidRef,
	}
	if parent != nil {
		parent.Children = append(parent.Children, scope)
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
	if kind == ast.ScopeFunctionBody {
		if scope.Parent.Kind != ast.ScopeFunctionArgs {
			panic("Internal error")
		}
		for name, member := range scope.Parent.Members {
			// Don't copy down the optional function expression name. Re-declaring
			// the name of a function expression is allowed.
			if p.symbols[member.Ref.InnerIndex].Kind != ast.SymbolHoistedFunction {
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

func (p *parser) newSymbol(kind ast.SymbolKind, name string) ast.Ref {
	ref := ast.Ref{OuterIndex: p.source.Index, InnerIndex: uint32(len(p.symbols))}
	p.symbols = append(p.symbols, ast.Symbol{
		Kind:         kind,
		OriginalName: name,
		Link:         ast.InvalidRef,
	})
	if p.TS.Parse {
		p.tsUseCounts = append(p.tsUseCounts, 0)
	}
	return ref
}

type mergeResult int

const (
	mergeForbidden = iota
	mergeReplaceWithNew
	mergeKeepExisting
	mergeBecomePrivateGetSetPair
	mergeBecomePrivateStaticGetSetPair
)

func (p *parser) canMergeSymbols(existing ast.SymbolKind, new ast.SymbolKind) mergeResult {
	if existing == ast.SymbolUnbound {
		return mergeReplaceWithNew
	}

	// In TypeScript, imports are allowed to silently collide with symbols within
	// the module. Presumably this is because the imports may be type-only:
	//
	//   import {Foo} from 'bar'
	//   class Foo {}
	//
	if p.TS.Parse && existing == ast.SymbolImport {
		return mergeReplaceWithNew
	}

	// "enum Foo {} enum Foo {}"
	// "namespace Foo { ... } enum Foo {}"
	if new == ast.SymbolTSEnum && (existing == ast.SymbolTSEnum || existing == ast.SymbolTSNamespace) {
		return mergeReplaceWithNew
	}

	// "namespace Foo { ... } namespace Foo { ... }"
	// "function Foo() {} namespace Foo { ... }"
	// "enum Foo {} namespace Foo { ... }"
	if new == ast.SymbolTSNamespace && (existing == ast.SymbolTSNamespace ||
		existing == ast.SymbolHoistedFunction || existing == ast.SymbolTSEnum || existing == ast.SymbolClass) {
		return mergeKeepExisting
	}

	// "var foo; var foo;"
	// "var foo; function foo() {}"
	// "function foo() {} var foo;"
	if new.IsHoisted() && existing.IsHoisted() {
		return mergeKeepExisting
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
		return mergeKeepExisting
	}

	return mergeForbidden
}

func (p *parser) declareSymbol(kind ast.SymbolKind, loc ast.Loc, name string) ast.Ref {
	// Allocate a new symbol
	ref := p.newSymbol(kind, name)

	// Check for a collision in the declaring scope
	if existing, ok := p.currentScope.Members[name]; ok {
		symbol := &p.symbols[existing.Ref.InnerIndex]

		switch p.canMergeSymbols(symbol.Kind, kind) {
		case mergeForbidden:
			r := lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddRangeError(&p.source, r, fmt.Sprintf("%q has already been declared", name))
			return existing.Ref

		case mergeKeepExisting:
			ref = existing.Ref

		case mergeReplaceWithNew:
			symbol.Link = ref

		case mergeBecomePrivateGetSetPair:
			ref = existing.Ref
			symbol.Kind = ast.SymbolPrivateGetSetPair

		case mergeBecomePrivateStaticGetSetPair:
			ref = existing.Ref
			symbol.Kind = ast.SymbolPrivateStaticGetSetPair
		}
	}

	// Overwrite this name in the declaring scope
	p.currentScope.Members[name] = ast.ScopeMember{Ref: ref, Loc: loc}
	return ref
}

func (p *parser) hoistSymbols(scope *ast.Scope) {
nextMember:
	for _, member := range scope.Members {
		symbol := &p.symbols[member.Ref.InnerIndex]

		// Check for collisions that would prevent to hoisting "var" symbols up to the enclosing function scope
		if (symbol.Kind.IsHoisted() || symbol.Kind == ast.SymbolCatchIdentifier) && !scope.Kind.StopsHoisting() {
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
				if s.Kind == ast.ScopeWith {
					symbol.MustNotBeRenamed = true
				}

				if existing, ok := s.Members[symbol.OriginalName]; ok {
					switch p.symbols[existing.Ref.InnerIndex].Kind {
					case ast.SymbolUnbound, ast.SymbolHoisted, ast.SymbolHoistedFunction, ast.SymbolCatchIdentifier:
						// Silently merge this symbol into the existing symbol
						symbol.Link = existing.Ref
						s.Members[symbol.OriginalName] = existing
						continue nextMember

					default:
						// An identifier binding from a catch statement and a function
						// declaration can both silently shadow another hoisted symbol
						if symbol.Kind != ast.SymbolCatchIdentifier && symbol.Kind != ast.SymbolHoistedFunction {
							r := lexer.RangeOfIdentifier(p.source, member.Loc)
							p.log.AddRangeError(&p.source, r, fmt.Sprintf("%q has already been declared", symbol.OriginalName))
						}
						continue nextMember
					}
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

func (p *parser) declareBinding(kind ast.SymbolKind, binding ast.Binding, opts parseStmtOpts) {
	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		name := p.loadNameFromRef(b.Ref)
		if !opts.isTypeScriptDeclare || (opts.isNamespaceScope && opts.isExport) {
			b.Ref = p.declareSymbol(kind, binding.Loc, name)
			if opts.isExport {
				p.recordExport(binding.Loc, name, b.Ref)
			}
		}

	case *ast.BArray:
		for _, i := range b.Items {
			p.declareBinding(kind, i.Binding, opts)
		}

	case *ast.BObject:
		for _, property := range b.Properties {
			p.declareBinding(kind, property.Value, opts)
		}

	default:
		panic("Internal error")
	}
}

func (p *parser) recordExport(loc ast.Loc, alias string, ref ast.Ref) {
	// This is only an ES6 export if we're not inside a TypeScript namespace
	if p.enclosingNamespaceRef == nil {
		if _, ok := p.namedExports[alias]; ok {
			// Warn about duplicate exports
			p.log.AddRangeError(&p.source, lexer.RangeOfIdentifier(p.source, loc),
				fmt.Sprintf("Multiple exports with the same name %q", alias))
		} else {
			p.namedExports[alias] = ref
		}
	}
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
	if p.TS.Parse {
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

func (p *parser) callRuntime(loc ast.Loc, name string, args []ast.Expr) ast.Expr {
	ref, ok := p.runtimeImports[name]
	if !ok {
		ref = p.newSymbol(ast.SymbolOther, name)
		p.runtimeImports[name] = ref
	}
	p.recordUsage(ref)
	return ast.Expr{Loc: loc, Data: &ast.ECall{
		Target: ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: ref}},
		Args:   args,
	}}
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
func (p *parser) storeNameInRef(name string) ast.Ref {
	c := (*reflect.StringHeader)(unsafe.Pointer(&p.source.Contents))
	n := (*reflect.StringHeader)(unsafe.Pointer(&name))

	// Is the data in "name" a subset of the data in "p.source.Contents"?
	if n.Data >= c.Data && n.Data+uintptr(n.Len) < c.Data+uintptr(c.Len) {
		// The name is a slice of the file contents, so we can just reference it by
		// length and don't have to allocate anything. This is the common case.
		//
		// It's stored as a negative value so we'll crash if we try to use it. That
		// way we'll catch cases where we've forgetten to call loadNameFromRef().
		// The length is the negative part because we know it's non-zero.
		return ast.Ref{OuterIndex: -uint32(n.Len), InnerIndex: uint32(n.Data - c.Data)}
	} else {
		// The name is some memory allocated elsewhere. This is either an inline
		// string constant in the parser or an identifier with escape sequences
		// in the source code, which is very unusual. Stash it away for later.
		// This uses allocations but it should hopefully be very uncommon.
		ref := ast.Ref{OuterIndex: 0x80000000, InnerIndex: uint32(len(p.allocatedNames))}
		p.allocatedNames = append(p.allocatedNames, name)
		return ref
	}
}

// This is the inverse of storeNameInRef() above
func (p *parser) loadNameFromRef(ref ast.Ref) string {
	if ref.OuterIndex == 0x80000000 {
		return p.allocatedNames[ref.InnerIndex]
	} else {
		if (ref.OuterIndex & 0x80000000) == 0 {
			panic("Internal error: invalid symbol reference")
		}
		return p.source.Contents[ref.InnerIndex : int32(ref.InnerIndex)-int32(ref.OuterIndex)]
	}
}

// Due to ES6 destructuring patterns, there are many cases where it's
// impossible to distinguish between an array or object literal and a
// destructuring assignment until we hit the "=" operator later on.
// This object defers errors about being in one state or the other
// until we discover which state we're in.
type deferredErrors struct {
	// These are errors for expressions
	invalidExprDefaultValue  ast.Range
	invalidExprAfterQuestion ast.Range

	// These are errors for destructuring patterns
	invalidBindingCommaAfterSpread ast.Range
}

func (from *deferredErrors) mergeInto(to *deferredErrors) {
	if from.invalidExprDefaultValue.Len > 0 {
		to.invalidExprDefaultValue = from.invalidExprDefaultValue
	}
	if from.invalidExprAfterQuestion.Len > 0 {
		to.invalidExprAfterQuestion = from.invalidExprAfterQuestion
	}
	if from.invalidBindingCommaAfterSpread.Len > 0 {
		to.invalidBindingCommaAfterSpread = from.invalidBindingCommaAfterSpread
	}
}

func (p *parser) logExprErrors(errors *deferredErrors) {
	if errors.invalidExprDefaultValue.Len > 0 {
		p.log.AddRangeError(&p.source, errors.invalidExprDefaultValue, "Unexpected \"=\"")
	}

	if errors.invalidExprAfterQuestion.Len > 0 {
		r := errors.invalidExprAfterQuestion
		p.log.AddRangeError(&p.source, r, fmt.Sprintf("Unexpected %q", p.source.Contents[r.Loc.Start:r.Loc.Start+r.Len]))
	}
}

func (p *parser) logBindingErrors(errors *deferredErrors) {
	if errors.invalidBindingCommaAfterSpread.Len > 0 {
		p.log.AddRangeError(&p.source, errors.invalidBindingCommaAfterSpread, "Unexpected \",\" after rest pattern")
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
	invalidExprAwait ast.Range
	invalidExprYield ast.Range
}

func (p *parser) logArrowArgErrors(errors *deferredArrowArgErrors) {
	if errors.invalidExprAwait.Len > 0 {
		r := errors.invalidExprAwait
		p.log.AddRangeError(&p.source, r, "Cannot use an \"await\" expression here")
	}

	if errors.invalidExprYield.Len > 0 {
		r := errors.invalidExprYield
		p.log.AddRangeError(&p.source, r, "Cannot use a \"yield\" expression here")
	}
}

func (p *parser) keyNameForError(key ast.Expr) string {
	switch k := key.Data.(type) {
	case *ast.EString:
		return fmt.Sprintf("%q", lexer.UTF16ToString(k.Value))
	case *ast.EPrivateIdentifier:
		return fmt.Sprintf("%q", p.loadNameFromRef(k.Ref))
	}
	return "property"
}

type propertyOpts struct {
	asyncRange  ast.Range
	isAsync     bool
	isGenerator bool

	// Class-related options
	isStatic          bool
	isClass           bool
	classHasExtends   bool
	allowTSDecorators bool
	tsDecorators      []ast.Expr
}

func (p *parser) parseProperty(kind ast.PropertyKind, opts propertyOpts, errors *deferredErrors) (ast.Property, bool) {
	var key ast.Expr
	keyRange := p.lexer.Range()
	isComputed := false

	switch p.lexer.Token {
	case lexer.TNumericLiteral:
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.ENumber{Value: p.lexer.Number}}
		p.lexer.Next()

	case lexer.TStringLiteral:
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EString{Value: p.lexer.StringLiteral}}
		p.lexer.Next()

	case lexer.TBigIntegerLiteral:
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EBigInt{Value: p.lexer.Identifier}}
		p.markSyntaxFeature(compat.BigInt, p.lexer.Range())
		p.lexer.Next()

	case lexer.TPrivateIdentifier:
		if !opts.isClass || len(opts.tsDecorators) > 0 {
			p.lexer.Expected(lexer.TIdentifier)
		}
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EPrivateIdentifier{Ref: p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Next()

	case lexer.TOpenBracket:
		isComputed = true
		p.markSyntaxFeature(compat.ObjectExtensions, p.lexer.Range())
		p.lexer.Next()
		wasIdentifier := p.lexer.Token == lexer.TIdentifier
		expr := p.parseExpr(ast.LComma)

		// Handle index signatures
		if p.TS.Parse && p.lexer.Token == lexer.TColon && wasIdentifier && opts.isClass {
			if _, ok := expr.Data.(*ast.EIdentifier); ok {
				// "[key: string]: any;"
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
				p.lexer.Expect(lexer.TCloseBracket)
				p.lexer.Expect(lexer.TColon)
				p.skipTypeScriptType(ast.LLowest)
				p.lexer.ExpectOrInsertSemicolon()

				// Skip this property entirely
				return ast.Property{}, false
			}
		}

		p.lexer.Expect(lexer.TCloseBracket)
		key = expr

	case lexer.TAsterisk:
		if kind != ast.PropertyNormal || opts.isGenerator {
			p.lexer.Unexpected()
		}
		p.lexer.Next()
		opts.isGenerator = true
		return p.parseProperty(ast.PropertyNormal, opts, errors)

	default:
		name := p.lexer.Identifier
		nameRange := p.lexer.Range()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		// Support contextual keywords
		if kind == ast.PropertyNormal && !opts.isGenerator {
			// Does the following token look like a key?
			couldBeModifierKeyword := p.lexer.IsIdentifierOrKeyword()
			if !couldBeModifierKeyword {
				switch p.lexer.Token {
				case lexer.TOpenBracket, lexer.TNumericLiteral, lexer.TStringLiteral,
					lexer.TAsterisk, lexer.TPrivateIdentifier:
					couldBeModifierKeyword = true
				}
			}

			// If so, check for a modifier keyword
			if couldBeModifierKeyword {
				switch name {
				case "get":
					if !opts.isAsync {
						p.markSyntaxFeature(compat.ObjectAccessors, nameRange)
						return p.parseProperty(ast.PropertyGet, opts, nil)
					}

				case "set":
					if !opts.isAsync {
						p.markSyntaxFeature(compat.ObjectAccessors, nameRange)
						return p.parseProperty(ast.PropertySet, opts, nil)
					}

				case "async":
					if !opts.isAsync {
						opts.isAsync = true
						opts.asyncRange = nameRange
						return p.parseProperty(kind, opts, nil)
					}

				case "static":
					if !opts.isStatic && !opts.isAsync && opts.isClass {
						opts.isStatic = true
						return p.parseProperty(kind, opts, nil)
					}

				case "private", "protected", "public", "readonly", "abstract", "declare":
					// Skip over TypeScript keywords
					if opts.isClass && p.TS.Parse {
						return p.parseProperty(kind, opts, nil)
					}
				}
			}
		}

		key = ast.Expr{Loc: nameRange.Loc, Data: &ast.EString{Value: lexer.StringToUTF16(name)}}

		// Parse a shorthand property
		if !opts.isClass && kind == ast.PropertyNormal && p.lexer.Token != lexer.TColon &&
			p.lexer.Token != lexer.TOpenParen && p.lexer.Token != lexer.TLessThan && !opts.isGenerator &&
			lexer.Keywords[name] == lexer.T(0) {
			ref := p.storeNameInRef(name)
			value := ast.Expr{Loc: key.Loc, Data: &ast.EIdentifier{Ref: ref}}

			// Destructuring patterns have an optional default value
			var initializer *ast.Expr = nil
			if errors != nil && p.lexer.Token == lexer.TEquals {
				errors.invalidExprDefaultValue = p.lexer.Range()
				p.lexer.Next()
				value := p.parseExpr(ast.LComma)
				initializer = &value
			}

			return ast.Property{
				Kind:         kind,
				Key:          key,
				Value:        &value,
				Initializer:  initializer,
				WasShorthand: true,
			}, true
		}
	}

	if p.TS.Parse {
		// "class X { foo?: number }"
		// "class X { foo!: number }"
		if opts.isClass && (p.lexer.Token == lexer.TQuestion || p.lexer.Token == lexer.TExclamation) {
			p.lexer.Next()
		}

		// "class X { foo?<T>(): T }"
		// "const x = { foo<T>(): T {} }"
		p.skipTypeScriptTypeParameters()
	}

	// Parse a class field with an optional initial value
	if opts.isClass && kind == ast.PropertyNormal && !opts.isAsync &&
		!opts.isGenerator && p.lexer.Token != lexer.TOpenParen {
		var initializer *ast.Expr

		// Forbid the names "constructor" and "prototype" in some cases
		if !isComputed {
			if str, ok := key.Data.(*ast.EString); ok && (lexer.UTF16EqualsString(str.Value, "constructor") ||
				(opts.isStatic && lexer.UTF16EqualsString(str.Value, "prototype"))) {
				p.log.AddRangeError(&p.source, keyRange, fmt.Sprintf("Invalid field name %q", lexer.UTF16ToString(str.Value)))
			}
		}

		// Skip over types
		if p.TS.Parse && p.lexer.Token == lexer.TColon {
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)
		}

		if p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			value := p.parseExpr(ast.LComma)
			initializer = &value
		}

		// Special-case private identifiers
		if private, ok := key.Data.(*ast.EPrivateIdentifier); ok {
			name := p.loadNameFromRef(private.Ref)
			if name == "#constructor" {
				p.log.AddRangeError(&p.source, keyRange, fmt.Sprintf("Invalid field name %q", name))
			}
			var declare ast.SymbolKind
			if opts.isStatic {
				declare = ast.SymbolPrivateStaticField
			} else {
				declare = ast.SymbolPrivateField
			}
			private.Ref = p.declareSymbol(declare, key.Loc, name)
		}

		p.lexer.ExpectOrInsertSemicolon()
		return ast.Property{
			TSDecorators: opts.tsDecorators,
			Kind:         kind,
			IsComputed:   isComputed,
			IsStatic:     opts.isStatic,
			Key:          key,
			Initializer:  initializer,
		}, true
	}

	// Parse a method expression
	if p.lexer.Token == lexer.TOpenParen || kind != ast.PropertyNormal ||
		opts.isClass || opts.isAsync || opts.isGenerator {
		if p.lexer.Token == lexer.TOpenParen && kind != ast.PropertyGet && kind != ast.PropertySet {
			p.markSyntaxFeature(compat.ObjectExtensions, p.lexer.Range())
		}
		loc := p.lexer.Loc()
		scopeIndex := p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)
		isConstructor := false

		// Forbid the names "constructor" and "prototype" in some cases
		if opts.isClass && !isComputed {
			if str, ok := key.Data.(*ast.EString); ok {
				if !opts.isStatic && lexer.UTF16EqualsString(str.Value, "constructor") {
					switch {
					case kind == ast.PropertyGet:
						p.log.AddRangeError(&p.source, keyRange, "Class constructor cannot be a getter")
					case kind == ast.PropertySet:
						p.log.AddRangeError(&p.source, keyRange, "Class constructor cannot be a setter")
					case opts.isAsync:
						p.log.AddRangeError(&p.source, keyRange, "Class constructor cannot be an async function")
					case opts.isGenerator:
						p.log.AddRangeError(&p.source, keyRange, "Class constructor cannot be a generator")
					default:
						isConstructor = true
					}
				} else if opts.isStatic && lexer.UTF16EqualsString(str.Value, "prototype") {
					p.log.AddRangeError(&p.source, keyRange, "Invalid static method name \"prototype\"")
				}
			}
		}

		fn, hadBody := p.parseFn(nil, fnOptsParse{
			asyncRange:        opts.asyncRange,
			allowAwait:        opts.isAsync,
			allowYield:        opts.isGenerator,
			allowSuperCall:    opts.classHasExtends && isConstructor,
			allowTSDecorators: opts.allowTSDecorators,
			isConstructor:     isConstructor,

			// Only allow omitting the body if we're parsing TypeScript class
			allowMissingBodyForTypeScript: p.TS.Parse && opts.isClass,
		})

		// "class Foo { foo(): void; foo(): void {} }"
		if !hadBody {
			// Skip this property entirely
			p.popAndDiscardScope(scopeIndex)
			return ast.Property{}, false
		}

		p.popScope()
		value := ast.Expr{Loc: loc, Data: &ast.EFunction{Fn: fn}}

		// Enforce argument rules for accessors
		switch kind {
		case ast.PropertyGet:
			if len(fn.Args) > 0 {
				r := lexer.RangeOfIdentifier(p.source, fn.Args[0].Binding.Loc)
				p.log.AddRangeError(&p.source, r, fmt.Sprintf("Getter %s must have zero arguments", p.keyNameForError(key)))
			}

		case ast.PropertySet:
			if len(fn.Args) != 1 {
				r := lexer.RangeOfIdentifier(p.source, key.Loc)
				if len(fn.Args) > 1 {
					r = lexer.RangeOfIdentifier(p.source, fn.Args[1].Binding.Loc)
				}
				p.log.AddRangeError(&p.source, r, fmt.Sprintf("Setter %s must have exactly one argument", p.keyNameForError(key)))
			}
		}

		// Special-case private identifiers
		if private, ok := key.Data.(*ast.EPrivateIdentifier); ok {
			var declare ast.SymbolKind
			var suffix string
			switch kind {
			case ast.PropertyGet:
				if opts.isStatic {
					declare = ast.SymbolPrivateStaticGet
				} else {
					declare = ast.SymbolPrivateGet
				}
				suffix = "_get"
			case ast.PropertySet:
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
				p.log.AddRangeError(&p.source, keyRange, fmt.Sprintf("Invalid method name %q", name))
			}
			private.Ref = p.declareSymbol(declare, key.Loc, name)
			if p.UnsupportedFeatures.Has(declare.Feature()) {
				methodRef := p.newSymbol(ast.SymbolOther, name[1:]+suffix)
				if kind == ast.PropertySet {
					p.privateSetters[private.Ref] = methodRef
				} else {
					p.privateGetters[private.Ref] = methodRef
				}
			}
		}

		return ast.Property{
			TSDecorators: opts.tsDecorators,
			Kind:         kind,
			IsComputed:   isComputed,
			IsMethod:     true,
			IsStatic:     opts.isStatic,
			Key:          key,
			Value:        &value,
		}, true
	}

	// Parse an object key/value pair
	p.lexer.Expect(lexer.TColon)
	value := p.parseExprOrBindings(ast.LComma, errors)
	return ast.Property{
		Kind:       kind,
		IsComputed: isComputed,
		Key:        key,
		Value:      &value,
	}, true
}

func (p *parser) parsePropertyBinding() ast.PropertyBinding {
	var key ast.Expr
	isComputed := false

	switch p.lexer.Token {
	case lexer.TDotDotDot:
		p.lexer.Next()
		value := ast.Binding{Loc: p.lexer.Loc(), Data: &ast.BIdentifier{Ref: p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Expect(lexer.TIdentifier)
		return ast.PropertyBinding{
			IsSpread: true,
			Value:    value,
		}

	case lexer.TNumericLiteral:
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.ENumber{Value: p.lexer.Number}}
		p.lexer.Next()

	case lexer.TStringLiteral:
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EString{Value: p.lexer.StringLiteral}}
		p.lexer.Next()

	case lexer.TBigIntegerLiteral:
		key = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EBigInt{Value: p.lexer.Identifier}}
		p.markSyntaxFeature(compat.BigInt, p.lexer.Range())
		p.lexer.Next()

	case lexer.TOpenBracket:
		isComputed = true
		p.lexer.Next()
		key = p.parseExpr(ast.LComma)
		p.lexer.Expect(lexer.TCloseBracket)

	default:
		name := p.lexer.Identifier
		loc := p.lexer.Loc()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()
		key = ast.Expr{Loc: loc, Data: &ast.EString{Value: lexer.StringToUTF16(name)}}

		if p.lexer.Token != lexer.TColon && p.lexer.Token != lexer.TOpenParen {
			ref := p.storeNameInRef(name)
			value := ast.Binding{Loc: loc, Data: &ast.BIdentifier{Ref: ref}}

			var defaultValue *ast.Expr
			if p.lexer.Token == lexer.TEquals {
				p.lexer.Next()
				init := p.parseExpr(ast.LComma)
				defaultValue = &init
			}

			return ast.PropertyBinding{
				Key:          key,
				Value:        value,
				DefaultValue: defaultValue,
			}
		}
	}

	p.lexer.Expect(lexer.TColon)
	value := p.parseBinding()

	var defaultValue *ast.Expr
	if p.lexer.Token == lexer.TEquals {
		p.lexer.Next()
		init := p.parseExpr(ast.LComma)
		defaultValue = &init
	}

	return ast.PropertyBinding{
		IsComputed:   isComputed,
		Key:          key,
		Value:        value,
		DefaultValue: defaultValue,
	}
}

func (p *parser) parseArrowBody(args []ast.Arg, opts fnOptsParse) *ast.EArrow {
	arrowLoc := p.lexer.Loc()

	// Newlines are not allowed before "=>"
	if p.lexer.HasNewlineBefore {
		p.log.AddRangeError(&p.source, p.lexer.Range(), "Unexpected newline before \"=>\"")
		panic(lexer.LexerPanic{})
	}

	if p.lexer.Token == lexer.TEqualsGreaterThan {
		p.markSyntaxFeature(compat.Arrow, p.lexer.Range())
		p.lexer.Next()
	} else {
		p.lexer.Expected(lexer.TEqualsGreaterThan)
	}

	for _, arg := range args {
		p.declareBinding(ast.SymbolHoisted, arg.Binding, parseStmtOpts{})
	}

	// The ability to call "super()" is inherited by arrow functions
	opts.allowSuperCall = p.fnOptsParse.allowSuperCall

	if p.lexer.Token == lexer.TOpenBrace {
		return &ast.EArrow{
			Args: args,
			Body: p.parseFnBody(opts),
		}
	}

	p.pushScopeForParsePass(ast.ScopeFunctionBody, arrowLoc)
	defer p.popScope()

	oldFnOpts := p.fnOptsParse
	p.fnOptsParse = opts
	expr := p.parseExpr(ast.LComma)
	p.fnOptsParse = oldFnOpts
	return &ast.EArrow{
		Args:       args,
		PreferExpr: true,
		Body:       ast.FnBody{Loc: arrowLoc, Stmts: []ast.Stmt{{Loc: expr.Loc, Data: &ast.SReturn{Value: &expr}}}},
	}
}

func (p *parser) isAsyncExprSuffix() bool {
	switch p.lexer.Token {
	case lexer.TFunction, lexer.TEqualsGreaterThan:
		return true
	}
	return false
}

// This parses an expression. This assumes we've already parsed the "async"
// keyword and are currently looking at the following token.
func (p *parser) parseAsyncPrefixExpr(asyncRange ast.Range) ast.Expr {
	// Make sure this matches the switch statement in isAsyncExprSuffix()
	switch p.lexer.Token {
	// "async function() {}"
	case lexer.TFunction:
		return p.parseFnExpr(asyncRange.Loc, true /* isAsync */, asyncRange)

		// "async => {}"
	case lexer.TEqualsGreaterThan:
		arg := ast.Arg{Binding: ast.Binding{Loc: asyncRange.Loc, Data: &ast.BIdentifier{Ref: p.storeNameInRef("async")}}}

		p.pushScopeForParsePass(ast.ScopeFunctionArgs, asyncRange.Loc)
		defer p.popScope()

		return ast.Expr{Loc: asyncRange.Loc, Data: p.parseArrowBody([]ast.Arg{arg}, fnOptsParse{})}

		// "async x => {}"
	case lexer.TIdentifier:
		ref := p.storeNameInRef(p.lexer.Identifier)
		arg := ast.Arg{Binding: ast.Binding{Loc: p.lexer.Loc(), Data: &ast.BIdentifier{Ref: ref}}}
		p.lexer.Next()

		p.pushScopeForParsePass(ast.ScopeFunctionArgs, asyncRange.Loc)
		defer p.popScope()

		arrow := p.parseArrowBody([]ast.Arg{arg}, fnOptsParse{allowAwait: true})
		arrow.IsAsync = true
		return ast.Expr{Loc: asyncRange.Loc, Data: arrow}

		// "async()"
		// "async () => {}"
	case lexer.TOpenParen:
		p.lexer.Next()
		return p.parseParenExpr(asyncRange.Loc, parenExprOpts{isAsync: true})

		// "async"
		// "async + 1"
	default:
		// Distinguish between a call like "async<T>()" and an arrow like "async <T>() => {}"
		if p.TS.Parse && p.lexer.Token == lexer.TLessThan && p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() {
			p.lexer.Next()
			return p.parseParenExpr(asyncRange.Loc, parenExprOpts{isAsync: true})
		}

		return ast.Expr{Loc: asyncRange.Loc, Data: &ast.EIdentifier{Ref: p.storeNameInRef("async")}}
	}
}

func (p *parser) parseFnExpr(loc ast.Loc, isAsync bool, asyncRange ast.Range) ast.Expr {
	p.lexer.Next()
	isGenerator := p.lexer.Token == lexer.TAsterisk
	if isGenerator {
		p.markSyntaxFeature(compat.Generator, p.lexer.Range())
		p.lexer.Next()
	}
	var name *ast.LocRef

	p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)
	defer p.popScope()

	// The name is optional
	if p.lexer.Token == lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		name = &ast.LocRef{Loc: nameLoc, Ref: p.declareSymbol(ast.SymbolHoistedFunction, nameLoc, p.lexer.Identifier)}
		p.lexer.Next()
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.TS.Parse {
		p.skipTypeScriptTypeParameters()
	}

	fn, _ := p.parseFn(name, fnOptsParse{
		asyncRange: asyncRange,
		allowAwait: isAsync,
		allowYield: isGenerator,
	})
	return ast.Expr{Loc: loc, Data: &ast.EFunction{Fn: fn}}
}

type parenExprOpts struct {
	isAsync      bool
	forceArrowFn bool
}

// This assumes that the open parenthesis has already been parsed by the caller
func (p *parser) parseParenExpr(loc ast.Loc, opts parenExprOpts) ast.Expr {
	items := []ast.Expr{}
	errors := deferredErrors{}
	arrowArgErrors := deferredArrowArgErrors{}
	spreadRange := ast.Range{}
	typeColonRange := ast.Range{}

	// Push a scope assuming this is an arrow function. It may not be, in which
	// case we'll need to roll this change back. This has to be done ahead of
	// parsing the arguments instead of later on when we hit the "=>" token and
	// we know it's an arrow function because the arguments may have default
	// values that introduce new scopes and declare new symbols. If this is an
	// arrow function, then those new scopes will need to be parented under the
	// scope of the arrow function itself.
	scopeIndex := p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)

	// Allow "in" inside parentheses
	oldAllowIn := p.allowIn
	p.allowIn = true

	// Forbid "await" and "yield", but only for arrow functions
	oldFnOpts := p.fnOptsParse
	p.fnOptsParse.arrowArgErrors = &arrowArgErrors

	// Scan over the comma-separated arguments or expressions
	for p.lexer.Token != lexer.TCloseParen {
		itemLoc := p.lexer.Loc()
		isSpread := p.lexer.Token == lexer.TDotDotDot

		if isSpread {
			spreadRange = p.lexer.Range()
			p.lexer.Next()
		}

		// We don't know yet whether these are arguments or expressions, so parse
		// a superset of the expression syntax. Errors about things that are valid
		// in one but not in the other are deferred.
		p.latestArrowArgLoc = p.lexer.Loc()
		item := p.parseExprOrBindings(ast.LComma, &errors)

		if isSpread {
			item = ast.Expr{Loc: itemLoc, Data: &ast.ESpread{Value: item}}
		}

		// Skip over types
		if p.TS.Parse && p.lexer.Token == lexer.TColon {
			typeColonRange = p.lexer.Range()
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)
		}

		// There may be a "=" after the type
		if p.TS.Parse && p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			item = ast.Assign(item, p.parseExpr(ast.LComma))
		}

		items = append(items, item)
		if p.lexer.Token != lexer.TComma {
			break
		}

		// Spread arguments must come last. If there's a spread argument followed
		// by a comma, throw an error if we use these expressions as bindings.
		if isSpread {
			errors.invalidBindingCommaAfterSpread = p.lexer.Range()
		}

		// Eat the comma token
		p.lexer.Next()
	}

	// The parenthetical construct must end with a close parenthesis
	p.lexer.Expect(lexer.TCloseParen)

	// Restore "in" operator status before we parse the arrow function body
	p.allowIn = oldAllowIn

	// Also restore "await" and "yield" expression errors
	p.fnOptsParse = oldFnOpts

	// Are these arguments to an arrow function?
	if p.lexer.Token == lexer.TEqualsGreaterThan || opts.forceArrowFn || (p.TS.Parse && p.lexer.Token == lexer.TColon) {
		invalidLog := []ast.Loc{}
		args := []ast.Arg{}

		// First, try converting the expressions to bindings
		for _, item := range items {
			isSpread := false
			if spread, ok := item.Data.(*ast.ESpread); ok {
				item = spread.Value
				isSpread = true
			}
			binding, initializer, log := p.convertExprToBindingAndInitializer(item, invalidLog)
			if initializer != nil && isSpread {
				index := strings.LastIndex(p.source.Contents[:initializer.Loc.Start], "=")
				r := ast.Range{Loc: ast.Loc{Start: int32(index)}, Len: 1}
				p.log.AddRangeError(&p.source, r, "A rest argument cannot have a default initializer")
			}
			invalidLog = log
			args = append(args, ast.Arg{Binding: binding, Default: initializer})
		}

		// Avoid parsing TypeScript code like "a ? (1 + 2) : (3 + 4)" as an arrow
		// function. The ":" after the ")" may be a return type annotation, so we
		// attempt to convert the expressions to bindings first before deciding
		// whether this is an arrow function, and only pick an arrow function if
		// there were no conversion errors.
		if p.lexer.Token == lexer.TEqualsGreaterThan || (len(invalidLog) == 0 &&
			p.trySkipTypeScriptArrowReturnTypeWithBacktracking()) || opts.forceArrowFn {
			p.logBindingErrors(&errors)
			p.logArrowArgErrors(&arrowArgErrors)

			// Now that we've decided we're an arrow function, report binding pattern
			// conversion errors
			if len(invalidLog) > 0 {
				for _, loc := range invalidLog {
					p.log.AddError(&p.source, loc, "Invalid binding pattern")
				}
				panic(lexer.LexerPanic{})
			}

			arrow := p.parseArrowBody(args, fnOptsParse{allowAwait: opts.isAsync})
			arrow.IsAsync = opts.isAsync
			arrow.HasRestArg = spreadRange.Len > 0
			p.popScope()
			return ast.Expr{Loc: loc, Data: arrow}
		}
	}

	// If we get here, it's not an arrow function so undo the pushing of the
	// scope we did earlier. This needs to flatten any child scopes into the
	// parent scope as if the scope was never pushed in the first place.
	p.popAndFlattenScope(scopeIndex)

	// If this isn't an arrow function, then types aren't allowed
	if typeColonRange.Len > 0 {
		p.log.AddRangeError(&p.source, typeColonRange, "Unexpected \":\"")
		panic(lexer.LexerPanic{})
	}

	// Are these arguments for a call to a function named "async"?
	if opts.isAsync {
		p.logExprErrors(&errors)
		async := ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: p.storeNameInRef("async")}}
		return ast.Expr{Loc: loc, Data: &ast.ECall{
			Target: async,
			Args:   items,
		}}
	}

	// Is this a chain of expressions and comma operators?
	if len(items) > 0 {
		p.logExprErrors(&errors)
		if spreadRange.Len > 0 {
			p.log.AddRangeError(&p.source, spreadRange, "Unexpected \"...\"")
			panic(lexer.LexerPanic{})
		}
		value := ast.JoinAllWithComma(items)
		markExprAsParenthesized(value)
		return value
	}

	// Indicate that we expected an arrow function
	p.lexer.Expected(lexer.TEqualsGreaterThan)
	return ast.Expr{}
}

func markExprAsParenthesized(value ast.Expr) {
	if e, ok := value.Data.(*ast.EArrow); ok {
		e.IsParenthesized = true
	}
}

func (p *parser) convertExprToBindingAndInitializer(expr ast.Expr, invalidLog []ast.Loc) (ast.Binding, *ast.Expr, []ast.Loc) {
	var initializer *ast.Expr
	if assign, ok := expr.Data.(*ast.EBinary); ok && assign.Op == ast.BinOpAssign {
		initializer = &assign.Right
		expr = assign.Left
	}
	binding, invalidLog := p.convertExprToBinding(expr, invalidLog)
	return binding, initializer, invalidLog
}

func (p *parser) convertExprToBinding(expr ast.Expr, invalidLog []ast.Loc) (ast.Binding, []ast.Loc) {
	switch e := expr.Data.(type) {
	case *ast.EMissing:
		return ast.Binding{Loc: expr.Loc, Data: &ast.BMissing{}}, invalidLog

	case *ast.EIdentifier:
		return ast.Binding{Loc: expr.Loc, Data: &ast.BIdentifier{Ref: e.Ref}}, invalidLog

	case *ast.EArray:
		items := []ast.ArrayBinding{}
		isSpread := false
		for _, item := range e.Items {
			if i, ok := item.Data.(*ast.ESpread); ok {
				isSpread = true
				item = i.Value
			}
			binding, initializer, log := p.convertExprToBindingAndInitializer(item, invalidLog)
			invalidLog = log
			items = append(items, ast.ArrayBinding{Binding: binding, DefaultValue: initializer})
		}
		return ast.Binding{Loc: expr.Loc, Data: &ast.BArray{
			Items:        items,
			HasSpread:    isSpread,
			IsSingleLine: e.IsSingleLine,
		}}, invalidLog

	case *ast.EObject:
		properties := []ast.PropertyBinding{}
		for _, item := range e.Properties {
			if item.IsMethod || item.Kind == ast.PropertyGet || item.Kind == ast.PropertySet {
				invalidLog = append(invalidLog, item.Key.Loc)
				continue
			}
			binding, initializer, log := p.convertExprToBindingAndInitializer(*item.Value, invalidLog)
			invalidLog = log
			if initializer == nil {
				initializer = item.Initializer
			}
			properties = append(properties, ast.PropertyBinding{
				IsSpread:     item.Kind == ast.PropertySpread,
				IsComputed:   item.IsComputed,
				Key:          item.Key,
				Value:        binding,
				DefaultValue: initializer,
			})
		}
		return ast.Binding{Loc: expr.Loc, Data: &ast.BObject{
			Properties:   properties,
			IsSingleLine: e.IsSingleLine,
		}}, invalidLog

	default:
		invalidLog = append(invalidLog, expr.Loc)
		return ast.Binding{}, invalidLog
	}
}

func (p *parser) convertBindingToExpr(binding ast.Binding, wrapIdentifier func(ast.Loc, ast.Ref) ast.Expr) ast.Expr {
	loc := binding.Loc

	switch b := binding.Data.(type) {
	case *ast.BMissing:
		return ast.Expr{Loc: loc, Data: &ast.EMissing{}}

	case *ast.BIdentifier:
		if wrapIdentifier != nil {
			return wrapIdentifier(loc, b.Ref)
		}
		return ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: b.Ref}}

	case *ast.BArray:
		exprs := make([]ast.Expr, len(b.Items))
		for i, item := range b.Items {
			expr := p.convertBindingToExpr(item.Binding, wrapIdentifier)
			if b.HasSpread && i+1 == len(b.Items) {
				expr = ast.Expr{Loc: expr.Loc, Data: &ast.ESpread{Value: expr}}
			} else if item.DefaultValue != nil {
				expr = ast.Assign(expr, *item.DefaultValue)
			}
			exprs[i] = expr
		}
		return ast.Expr{Loc: loc, Data: &ast.EArray{
			Items:        exprs,
			IsSingleLine: b.IsSingleLine,
		}}

	case *ast.BObject:
		properties := make([]ast.Property, len(b.Properties))
		for i, property := range b.Properties {
			value := p.convertBindingToExpr(property.Value, wrapIdentifier)
			kind := ast.PropertyNormal
			if property.IsSpread {
				kind = ast.PropertySpread
			}
			properties[i] = ast.Property{
				Kind:        kind,
				IsComputed:  property.IsComputed,
				Key:         property.Key,
				Value:       &value,
				Initializer: property.DefaultValue,
			}
		}
		return ast.Expr{Loc: loc, Data: &ast.EObject{
			Properties:   properties,
			IsSingleLine: b.IsSingleLine,
		}}

	default:
		panic("Internal error")
	}
}

type exprFlag uint8

const (
	exprFlagTSDecorator exprFlag = 1 << iota
)

func (p *parser) parsePrefix(level ast.L, errors *deferredErrors, flags exprFlag) ast.Expr {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TSuper:
		p.lexer.Next()

		switch p.lexer.Token {
		case lexer.TOpenParen:
			if level < ast.LCall && p.fnOptsParse.allowSuperCall {
				return ast.Expr{Loc: loc, Data: &ast.ESuper{}}
			}

		case lexer.TDot, lexer.TOpenBracket:
			return ast.Expr{Loc: loc, Data: &ast.ESuper{}}
		}

		p.lexer.Unexpected()
		return ast.Expr{}

	case lexer.TOpenParen:
		p.lexer.Next()

		// Arrow functions aren't allowed in the middle of expressions
		if level > ast.LAssign {
			// Allow "in" inside parentheses
			oldAllowIn := p.allowIn
			p.allowIn = true

			value := p.parseExpr(ast.LLowest)
			markExprAsParenthesized(value)
			p.lexer.Expect(lexer.TCloseParen)

			p.allowIn = oldAllowIn
			return value
		}

		value := p.parseParenExpr(loc, parenExprOpts{})
		return value

	case lexer.TFalse:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: false}}

	case lexer.TTrue:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: true}}

	case lexer.TNull:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.ENull{}}

	case lexer.TThis:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EThis{}}

	case lexer.TIdentifier:
		name := p.lexer.Identifier
		nameRange := p.lexer.Range()
		raw := p.lexer.Raw()
		p.lexer.Next()

		// Handle async and await expressions
		switch raw {
		case "async":
			return p.parseAsyncPrefixExpr(nameRange)

		case "await":
			if p.fnOptsParse.allowAwait {
				if p.fnOptsParse.isTopLevel {
					p.markSyntaxFeature(compat.TopLevelAwait, nameRange)
				}
				if p.fnOptsParse.arrowArgErrors != nil {
					p.fnOptsParse.arrowArgErrors.invalidExprAwait = nameRange
				}
				return ast.Expr{Loc: loc, Data: &ast.EAwait{Value: p.parseExpr(ast.LPrefix)}}
			}

		case "yield":
			if p.fnOptsParse.allowYield {
				if level > ast.LAssign {
					p.log.AddRangeError(&p.source, nameRange, "Cannot use a \"yield\" expression here without parentheses")
				}
				if p.fnOptsParse.arrowArgErrors != nil {
					p.fnOptsParse.arrowArgErrors.invalidExprYield = nameRange
				}
				return p.parseYieldExpr(loc)
			} else if !p.lexer.HasNewlineBefore {
				// Try to gracefully recover if "yield" is used in the wrong place
				switch p.lexer.Token {
				case lexer.TNull, lexer.TIdentifier, lexer.TFalse, lexer.TTrue,
					lexer.TNumericLiteral, lexer.TBigIntegerLiteral, lexer.TStringLiteral:
					p.log.AddRangeError(&p.source, nameRange, "Cannot use \"yield\" outside a generator function")
					return p.parseYieldExpr(loc)
				}
			}
		}

		// Handle the start of an arrow expression
		if p.lexer.Token == lexer.TEqualsGreaterThan {
			ref := p.storeNameInRef(name)
			arg := ast.Arg{Binding: ast.Binding{Loc: loc, Data: &ast.BIdentifier{Ref: ref}}}

			p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)
			defer p.popScope()

			return ast.Expr{Loc: loc, Data: p.parseArrowBody([]ast.Arg{arg}, fnOptsParse{})}
		}

		ref := p.storeNameInRef(name)
		return ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: ref}}

	case lexer.TStringLiteral:
		value := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EString{Value: value}}

	case lexer.TNoSubstitutionTemplateLiteral:
		head := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EString{Value: head, PreferTemplate: true}}

	case lexer.TTemplateHead:
		head := p.lexer.StringLiteral
		parts := p.parseTemplateParts(false /* includeRaw */)
		if p.UnsupportedFeatures.Has(compat.TemplateLiteral) {
			var value ast.Expr
			if len(head) == 0 {
				// "`${x}y`" => "x + 'y'"
				part := parts[0]
				value = ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpAdd,
					Left:  part.Value,
					Right: ast.Expr{Loc: part.TailLoc, Data: &ast.EString{Value: part.Tail}},
				}}
				parts = parts[1:]
			} else {
				// "`x${y}`" => "'x' + y"
				value = ast.Expr{Loc: loc, Data: &ast.EString{Value: head}}
			}
			for _, part := range parts {
				value = ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpAdd,
					Left:  value,
					Right: part.Value,
				}}
				if len(part.Tail) > 0 {
					value = ast.Expr{Loc: loc, Data: &ast.EBinary{
						Op:    ast.BinOpAdd,
						Left:  value,
						Right: ast.Expr{Loc: part.TailLoc, Data: &ast.EString{Value: part.Tail}},
					}}
				}
			}
			return value
		}
		return ast.Expr{Loc: loc, Data: &ast.ETemplate{Head: head, Parts: parts}}

	case lexer.TNumericLiteral:
		value := p.lexer.Number
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.ENumber{Value: value}}

	case lexer.TBigIntegerLiteral:
		value := p.lexer.Identifier
		p.markSyntaxFeature(compat.BigInt, p.lexer.Range())
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EBigInt{Value: value}}

	case lexer.TSlash, lexer.TSlashEquals:
		p.lexer.ScanRegExp()
		value := p.lexer.Raw()
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.ERegExp{Value: value}}

	case lexer.TVoid:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpVoid, Value: value}}

	case lexer.TTypeof:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpTypeof, Value: value}}

	case lexer.TDelete:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		if index, ok := value.Data.(*ast.EIndex); ok {
			if private, ok := index.Index.Data.(*ast.EPrivateIdentifier); ok {
				name := p.loadNameFromRef(private.Ref)
				r := ast.Range{Loc: index.Index.Loc, Len: int32(len(name))}
				p.log.AddRangeError(&p.source, r, fmt.Sprintf("Deleting the private name %q is forbidden", name))
			}
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpDelete, Value: value}}

	case lexer.TPlus:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpPos, Value: value}}

	case lexer.TMinus:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpNeg, Value: value}}

	case lexer.TTilde:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpCpl, Value: value}}

	case lexer.TExclamation:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpNot, Value: value}}

	case lexer.TMinusMinus:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpPreDec, Value: p.parseExpr(ast.LPrefix)}}

	case lexer.TPlusPlus:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EUnary{Op: ast.UnOpPreInc, Value: p.parseExpr(ast.LPrefix)}}

	case lexer.TFunction:
		return p.parseFnExpr(loc, false /* isAsync */, ast.Range{})

	case lexer.TClass:
		p.markSyntaxFeature(compat.Class, p.lexer.Range())
		p.lexer.Next()
		var name *ast.LocRef

		if p.lexer.Token == lexer.TIdentifier {
			p.pushScopeForParsePass(ast.ScopeClassName, loc)
			nameLoc := p.lexer.Loc()
			name = &ast.LocRef{Loc: loc, Ref: p.declareSymbol(ast.SymbolOther, nameLoc, p.lexer.Identifier)}
			p.lexer.Next()
		}

		// Even anonymous classes can have TypeScript type parameters
		if p.TS.Parse {
			p.skipTypeScriptTypeParameters()
		}

		class := p.parseClass(name, parseClassOpts{})

		if name != nil {
			p.popScope()
		}

		return ast.Expr{Loc: loc, Data: &ast.EClass{Class: class}}

	case lexer.TNew:
		p.lexer.Next()

		// Special-case the weird "new.target" expression here
		if p.lexer.Token == lexer.TDot {
			p.lexer.Next()
			if p.lexer.Token != lexer.TIdentifier || p.lexer.Identifier != "target" {
				p.lexer.Unexpected()
			}
			r := ast.Range{Loc: loc, Len: p.lexer.Range().End() - loc.Start}
			p.markSyntaxFeature(compat.NewTarget, r)
			p.lexer.Next()
			return ast.Expr{Loc: loc, Data: &ast.ENewTarget{}}
		}

		target := p.parseExprWithFlags(ast.LCall, flags)
		args := []ast.Expr{}

		if p.TS.Parse {
			// Skip over TypeScript non-null assertions
			if p.lexer.Token == lexer.TExclamation && !p.lexer.HasNewlineBefore {
				p.lexer.Next()
			}

			// Skip over TypeScript type arguments here if there are any
			if p.lexer.Token == lexer.TLessThan {
				p.trySkipTypeScriptTypeArgumentsWithBacktracking()
			}
		}

		if p.lexer.Token == lexer.TOpenParen {
			args = p.parseCallArgs()
		}

		return ast.Expr{Loc: loc, Data: &ast.ENew{Target: target, Args: args}}

	case lexer.TOpenBracket:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []ast.Expr{}
		selfErrors := deferredErrors{}

		// Allow "in" inside arrays
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBracket {
			switch p.lexer.Token {
			case lexer.TComma:
				items = append(items, ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EMissing{}})

			case lexer.TDotDotDot:
				p.markSyntaxFeature(compat.ArraySpread, p.lexer.Range())
				dotsLoc := p.lexer.Loc()
				p.lexer.Next()
				item := p.parseExprOrBindings(ast.LComma, &selfErrors)
				items = append(items, ast.Expr{Loc: dotsLoc, Data: &ast.ESpread{Value: item}})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == lexer.TComma {
					selfErrors.invalidBindingCommaAfterSpread = p.lexer.Range()
				}

			default:
				item := p.parseExprOrBindings(ast.LComma, &selfErrors)
				items = append(items, item)
			}

			if p.lexer.Token != lexer.TComma {
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
		p.lexer.Expect(lexer.TCloseBracket)
		p.allowIn = oldAllowIn

		if p.willNeedBindingPattern() {
			// Is this a binding pattern?
			p.logBindingErrors(&selfErrors)
		} else if errors == nil {
			// Is this an expression?
			p.logExprErrors(&selfErrors)
		} else {
			// In this case, we can't distinguish between the two yet
			selfErrors.mergeInto(errors)
		}

		return ast.Expr{Loc: loc, Data: &ast.EArray{
			Items:        items,
			IsSingleLine: isSingleLine,
		}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []ast.Property{}
		selfErrors := deferredErrors{}

		// Allow "in" inside object literals
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBrace {
			if p.lexer.Token == lexer.TDotDotDot {
				p.lexer.Next()
				value := p.parseExpr(ast.LComma)
				properties = append(properties, ast.Property{
					Kind:  ast.PropertySpread,
					Value: &value,
				})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == lexer.TComma {
					selfErrors.invalidBindingCommaAfterSpread = p.lexer.Range()
				}
			} else {
				// This property may turn out to be a type in TypeScript, which should be ignored
				if property, ok := p.parseProperty(ast.PropertyNormal, propertyOpts{}, &selfErrors); ok {
					properties = append(properties, property)
				}
			}

			if p.lexer.Token != lexer.TComma {
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
		p.lexer.Expect(lexer.TCloseBrace)
		p.allowIn = oldAllowIn

		if p.willNeedBindingPattern() {
			// Is this a binding pattern?
			p.logBindingErrors(&selfErrors)
		} else if errors == nil {
			// Is this an expression?
			p.logExprErrors(&selfErrors)
		} else {
			// In this case, we can't distinguish between the two yet
			selfErrors.mergeInto(errors)
		}

		return ast.Expr{Loc: loc, Data: &ast.EObject{
			Properties:   properties,
			IsSingleLine: isSingleLine,
		}}

	case lexer.TLessThan:
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

		if p.TS.Parse && p.JSX.Parse {
			oldLexer := p.lexer
			p.lexer.Next()

			// Look ahead to see if this should be an arrow function instead
			isTSArrowFn := false
			if p.lexer.Token == lexer.TIdentifier {
				p.lexer.Next()
				if p.lexer.Token == lexer.TComma {
					isTSArrowFn = true
				} else if p.lexer.Token == lexer.TExtends {
					p.lexer.Next()
					isTSArrowFn = p.lexer.Token != lexer.TEquals && p.lexer.Token != lexer.TGreaterThan
				}
			}

			// Restore the lexer
			p.lexer = oldLexer

			if isTSArrowFn {
				p.skipTypeScriptTypeParameters()
				p.lexer.Expect(lexer.TOpenParen)
				return p.parseParenExpr(loc, parenExprOpts{forceArrowFn: true})
			}
		}

		if p.JSX.Parse {
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

		if p.TS.Parse {
			// This is either an old-style type cast or a generic lambda function

			// "<T>(x)"
			// "<T>(x) => {}"
			if p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() {
				p.lexer.Expect(lexer.TOpenParen)
				return p.parseParenExpr(loc, parenExprOpts{})
			}

			// "<T>x"
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)
			p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)
			return p.parsePrefix(level, errors, flags)
		}

		p.lexer.Unexpected()
		return ast.Expr{}

	case lexer.TImport:
		p.hasES6ImportSyntax = true
		p.lexer.Next()
		return p.parseImportExpr(loc)

	default:
		p.lexer.Unexpected()
		return ast.Expr{}
	}
}

func (p *parser) parseYieldExpr(loc ast.Loc) ast.Expr {
	// Parse a yield-from expression, which yields from an iterator
	isStar := p.lexer.Token == lexer.TAsterisk
	if isStar {
		if p.lexer.HasNewlineBefore {
			p.lexer.Unexpected()
		}
		p.lexer.Next()
	}

	var value *ast.Expr

	// The yield expression only has a value in certain cases
	switch p.lexer.Token {
	case lexer.TCloseBrace, lexer.TCloseBracket, lexer.TCloseParen,
		lexer.TColon, lexer.TComma, lexer.TSemicolon:

	default:
		if isStar || !p.lexer.HasNewlineBefore {
			expr := p.parseExpr(ast.LYield)
			value = &expr
		}
	}

	return ast.Expr{Loc: loc, Data: &ast.EYield{Value: value, IsStar: isStar}}
}

func (p *parser) willNeedBindingPattern() bool {
	switch p.lexer.Token {
	case lexer.TEquals:
		// "[a] = b;"
		return true

	case lexer.TIn:
		// "for ([a] in b) {}"
		return !p.allowIn

	case lexer.TIdentifier:
		// "for ([a] of b) {}"
		return !p.allowIn && p.lexer.IsContextualKeyword("of")

	default:
		return false
	}
}

func (p *parser) parseImportExpr(loc ast.Loc) ast.Expr {
	// Parse an "import.meta" expression
	if p.lexer.Token == lexer.TDot {
		p.lexer.Next()
		if p.lexer.IsContextualKeyword("meta") {
			r := p.lexer.Range()
			p.lexer.Next()
			p.hasImportMeta = true
			if p.UnsupportedFeatures.Has(compat.ImportMeta) {
				r = ast.Range{Loc: loc, Len: r.End() - loc.Start}
				p.markSyntaxFeature(compat.ImportMeta, r)
			}
			return ast.Expr{Loc: loc, Data: &ast.EImportMeta{}}
		} else {
			p.lexer.ExpectedString("\"meta\"")
		}
	}

	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	p.lexer.PreserveAllCommentsBefore = true
	p.lexer.Expect(lexer.TOpenParen)
	comments := p.lexer.CommentsToPreserveBefore
	p.lexer.PreserveAllCommentsBefore = false

	value := p.parseExpr(ast.LComma)
	p.lexer.Expect(lexer.TCloseParen)

	p.allowIn = oldAllowIn
	return ast.Expr{Loc: loc, Data: &ast.EImport{Expr: value, LeadingInteriorComments: comments}}
}

func (p *parser) parseExprOrBindings(level ast.L, errors *deferredErrors) ast.Expr {
	return p.parseExprCommon(level, errors, 0)
}

func (p *parser) parseExpr(level ast.L) ast.Expr {
	return p.parseExprCommon(level, nil, 0)
}

func (p *parser) parseExprWithFlags(level ast.L, flags exprFlag) ast.Expr {
	return p.parseExprCommon(level, nil, flags)
}

func (p *parser) parseExprCommon(level ast.L, errors *deferredErrors, flags exprFlag) ast.Expr {
	hadPureCommentBefore := p.lexer.HasPureCommentBefore
	expr := p.parsePrefix(level, errors, flags)

	// There is no formal spec for "__PURE__" comments but from reverse-
	// engineering, it looks like they apply to the next CallExpression or
	// NewExpression. So in "/* @__PURE__ */ a().b() + c()" the comment applies
	// to the expression "a().b()".
	if hadPureCommentBefore && level < ast.LCall {
		expr = p.parseSuffix(expr, ast.LCall-1, errors, flags)
		switch e := expr.Data.(type) {
		case *ast.ECall:
			e.CanBeUnwrappedIfUnused = true
		case *ast.ENew:
			e.CanBeUnwrappedIfUnused = true
		}
	}

	return p.parseSuffix(expr, level, errors, flags)
}

func (p *parser) parseSuffix(left ast.Expr, level ast.L, errors *deferredErrors, flags exprFlag) ast.Expr {
	// ArrowFunction is a special case in the grammar. Although it appears to be
	// a PrimaryExpression, it's actually an AssigmentExpression. This means if
	// a AssigmentExpression ends up producing an ArrowFunction then nothing can
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
	if level < ast.LAssign {
		if arrow, ok := left.Data.(*ast.EArrow); ok && !arrow.IsParenthesized {
			for {
				switch p.lexer.Token {
				case lexer.TComma:
					if level >= ast.LComma {
						return left
					}
					p.lexer.Next()
					left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpComma, Left: left, Right: p.parseExpr(ast.LComma)}}

				default:
					return left
				}
			}
		}
	}

	optionalChain := ast.OptionalChainNone

	for {
		// Reset the optional chain flag by default. That way we won't accidentally
		// treat "c.d" as OptionalChainContinue in "a?.b + c.d".
		oldOptionalChain := optionalChain
		optionalChain = ast.OptionalChainNone

		switch p.lexer.Token {
		case lexer.TDot:
			p.lexer.Next()

			if p.lexer.Token == lexer.TPrivateIdentifier && p.allowPrivateIdentifiers {
				// "a.#b"
				// "a?.b.#c"
				if _, ok := left.Data.(*ast.ESuper); ok {
					p.lexer.Expected(lexer.TIdentifier)
				}
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				p.lexer.Next()
				ref := p.storeNameInRef(name)
				left = ast.Expr{Loc: left.Loc, Data: &ast.EIndex{
					Target:        left,
					Index:         ast.Expr{Loc: nameLoc, Data: &ast.EPrivateIdentifier{Ref: ref}},
					OptionalChain: oldOptionalChain,
				}}
			} else {
				// "a.b"
				// "a?.b.c"
				if !p.lexer.IsIdentifierOrKeyword() {
					p.lexer.Expect(lexer.TIdentifier)
				}
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				p.lexer.Next()
				left = ast.Expr{Loc: left.Loc, Data: &ast.EDot{
					Target:        left,
					Name:          name,
					NameLoc:       nameLoc,
					OptionalChain: oldOptionalChain,
				}}
			}

			optionalChain = oldOptionalChain

		case lexer.TQuestionDot:
			p.lexer.Next()

			switch p.lexer.Token {
			case lexer.TOpenBracket:
				// "a?.[b]"
				p.lexer.Next()

				// Allow "in" inside the brackets
				oldAllowIn := p.allowIn
				p.allowIn = true

				index := p.parseExpr(ast.LLowest)

				p.allowIn = oldAllowIn

				p.lexer.Expect(lexer.TCloseBracket)
				left = ast.Expr{Loc: left.Loc, Data: &ast.EIndex{
					Target:        left,
					Index:         index,
					OptionalChain: ast.OptionalChainStart,
				}}

			case lexer.TOpenParen:
				// "a?.()"
				if level >= ast.LCall {
					return left
				}
				left = ast.Expr{Loc: left.Loc, Data: &ast.ECall{
					Target:        left,
					Args:          p.parseCallArgs(),
					OptionalChain: ast.OptionalChainStart,
				}}

			case lexer.TLessThan:
				// "a?.<T>()"
				if !p.TS.Parse {
					p.lexer.Expected(lexer.TIdentifier)
				}
				p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
				if p.lexer.Token != lexer.TOpenParen {
					p.lexer.Expected(lexer.TOpenParen)
				}
				if level >= ast.LCall {
					return left
				}
				left = ast.Expr{Loc: left.Loc, Data: &ast.ECall{
					Target:        left,
					Args:          p.parseCallArgs(),
					OptionalChain: ast.OptionalChainStart,
				}}

			default:
				if p.lexer.Token == lexer.TPrivateIdentifier && p.allowPrivateIdentifiers {
					// "a?.#b"
					name := p.lexer.Identifier
					nameLoc := p.lexer.Loc()
					p.lexer.Next()
					ref := p.storeNameInRef(name)
					left = ast.Expr{Loc: left.Loc, Data: &ast.EIndex{
						Target:        left,
						Index:         ast.Expr{Loc: nameLoc, Data: &ast.EPrivateIdentifier{Ref: ref}},
						OptionalChain: ast.OptionalChainStart,
					}}
				} else {
					// "a?.b"
					if !p.lexer.IsIdentifierOrKeyword() {
						p.lexer.Expect(lexer.TIdentifier)
					}
					name := p.lexer.Identifier
					nameLoc := p.lexer.Loc()
					p.lexer.Next()
					left = ast.Expr{Loc: left.Loc, Data: &ast.EDot{
						Target:        left,
						Name:          name,
						NameLoc:       nameLoc,
						OptionalChain: ast.OptionalChainStart,
					}}
				}
			}

			optionalChain = ast.OptionalChainContinue

		case lexer.TNoSubstitutionTemplateLiteral:
			if level >= ast.LPrefix {
				return left
			}
			p.markSyntaxFeature(compat.TemplateLiteral, p.lexer.Range())
			head := p.lexer.StringLiteral
			headRaw := p.lexer.RawTemplateContents()
			p.lexer.Next()
			tag := left
			left = ast.Expr{Loc: left.Loc, Data: &ast.ETemplate{Tag: &tag, Head: head, HeadRaw: headRaw}}

		case lexer.TTemplateHead:
			if level >= ast.LPrefix {
				return left
			}
			p.markSyntaxFeature(compat.TemplateLiteral, p.lexer.Range())
			head := p.lexer.StringLiteral
			headRaw := p.lexer.RawTemplateContents()
			parts := p.parseTemplateParts(true /* includeRaw */)
			tag := left
			left = ast.Expr{Loc: left.Loc, Data: &ast.ETemplate{Tag: &tag, Head: head, HeadRaw: headRaw, Parts: parts}}

		case lexer.TOpenBracket:
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

			index := p.parseExpr(ast.LLowest)

			p.allowIn = oldAllowIn

			p.lexer.Expect(lexer.TCloseBracket)
			left = ast.Expr{Loc: left.Loc, Data: &ast.EIndex{
				Target:        left,
				Index:         index,
				OptionalChain: oldOptionalChain,
			}}
			optionalChain = oldOptionalChain

		case lexer.TOpenParen:
			if level >= ast.LCall {
				return left
			}
			left = ast.Expr{Loc: left.Loc, Data: &ast.ECall{
				Target:        left,
				Args:          p.parseCallArgs(),
				OptionalChain: oldOptionalChain,
			}}
			optionalChain = oldOptionalChain

		case lexer.TQuestion:
			if level >= ast.LConditional {
				return left
			}
			p.lexer.Next()

			// Stop now if we're parsing one of these:
			// "(a?) => {}"
			// "(a?: b) => {}"
			// "(a?, b?) => {}"
			if p.TS.Parse && left.Loc == p.latestArrowArgLoc && (p.lexer.Token == lexer.TColon ||
				p.lexer.Token == lexer.TCloseParen || p.lexer.Token == lexer.TComma) {
				if errors == nil {
					p.lexer.Unexpected()
				}
				errors.invalidExprAfterQuestion = p.lexer.Range()
				return left
			}

			// Allow "in" in between "?" and ":"
			oldAllowIn := p.allowIn
			p.allowIn = true

			yes := p.parseExpr(ast.LComma)

			p.allowIn = oldAllowIn

			p.lexer.Expect(lexer.TColon)
			no := p.parseExpr(ast.LComma)
			left = ast.Expr{Loc: left.Loc, Data: &ast.EIf{Test: left, Yes: yes, No: no}}

		case lexer.TExclamation:
			// Skip over TypeScript non-null assertions
			if p.lexer.HasNewlineBefore {
				return left
			}
			if !p.TS.Parse {
				p.lexer.Unexpected()
			}
			if level >= ast.LPostfix {
				return left
			}
			p.lexer.Next()
			optionalChain = oldOptionalChain

		case lexer.TMinusMinus:
			if p.lexer.HasNewlineBefore || level >= ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EUnary{Op: ast.UnOpPostDec, Value: left}}

		case lexer.TPlusPlus:
			if p.lexer.HasNewlineBefore || level >= ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EUnary{Op: ast.UnOpPostInc, Value: left}}

		case lexer.TComma:
			if level >= ast.LComma {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpComma, Left: left, Right: p.parseExpr(ast.LComma)}}

		case lexer.TPlus:
			if level >= ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpAdd, Left: left, Right: p.parseExpr(ast.LAdd)}}

		case lexer.TPlusEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpAddAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TMinus:
			if level >= ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpSub, Left: left, Right: p.parseExpr(ast.LAdd)}}

		case lexer.TMinusEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpSubAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAsterisk:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpMul, Left: left, Right: p.parseExpr(ast.LMultiply)}}

		case lexer.TAsteriskAsterisk:
			if level >= ast.LExponentiation {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpPow, Left: left, Right: p.parseExpr(ast.LExponentiation - 1)}}

		case lexer.TAsteriskAsteriskEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpPowAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAsteriskEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpMulAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TPercent:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpRem, Left: left, Right: p.parseExpr(ast.LMultiply)}}

		case lexer.TPercentEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpRemAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TSlash:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpDiv, Left: left, Right: p.parseExpr(ast.LMultiply)}}

		case lexer.TSlashEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpDivAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLooseEq, Left: left, Right: p.parseExpr(ast.LEquals)}}

		case lexer.TExclamationEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLooseNe, Left: left, Right: p.parseExpr(ast.LEquals)}}

		case lexer.TEqualsEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpStrictEq, Left: left, Right: p.parseExpr(ast.LEquals)}}

		case lexer.TExclamationEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpStrictNe, Left: left, Right: p.parseExpr(ast.LEquals)}}

		case lexer.TLessThan:
			// TypeScript allows type arguments to be specified with angle brackets
			// inside an expression. Unlike in other languages, this unfortunately
			// appears to require backtracking to parse.
			if p.TS.Parse && p.trySkipTypeScriptTypeArgumentsWithBacktracking() {
				optionalChain = oldOptionalChain
				continue
			}

			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLt, Left: left, Right: p.parseExpr(ast.LCompare)}}

		case lexer.TLessThanEquals:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLe, Left: left, Right: p.parseExpr(ast.LCompare)}}

		case lexer.TGreaterThan:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpGt, Left: left, Right: p.parseExpr(ast.LCompare)}}

		case lexer.TGreaterThanEquals:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpGe, Left: left, Right: p.parseExpr(ast.LCompare)}}

		case lexer.TLessThanLessThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpShl, Left: left, Right: p.parseExpr(ast.LShift)}}

		case lexer.TLessThanLessThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpShlAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TGreaterThanGreaterThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpShr, Left: left, Right: p.parseExpr(ast.LShift)}}

		case lexer.TGreaterThanGreaterThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpShrAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TGreaterThanGreaterThanGreaterThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpUShr, Left: left, Right: p.parseExpr(ast.LShift)}}

		case lexer.TGreaterThanGreaterThanGreaterThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpUShrAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TQuestionQuestion:
			if level >= ast.LNullishCoalescing {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpNullishCoalescing, Left: left, Right: p.parseExpr(ast.LNullishCoalescing)}}

		case lexer.TQuestionQuestionEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpNullishCoalescingAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TBarBar:
			if level >= ast.LLogicalOr {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLogicalOr, Left: left, Right: p.parseExpr(ast.LLogicalOr)}}

		case lexer.TBarBarEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLogicalOrAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAmpersandAmpersand:
			if level >= ast.LLogicalAnd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLogicalAnd, Left: left, Right: p.parseExpr(ast.LLogicalAnd)}}

		case lexer.TAmpersandAmpersandEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpLogicalAndAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TBar:
			if level >= ast.LBitwiseOr {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpBitwiseOr, Left: left, Right: p.parseExpr(ast.LBitwiseOr)}}

		case lexer.TBarEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpBitwiseOrAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAmpersand:
			if level >= ast.LBitwiseAnd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpBitwiseAnd, Left: left, Right: p.parseExpr(ast.LBitwiseAnd)}}

		case lexer.TAmpersandEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpBitwiseAndAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TCaret:
			if level >= ast.LBitwiseXor {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpBitwiseXor, Left: left, Right: p.parseExpr(ast.LBitwiseXor)}}

		case lexer.TCaretEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpBitwiseXorAssign, Left: left, Right: p.parseExpr(ast.LAssign - 1)}}

		case lexer.TEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Assign(left, p.parseExpr(ast.LAssign-1))

		case lexer.TIn:
			if level >= ast.LCompare || !p.allowIn {
				return left
			}

			// Warn about "!a in b" instead of "!(a in b)"
			if e, ok := left.Data.(*ast.EUnary); ok && e.Op == ast.UnOpNot {
				p.log.AddWarning(&p.source, left.Loc,
					"Suspicious use of the \"!\" operator inside the \"in\" operator")
			}

			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpIn, Left: left, Right: p.parseExpr(ast.LCompare)}}

		case lexer.TInstanceof:
			if level >= ast.LCompare {
				return left
			}

			// Warn about "!a instanceof b" instead of "!(a instanceof b)"
			if e, ok := left.Data.(*ast.EUnary); ok && e.Op == ast.UnOpNot {
				p.log.AddWarning(&p.source, left.Loc,
					"Suspicious use of the \"!\" operator inside the \"instanceof\" operator")
			}

			p.lexer.Next()
			left = ast.Expr{Loc: left.Loc, Data: &ast.EBinary{Op: ast.BinOpInstanceof, Left: left, Right: p.parseExpr(ast.LCompare)}}

		default:
			// Handle the TypeScript "as" operator
			if p.TS.Parse && p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
				continue
			}

			return left
		}
	}
}

func (p *parser) parseCallArgs() []ast.Expr {
	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	args := []ast.Expr{}
	p.lexer.Expect(lexer.TOpenParen)

	for p.lexer.Token != lexer.TCloseParen {
		loc := p.lexer.Loc()
		isSpread := p.lexer.Token == lexer.TDotDotDot
		if isSpread {
			p.markSyntaxFeature(compat.RestArgument, p.lexer.Range())
			p.lexer.Next()
		}
		arg := p.parseExpr(ast.LComma)
		if isSpread {
			arg = ast.Expr{Loc: loc, Data: &ast.ESpread{Value: arg}}
		}
		args = append(args, arg)
		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)
	p.allowIn = oldAllowIn
	return args
}

func (p *parser) parseJSXTag() (ast.Range, string, *ast.Expr) {
	loc := p.lexer.Loc()

	// A missing tag is a fragment
	if p.lexer.Token == lexer.TGreaterThan {
		return ast.Range{Loc: loc, Len: 0}, "", nil
	}

	// The tag is an identifier
	name := p.lexer.Identifier
	tagRange := p.lexer.Range()
	p.lexer.ExpectInsideJSXElement(lexer.TIdentifier)

	// Certain identifiers are strings
	if strings.ContainsRune(name, '-') || (p.lexer.Token != lexer.TDot && name[0] >= 'a' && name[0] <= 'z') {
		return tagRange, name, &ast.Expr{Loc: loc, Data: &ast.EString{Value: lexer.StringToUTF16(name)}}
	}

	// Otherwise, this is an identifier
	tag := &ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: p.storeNameInRef(name)}}

	// Parse a member expression chain
	for p.lexer.Token == lexer.TDot {
		p.lexer.NextInsideJSXElement()
		memberRange := p.lexer.Range()
		member := p.lexer.Identifier
		p.lexer.ExpectInsideJSXElement(lexer.TIdentifier)

		// Dashes are not allowed in member expression chains
		index := strings.IndexByte(member, '-')
		if index >= 0 {
			p.log.AddError(&p.source, ast.Loc{Start: memberRange.Loc.Start + int32(index)}, "Unexpected \"-\"")
			panic(lexer.LexerPanic{})
		}

		name += "." + member
		tag = &ast.Expr{Loc: loc, Data: &ast.EDot{
			Target:  *tag,
			Name:    member,
			NameLoc: memberRange.Loc,
		}}
		tagRange.Len = memberRange.Loc.Start + memberRange.Len - tagRange.Loc.Start
	}

	return tagRange, name, tag
}

func (p *parser) parseJSXElement(loc ast.Loc) ast.Expr {
	// Parse the tag
	_, startText, startTag := p.parseJSXTag()

	// The tag may have TypeScript type arguments: "<Foo<T>/>"
	if p.TS.Parse {
		// Pass a flag to the type argument skipper because we need to call
		// lexer.NextInsideJSXElement() after we hit the closing ">". The next
		// token after the ">" might be an attribute name with a dash in it
		// like this: "<Foo<T> data-disabled/>"
		p.skipTypeScriptTypeArguments(true /* isInsideJSXElement */)
	}

	// Parse attributes
	properties := []ast.Property{}
	if startTag != nil {
	parseAttributes:
		for {
			switch p.lexer.Token {
			case lexer.TIdentifier:
				// Parse the key
				keyRange := p.lexer.Range()
				key := ast.Expr{Loc: keyRange.Loc, Data: &ast.EString{Value: lexer.StringToUTF16(p.lexer.Identifier)}}
				p.lexer.NextInsideJSXElement()

				// Parse the value
				var value ast.Expr
				if p.lexer.Token != lexer.TEquals {
					// Implicitly true value
					value = ast.Expr{Loc: ast.Loc{Start: keyRange.Loc.Start + keyRange.Len}, Data: &ast.EBoolean{Value: true}}
				} else {
					// Use NextInsideJSXElement() not Next() so we can parse a JSX-style string literal
					p.lexer.NextInsideJSXElement()
					if p.lexer.Token == lexer.TStringLiteral {
						value = ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EString{Value: p.lexer.StringLiteral}}
						p.lexer.NextInsideJSXElement()
					} else {
						// Use Expect() not ExpectInsideJSXElement() so we can parse expression tokens
						p.lexer.Expect(lexer.TOpenBrace)
						value = p.parseExpr(ast.LLowest)
						p.lexer.ExpectInsideJSXElement(lexer.TCloseBrace)
					}
				}

				// Add a property
				properties = append(properties, ast.Property{
					Key:   key,
					Value: &value,
				})

			case lexer.TOpenBrace:
				// Use Next() not ExpectInsideJSXElement() so we can parse "..."
				p.lexer.Next()
				p.lexer.Expect(lexer.TDotDotDot)
				value := p.parseExpr(ast.LComma)
				properties = append(properties, ast.Property{
					Kind:  ast.PropertySpread,
					Value: &value,
				})

				// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
				p.lexer.NextInsideJSXElement()

			default:
				break parseAttributes
			}
		}
	}

	// A slash here is a self-closing element
	if p.lexer.Token == lexer.TSlash {
		// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
		p.lexer.NextInsideJSXElement()
		if p.lexer.Token != lexer.TGreaterThan {
			p.lexer.Expected(lexer.TGreaterThan)
		}
		return ast.Expr{Loc: loc, Data: &ast.EJSXElement{Tag: startTag, Properties: properties}}
	}

	// Use ExpectJSXElementChild() so we parse child strings
	p.lexer.ExpectJSXElementChild(lexer.TGreaterThan)

	// Parse the children of this element
	children := []ast.Expr{}
	for {
		switch p.lexer.Token {
		case lexer.TStringLiteral:
			children = append(children, ast.Expr{Loc: p.lexer.Loc(), Data: &ast.EString{Value: p.lexer.StringLiteral}})
			p.lexer.NextJSXElementChild()

		case lexer.TOpenBrace:
			// Use Next() instead of NextJSXElementChild() here since the next token is an expression
			p.lexer.Next()

			// The "..." here is ignored (it's used to signal an array type in TypeScript)
			if p.lexer.Token == lexer.TDotDotDot && p.TS.Parse {
				p.lexer.Next()
			}

			// The expression is optional, and may be absent
			if p.lexer.Token != lexer.TCloseBrace {
				children = append(children, p.parseExpr(ast.LLowest))
			}

			// Use ExpectJSXElementChild() so we parse child strings
			p.lexer.ExpectJSXElementChild(lexer.TCloseBrace)

		case lexer.TLessThan:
			lessThanLoc := p.lexer.Loc()
			p.lexer.NextInsideJSXElement()

			if p.lexer.Token != lexer.TSlash {
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
				p.log.AddRangeError(&p.source, endRange, fmt.Sprintf("Expected closing tag %q to match opening tag %q", endText, startText))
			}
			if p.lexer.Token != lexer.TGreaterThan {
				p.lexer.Expected(lexer.TGreaterThan)
			}

			return ast.Expr{Loc: loc, Data: &ast.EJSXElement{Tag: startTag, Properties: properties, Children: children}}

		default:
			p.lexer.Unexpected()
		}
	}
}

func (p *parser) parseTemplateParts(includeRaw bool) []ast.TemplatePart {
	parts := []ast.TemplatePart{}
	for {
		p.lexer.Next()
		value := p.parseExpr(ast.LLowest)
		tailLoc := p.lexer.Loc()
		p.lexer.RescanCloseBraceAsTemplateToken()
		tail := p.lexer.StringLiteral
		tailRaw := ""
		if includeRaw {
			tailRaw = p.lexer.RawTemplateContents()
		}
		parts = append(parts, ast.TemplatePart{Value: value, TailLoc: tailLoc, Tail: tail, TailRaw: tailRaw})
		if p.lexer.Token == lexer.TTemplateTail {
			p.lexer.Next()
			break
		}
	}
	return parts
}

func (p *parser) parseAndDeclareDecls(kind ast.SymbolKind, opts parseStmtOpts) []ast.Decl {
	decls := []ast.Decl{}

	for {
		var value *ast.Expr
		local := p.parseBinding()
		p.declareBinding(kind, local, opts)

		// Skip over types
		if p.TS.Parse {
			// "let foo!"
			isDefiniteAssignmentAssertion := p.lexer.Token == lexer.TExclamation
			if isDefiniteAssignmentAssertion {
				p.lexer.Next()
			}

			// "let foo: number"
			if isDefiniteAssignmentAssertion || p.lexer.Token == lexer.TColon {
				p.lexer.Expect(lexer.TColon)
				p.skipTypeScriptType(ast.LLowest)
			}
		}

		if p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			expr := p.parseExpr(ast.LComma)
			value = &expr
		}

		decls = append(decls, ast.Decl{Binding: local, Value: value})

		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	return decls
}

func (p *parser) requireInitializers(decls []ast.Decl) {
	for _, d := range decls {
		if d.Value == nil {
			if _, ok := d.Binding.Data.(*ast.BIdentifier); ok {
				p.log.AddError(&p.source, d.Binding.Loc, "This constant must be initialized")
			}
		}
	}
}

func (p *parser) forbidInitializers(decls []ast.Decl, loopType string, isVar bool) {
	if len(decls) > 1 {
		p.log.AddError(&p.source, decls[0].Binding.Loc, fmt.Sprintf("for-%s loops must have a single declaration", loopType))
	} else if len(decls) == 1 && decls[0].Value != nil {
		if isVar {
			if _, ok := decls[0].Binding.Data.(*ast.BIdentifier); ok {
				// This is a weird special case. Initializers are allowed in "var"
				// statements with identifier bindings.
				return
			}
		}
		p.log.AddError(&p.source, decls[0].Value.Loc, fmt.Sprintf("for-%s loop variables cannot have an initializer", loopType))
	}
}

func (p *parser) parseImportClause() ([]ast.ClauseItem, bool) {
	items := []ast.ClauseItem{}
	p.lexer.Expect(lexer.TOpenBrace)
	isSingleLine := !p.lexer.HasNewlineBefore

	for p.lexer.Token != lexer.TCloseBrace {
		alias := p.lexer.Identifier
		aliasLoc := p.lexer.Loc()
		name := ast.LocRef{Loc: aliasLoc, Ref: p.storeNameInRef(alias)}
		originalName := alias

		// The alias may be a keyword
		isIdentifier := p.lexer.Token == lexer.TIdentifier
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		if p.lexer.IsContextualKeyword("as") {
			p.lexer.Next()
			originalName := p.lexer.Identifier
			name = ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(originalName)}
			p.lexer.Expect(lexer.TIdentifier)
		} else if !isIdentifier {
			// An import where the name is a keyword must have an alias
			p.lexer.Unexpected()
		}

		items = append(items, ast.ClauseItem{
			Alias:        alias,
			AliasLoc:     aliasLoc,
			Name:         name,
			OriginalName: originalName,
		})

		if p.lexer.Token != lexer.TComma {
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
	p.lexer.Expect(lexer.TCloseBrace)
	return items, isSingleLine
}

func (p *parser) parseExportClause() ([]ast.ClauseItem, bool) {
	items := []ast.ClauseItem{}
	firstKeywordItemLoc := ast.Loc{}
	p.lexer.Expect(lexer.TOpenBrace)
	isSingleLine := !p.lexer.HasNewlineBefore

	for p.lexer.Token != lexer.TCloseBrace {
		alias := p.lexer.Identifier
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
		if p.lexer.Token != lexer.TIdentifier {
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			if firstKeywordItemLoc.Start == 0 {
				firstKeywordItemLoc = p.lexer.Loc()
			}
		}
		p.lexer.Next()

		if p.lexer.IsContextualKeyword("as") {
			p.lexer.Next()
			alias = p.lexer.Identifier
			aliasLoc = p.lexer.Loc()

			// The alias may be a keyword
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.Next()
		}

		items = append(items, ast.ClauseItem{
			Alias:        alias,
			AliasLoc:     aliasLoc,
			Name:         name,
			OriginalName: originalName,
		})

		if p.lexer.Token != lexer.TComma {
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
	p.lexer.Expect(lexer.TCloseBrace)

	// Throw an error here if we found a keyword earlier and this isn't an
	// "export from" statement after all
	if firstKeywordItemLoc.Start != 0 && !p.lexer.IsContextualKeyword("from") {
		r := lexer.RangeOfIdentifier(p.source, firstKeywordItemLoc)
		p.log.AddRangeError(&p.source, r, fmt.Sprintf("Expected identifier but found %q", p.source.TextForRange(r)))
		panic(lexer.LexerPanic{})
	}

	return items, isSingleLine
}

func (p *parser) parseBinding() ast.Binding {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TIdentifier:
		ref := p.storeNameInRef(p.lexer.Identifier)
		p.lexer.Next()
		return ast.Binding{Loc: loc, Data: &ast.BIdentifier{Ref: ref}}

	case lexer.TOpenBracket:
		p.markSyntaxFeature(compat.Destructuring, p.lexer.Range())
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []ast.ArrayBinding{}
		hasSpread := false

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBracket {
			if p.lexer.Token == lexer.TComma {
				binding := ast.Binding{Loc: p.lexer.Loc(), Data: &ast.BMissing{}}
				items = append(items, ast.ArrayBinding{Binding: binding})
			} else {
				if p.lexer.Token == lexer.TDotDotDot {
					p.lexer.Next()
					hasSpread = true

					// This was a bug in the ES2015 spec that was fixed in ES2016
					if p.lexer.Token != lexer.TIdentifier {
						p.markSyntaxFeature(compat.NestedRestBinding, p.lexer.Range())
					}
				}

				binding := p.parseBinding()

				var defaultValue *ast.Expr
				if !hasSpread && p.lexer.Token == lexer.TEquals {
					p.lexer.Next()
					value := p.parseExpr(ast.LComma)
					defaultValue = &value
				}

				items = append(items, ast.ArrayBinding{Binding: binding, DefaultValue: defaultValue})

				// Commas after spread elements are not allowed
				if hasSpread && p.lexer.Token == lexer.TComma {
					p.log.AddRangeError(&p.source, p.lexer.Range(), "Unexpected \",\" after rest pattern")
					panic(lexer.LexerPanic{})
				}
			}

			if p.lexer.Token != lexer.TComma {
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
		p.lexer.Expect(lexer.TCloseBracket)
		return ast.Binding{Loc: loc, Data: &ast.BArray{
			Items:        items,
			HasSpread:    hasSpread,
			IsSingleLine: isSingleLine,
		}}

	case lexer.TOpenBrace:
		p.markSyntaxFeature(compat.Destructuring, p.lexer.Range())
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []ast.PropertyBinding{}

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBrace {
			property := p.parsePropertyBinding()
			properties = append(properties, property)

			// Commas after spread elements are not allowed
			if property.IsSpread && p.lexer.Token == lexer.TComma {
				p.log.AddRangeError(&p.source, p.lexer.Range(), "Unexpected \",\" after rest pattern")
				panic(lexer.LexerPanic{})
			}

			if p.lexer.Token != lexer.TComma {
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
		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Binding{Loc: loc, Data: &ast.BObject{
			Properties:   properties,
			IsSingleLine: isSingleLine,
		}}
	}

	p.lexer.Expect(lexer.TIdentifier)
	return ast.Binding{}
}

func (p *parser) parseFn(name *ast.LocRef, opts fnOptsParse) (fn ast.Fn, hadBody bool) {
	if opts.allowAwait && opts.allowYield {
		p.markSyntaxFeature(compat.AsyncGenerator, opts.asyncRange)
	}

	fn.Name = name
	fn.HasRestArg = false
	fn.IsAsync = opts.allowAwait
	fn.IsGenerator = opts.allowYield
	fn.OpenParenLoc = p.lexer.Loc()
	p.lexer.Expect(lexer.TOpenParen)

	// Await and yield are not allowed in function arguments
	oldFnOpts := p.fnOptsParse
	p.fnOptsParse.allowAwait = false
	p.fnOptsParse.allowYield = false

	// Reserve the special name "arguments" in this scope. This ensures that it
	// shadows any variable called "arguments" in any parent scopes.
	fn.ArgumentsRef = p.declareSymbol(ast.SymbolHoisted, ast.Loc{}, "arguments")
	p.symbols[fn.ArgumentsRef.InnerIndex].MustNotBeRenamed = true

	for p.lexer.Token != lexer.TCloseParen {
		// Skip over "this" type annotations
		if p.TS.Parse && p.lexer.Token == lexer.TThis {
			p.lexer.Next()
			if p.lexer.Token == lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
			}
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
			continue
		}

		var tsDecorators []ast.Expr
		if opts.allowTSDecorators {
			tsDecorators = p.parseTypeScriptDecorators()
		}

		if !fn.HasRestArg && p.lexer.Token == lexer.TDotDotDot {
			p.markSyntaxFeature(compat.RestArgument, p.lexer.Range())
			p.lexer.Next()
			fn.HasRestArg = true
		}

		isTypeScriptCtorField := false
		isIdentifier := p.lexer.Token == lexer.TIdentifier
		text := p.lexer.Identifier
		arg := p.parseBinding()

		if p.TS.Parse {
			// Skip over TypeScript accessibility modifiers, which turn this argument
			// into a class field when used inside a class constructor. This is known
			// as a "parameter property" in TypeScript.
			if isIdentifier && opts.isConstructor {
				for p.lexer.Token == lexer.TIdentifier || p.lexer.Token == lexer.TOpenBrace || p.lexer.Token == lexer.TOpenBracket {
					if text != "public" && text != "private" && text != "protected" && text != "readonly" {
						break
					}
					isTypeScriptCtorField = true

					// TypeScript requires an identifier binding
					if p.lexer.Token != lexer.TIdentifier {
						p.lexer.Expect(lexer.TIdentifier)
					}
					text = p.lexer.Identifier

					// Re-parse the binding (the current binding is the TypeScript keyword)
					arg = p.parseBinding()
				}
			}

			// "function foo(a?) {}"
			if p.lexer.Token == lexer.TQuestion {
				p.lexer.Next()
			}

			// "function foo(a: any) {}"
			if p.lexer.Token == lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
			}
		}

		p.declareBinding(ast.SymbolHoisted, arg, parseStmtOpts{})

		var defaultValue *ast.Expr
		if !fn.HasRestArg && p.lexer.Token == lexer.TEquals {
			p.markSyntaxFeature(compat.DefaultArgument, p.lexer.Range())
			p.lexer.Next()
			value := p.parseExpr(ast.LComma)
			defaultValue = &value
		}

		fn.Args = append(fn.Args, ast.Arg{
			TSDecorators: tsDecorators,
			Binding:      arg,
			Default:      defaultValue,

			// We need to track this because it affects code generation
			IsTypeScriptCtorField: isTypeScriptCtorField,
		})

		if p.lexer.Token != lexer.TComma {
			break
		}
		if fn.HasRestArg {
			p.lexer.Expect(lexer.TCloseParen)
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)
	p.fnOptsParse = oldFnOpts

	// "function foo(): any {}"
	if p.TS.Parse && p.lexer.Token == lexer.TColon {
		p.lexer.Next()
		p.skipTypeScriptReturnType()
	}

	// "function foo(): any;"
	if opts.allowMissingBodyForTypeScript && p.lexer.Token != lexer.TOpenBrace {
		p.lexer.ExpectOrInsertSemicolon()
		return
	}

	fn.Body = p.parseFnBody(opts)
	hadBody = true
	return
}

func (p *parser) parseClassStmt(loc ast.Loc, opts parseStmtOpts) ast.Stmt {
	var name *ast.LocRef
	if p.lexer.Token == lexer.TClass {
		p.markSyntaxFeature(compat.Class, p.lexer.Range())
		p.lexer.Next()
	} else {
		p.lexer.Expected(lexer.TClass)
	}

	if !opts.isNameOptional || p.lexer.Token == lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		nameText := p.lexer.Identifier
		p.lexer.Expect(lexer.TIdentifier)
		name = &ast.LocRef{Loc: nameLoc, Ref: ast.InvalidRef}
		if !opts.isTypeScriptDeclare {
			name.Ref = p.declareSymbol(ast.SymbolClass, nameLoc, nameText)
			if opts.isExport {
				p.recordExport(nameLoc, nameText, name.Ref)
			}
		}
	}

	// Even anonymous classes can have TypeScript type parameters
	if p.TS.Parse {
		p.skipTypeScriptTypeParameters()
	}

	classOpts := parseClassOpts{
		allowTSDecorators:   true,
		isTypeScriptDeclare: opts.isTypeScriptDeclare,
	}
	if opts.tsDecorators != nil {
		classOpts.tsDecorators = opts.tsDecorators.values
	}
	class := p.parseClass(name, classOpts)
	return ast.Stmt{Loc: loc, Data: &ast.SClass{Class: class, IsExport: opts.isExport}}
}

type parseClassOpts struct {
	tsDecorators        []ast.Expr
	allowTSDecorators   bool
	isTypeScriptDeclare bool
}

// By the time we call this, the identifier and type parameters have already
// been parsed. We need to start parsing from the "extends" clause.
func (p *parser) parseClass(name *ast.LocRef, classOpts parseClassOpts) ast.Class {
	var extends *ast.Expr

	if p.lexer.Token == lexer.TExtends {
		p.lexer.Next()
		value := p.parseExpr(ast.LNew)
		extends = &value

		// TypeScript's type argument parser inside expressions backtracks if the
		// first token after the end of the type parameter list is "{", so the
		// parsed expression above will have backtracked if there are any type
		// arguments. This means we have to re-parse for any type arguments here.
		// This seems kind of wasteful to me but it's what the official compiler
		// does and it probably doesn't have that high of a performance overhead
		// because "extends" clauses aren't that frequent, so it should be ok.
		if p.TS.Parse {
			p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
		}
	}

	if p.TS.Parse && p.lexer.IsContextualKeyword("implements") {
		p.lexer.Next()
		for {
			p.skipTypeScriptType(ast.LLowest)
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}
	}

	bodyLoc := p.lexer.Loc()
	p.lexer.Expect(lexer.TOpenBrace)
	properties := []ast.Property{}

	// Allow "in" and private fields inside class bodies
	oldAllowIn := p.allowIn
	oldAllowPrivateIdentifiers := p.allowPrivateIdentifiers
	p.allowIn = true
	p.allowPrivateIdentifiers = true

	// A scope is needed for private identifiers
	scopeIndex := p.pushScopeForParsePass(ast.ScopeClassBody, bodyLoc)

	for p.lexer.Token != lexer.TCloseBrace {
		if p.lexer.Token == lexer.TSemicolon {
			p.lexer.Next()
			continue
		}

		opts := propertyOpts{
			isClass:           true,
			allowTSDecorators: classOpts.allowTSDecorators,
			classHasExtends:   extends != nil,
		}

		// Parse decorators for this property
		if opts.allowTSDecorators {
			opts.tsDecorators = p.parseTypeScriptDecorators()
		}

		// This property may turn out to be a type in TypeScript, which should be ignored
		if property, ok := p.parseProperty(ast.PropertyNormal, opts, nil); ok {
			properties = append(properties, property)
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

	p.lexer.Expect(lexer.TCloseBrace)
	return ast.Class{
		TSDecorators: classOpts.tsDecorators,
		Name:         name,
		Extends:      extends,
		BodyLoc:      bodyLoc,
		Properties:   properties,
	}
}

func (p *parser) parseLabelName() *ast.LocRef {
	if p.lexer.Token != lexer.TIdentifier || p.lexer.HasNewlineBefore {
		return nil
	}

	name := ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(p.lexer.Identifier)}
	p.lexer.Next()
	return &name
}

func (p *parser) parsePath() (ast.Loc, string) {
	pathLoc := p.lexer.Loc()
	pathText := lexer.UTF16ToString(p.lexer.StringLiteral)
	if p.lexer.Token == lexer.TNoSubstitutionTemplateLiteral {
		p.lexer.Next()
	} else {
		p.lexer.Expect(lexer.TStringLiteral)
	}
	return pathLoc, pathText
}

// This assumes the "function" token has already been parsed
func (p *parser) parseFnStmt(loc ast.Loc, opts parseStmtOpts, isAsync bool, asyncRange ast.Range) ast.Stmt {
	isGenerator := p.lexer.Token == lexer.TAsterisk
	if isGenerator {
		p.markSyntaxFeature(compat.Generator, p.lexer.Range())
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
	if !opts.isNameOptional || p.lexer.Token == lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		nameText = p.lexer.Identifier
		p.lexer.Expect(lexer.TIdentifier)
		name = &ast.LocRef{Loc: nameLoc, Ref: ast.InvalidRef}
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.TS.Parse {
		p.skipTypeScriptTypeParameters()
	}

	// Introduce a fake block scope for function declarations inside if statements
	var ifStmtScopeIndex int
	hasIfScope := opts.lexicalDecl == lexicalDeclAllowFnInsideIf
	if hasIfScope {
		ifStmtScopeIndex = p.pushScopeForParsePass(ast.ScopeBlock, loc)
	}

	scopeIndex := p.pushScopeForParsePass(ast.ScopeFunctionArgs, p.lexer.Loc())

	fn, hadBody := p.parseFn(name, fnOptsParse{
		asyncRange: asyncRange,
		allowAwait: isAsync,
		allowYield: isGenerator,

		// Only allow omitting the body if we're parsing TypeScript
		allowMissingBodyForTypeScript: p.TS.Parse,
	})

	// Don't output anything if it's just a forward declaration of a function
	if opts.isTypeScriptDeclare || !hadBody {
		p.popAndDiscardScope(scopeIndex)

		// Balance the fake block scope introduced above
		if hasIfScope {
			p.popAndDiscardScope(ifStmtScopeIndex)
		}

		return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
	}

	p.popScope()

	// Only declare the function after we know if it had a body or not. Otherwise
	// TypeScript code such as this will double-declare the symbol:
	//
	//     function foo(): void;
	//     function foo(): void {}
	//
	if name != nil {
		kind := ast.SymbolOther
		if !isGenerator && !isAsync {
			kind = ast.SymbolHoistedFunction
		}
		name.Ref = p.declareSymbol(kind, name.Loc, nameText)
		if opts.isExport {
			p.recordExport(name.Loc, nameText, name.Ref)
		}
	}

	// Balance the fake block scope introduced above
	if hasIfScope {
		p.popScope()
	}

	fn.HasIfScope = hasIfScope
	return ast.Stmt{Loc: loc, Data: &ast.SFunction{Fn: fn, IsExport: opts.isExport}}
}

type deferredTSDecorators struct {
	values []ast.Expr

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
}

func (p *parser) parseStmt(opts parseStmtOpts) ast.Stmt {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TSemicolon:
		p.lexer.Next()
		return ast.Stmt{Loc: loc, Data: &ast.SEmpty{}}

	case lexer.TExport:
		if opts.isModuleScope {
			p.hasES6ExportSyntax = true
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
		if opts.tsDecorators != nil && p.lexer.Token != lexer.TClass && p.lexer.Token != lexer.TDefault &&
			!p.lexer.IsContextualKeyword("abstract") && !p.lexer.IsContextualKeyword("declare") {
			p.lexer.Expected(lexer.TClass)
		}

		switch p.lexer.Token {
		case lexer.TClass, lexer.TConst, lexer.TFunction, lexer.TLet, lexer.TVar:
			opts.isExport = true
			return p.parseStmt(opts)

		case lexer.TImport:
			// "export import foo = bar"
			if p.TS.Parse && (opts.isModuleScope || opts.isNamespaceScope) {
				opts.isExport = true
				return p.parseStmt(opts)
			}

			p.lexer.Unexpected()
			return ast.Stmt{}

		case lexer.TEnum:
			if !p.TS.Parse {
				p.lexer.Unexpected()
			}
			opts.isExport = true
			return p.parseStmt(opts)

		case lexer.TIdentifier:
			if p.lexer.IsContextualKeyword("async") {
				// "export async function foo() {}"
				asyncRange := p.lexer.Range()
				p.lexer.Next()
				p.lexer.Expect(lexer.TFunction)
				opts.isExport = true
				return p.parseFnStmt(loc, opts, true /* isAsync */, asyncRange)
			}

			if p.TS.Parse {
				switch p.lexer.Identifier {
				case "type":
					// "export type foo = ..."
					p.lexer.Next()
					p.skipTypeScriptTypeStmt(parseStmtOpts{isModuleScope: opts.isModuleScope, isExport: true})
					return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}

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
			return ast.Stmt{}

		case lexer.TDefault:
			if !opts.isModuleScope {
				p.lexer.Unexpected()
			}

			defaultLoc := p.lexer.Loc()
			p.lexer.Next()

			// The default name is lazily generated only if no other name is present
			createDefaultName := func() ast.LocRef {
				defaultName := ast.LocRef{Loc: defaultLoc, Ref: p.newSymbol(ast.SymbolOther, p.source.IdentifierName+"_default")}
				p.currentScope.Generated = append(p.currentScope.Generated, defaultName.Ref)
				return defaultName
			}

			// TypeScript decorators only work on class declarations
			// "@decorator export default class Foo {}"
			// "@decorator export default abstract class Foo {}"
			if opts.tsDecorators != nil && p.lexer.Token != lexer.TClass && !p.lexer.IsContextualKeyword("abstract") {
				p.lexer.Expected(lexer.TClass)
			}

			if p.lexer.IsContextualKeyword("async") {
				asyncRange := p.lexer.Range()
				p.lexer.Next()

				if p.lexer.Token == lexer.TFunction {
					p.lexer.Expect(lexer.TFunction)
					stmt := p.parseFnStmt(loc, parseStmtOpts{
						isNameOptional: true,
						lexicalDecl:    lexicalDeclAllowAll,
					}, true /* isAsync */, asyncRange)
					if _, ok := stmt.Data.(*ast.STypeScript); ok {
						return stmt // This was just a type annotation
					}

					// Use the statement name if present, since it's a better name
					var defaultName ast.LocRef
					if s, ok := stmt.Data.(*ast.SFunction); ok && s.Fn.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Fn.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}

					p.recordExport(defaultLoc, "default", defaultName.Ref)
					return ast.Stmt{Loc: loc, Data: &ast.SExportDefault{DefaultName: defaultName, Value: ast.ExprOrStmt{Stmt: &stmt}}}
				}

				defaultName := createDefaultName()
				p.recordExport(defaultLoc, "default", defaultName.Ref)
				expr := p.parseSuffix(p.parseAsyncPrefixExpr(asyncRange), ast.LComma, nil, 0)
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{Loc: loc, Data: &ast.SExportDefault{DefaultName: defaultName, Value: ast.ExprOrStmt{Expr: &expr}}}
			}

			if p.lexer.Token == lexer.TFunction || p.lexer.Token == lexer.TClass || p.lexer.IsContextualKeyword("interface") {
				stmt := p.parseStmt(parseStmtOpts{
					tsDecorators:   opts.tsDecorators,
					isNameOptional: true,
					lexicalDecl:    lexicalDeclAllowAll,
				})
				if _, ok := stmt.Data.(*ast.STypeScript); ok {
					return stmt // This was just a type annotation
				}

				// Use the statement name if present, since it's a better name
				var defaultName ast.LocRef
				switch s := stmt.Data.(type) {
				case *ast.SFunction:
					if s.Fn.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Fn.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}
				case *ast.SClass:
					if s.Class.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Class.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}
				default:
					defaultName = createDefaultName()
				}

				p.recordExport(defaultLoc, "default", defaultName.Ref)
				return ast.Stmt{Loc: loc, Data: &ast.SExportDefault{DefaultName: defaultName, Value: ast.ExprOrStmt{Stmt: &stmt}}}
			}

			isIdentifier := p.lexer.Token == lexer.TIdentifier
			name := p.lexer.Identifier
			expr := p.parseExpr(ast.LComma)

			// Handle the default export of an abstract class in TypeScript
			if p.TS.Parse && isIdentifier && name == "abstract" {
				if _, ok := expr.Data.(*ast.EIdentifier); ok && (p.lexer.Token == lexer.TClass || opts.tsDecorators != nil) {
					stmt := p.parseClassStmt(loc, parseStmtOpts{
						tsDecorators:   opts.tsDecorators,
						isNameOptional: true,
					})

					// Use the statement name if present, since it's a better name
					var defaultName ast.LocRef
					if s, ok := stmt.Data.(*ast.SClass); ok && s.Class.Name != nil {
						defaultName = ast.LocRef{Loc: defaultLoc, Ref: s.Class.Name.Ref}
					} else {
						defaultName = createDefaultName()
					}

					p.recordExport(defaultLoc, "default", defaultName.Ref)
					return ast.Stmt{Loc: loc, Data: &ast.SExportDefault{DefaultName: defaultName, Value: ast.ExprOrStmt{Stmt: &stmt}}}
				}
			}

			p.lexer.ExpectOrInsertSemicolon()
			defaultName := createDefaultName()
			p.recordExport(defaultLoc, "default", defaultName.Ref)
			return ast.Stmt{Loc: loc, Data: &ast.SExportDefault{DefaultName: defaultName, Value: ast.ExprOrStmt{Expr: &expr}}}

		case lexer.TAsterisk:
			if !opts.isModuleScope {
				p.lexer.Unexpected()
			}

			p.lexer.Next()
			var namespaceRef ast.Ref
			var alias *ast.ExportStarAlias
			var pathLoc ast.Loc
			var pathText string

			if p.lexer.IsContextualKeyword("as") {
				// "export * as ns from 'path'"
				p.lexer.Next()
				name := p.lexer.Identifier
				namespaceRef = p.storeNameInRef(name)
				alias = &ast.ExportStarAlias{Loc: p.lexer.Loc(), Name: name}
				if !p.lexer.IsIdentifierOrKeyword() {
					p.lexer.Expect(lexer.TIdentifier)
				}
				p.lexer.Next()
				p.lexer.ExpectContextualKeyword("from")
				pathLoc, pathText = p.parsePath()
			} else {
				// "export * from 'path'"
				p.lexer.ExpectContextualKeyword("from")
				pathLoc, pathText = p.parsePath()
				name := ast.GenerateNonUniqueNameFromPath(pathText) + "_star"
				namespaceRef = p.storeNameInRef(name)
			}
			importRecordIndex := p.addImportRecord(ast.ImportStmt, pathLoc, pathText)

			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{Loc: loc, Data: &ast.SExportStar{
				NamespaceRef:      namespaceRef,
				Alias:             alias,
				ImportRecordIndex: importRecordIndex,
			}}

		case lexer.TOpenBrace:
			if !opts.isModuleScope {
				p.lexer.Unexpected()
			}

			items, isSingleLine := p.parseExportClause()
			if p.lexer.IsContextualKeyword("from") {
				p.lexer.Next()
				pathLoc, pathText := p.parsePath()
				importRecordIndex := p.addImportRecord(ast.ImportStmt, pathLoc, pathText)
				name := ast.GenerateNonUniqueNameFromPath(pathText)
				namespaceRef := p.storeNameInRef(name)
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{Loc: loc, Data: &ast.SExportFrom{
					Items:             items,
					NamespaceRef:      namespaceRef,
					ImportRecordIndex: importRecordIndex,
					IsSingleLine:      isSingleLine,
				}}
			}
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{Loc: loc, Data: &ast.SExportClause{Items: items, IsSingleLine: isSingleLine}}

		case lexer.TEquals:
			// "export = value;"
			if p.TS.Parse {
				p.lexer.Next()
				value := p.parseExpr(ast.LLowest)
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{Loc: loc, Data: &ast.SExportEquals{Value: value}}
			}
			p.lexer.Unexpected()
			return ast.Stmt{}

		default:
			p.lexer.Unexpected()
			return ast.Stmt{}
		}

	case lexer.TFunction:
		p.lexer.Next()
		return p.parseFnStmt(loc, opts, false /* isAsync */, ast.Range{})

	case lexer.TEnum:
		if !p.TS.Parse {
			p.lexer.Unexpected()
		}
		return p.parseTypeScriptEnumStmt(loc, opts)

	case lexer.TAt:
		// Parse decorators before class statements, which are potentially exported
		if p.TS.Parse {
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
			if p.lexer.Token != lexer.TClass && p.lexer.Token != lexer.TExport &&
				!p.lexer.IsContextualKeyword("abstract") && !p.lexer.IsContextualKeyword("declare") {
				p.lexer.Expected(lexer.TClass)
			}

			return p.parseStmt(opts)
		}

		p.lexer.Unexpected()
		return ast.Stmt{}

	case lexer.TClass:
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(loc)
		}
		return p.parseClassStmt(loc, opts)

	case lexer.TVar:
		p.lexer.Next()
		decls := p.parseAndDeclareDecls(ast.SymbolHoisted, opts)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{Loc: loc, Data: &ast.SLocal{
			Kind:     ast.LocalVar,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case lexer.TLet:
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(loc)
		}
		p.markSyntaxFeature(compat.Let, p.lexer.Range())
		p.lexer.Next()
		decls := p.parseAndDeclareDecls(ast.SymbolOther, opts)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{Loc: loc, Data: &ast.SLocal{
			Kind:     ast.LocalLet,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case lexer.TConst:
		if opts.lexicalDecl != lexicalDeclAllowAll {
			p.forbidLexicalDecl(loc)
		}
		p.markSyntaxFeature(compat.Const, p.lexer.Range())
		p.lexer.Next()

		if p.TS.Parse && p.lexer.Token == lexer.TEnum {
			return p.parseTypeScriptEnumStmt(loc, opts)
		}

		decls := p.parseAndDeclareDecls(ast.SymbolOther, opts)
		p.lexer.ExpectOrInsertSemicolon()
		if !opts.isTypeScriptDeclare {
			p.requireInitializers(decls)
		}
		return ast.Stmt{Loc: loc, Data: &ast.SLocal{
			Kind:     ast.LocalConst,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case lexer.TIf:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		yes := p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowFnInsideIf})
		var no *ast.Stmt = nil
		if p.lexer.Token == lexer.TElse {
			p.lexer.Next()
			stmt := p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowFnInsideIf})
			no = &stmt
		}
		return ast.Stmt{Loc: loc, Data: &ast.SIf{Test: test, Yes: yes, No: no}}

	case lexer.TDo:
		p.lexer.Next()
		body := p.parseStmt(parseStmtOpts{})
		p.lexer.Expect(lexer.TWhile)
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)

		// This is a weird corner case where automatic semicolon insertion applies
		// even without a newline present
		if p.lexer.Token == lexer.TSemicolon {
			p.lexer.Next()
		}
		return ast.Stmt{Loc: loc, Data: &ast.SDoWhile{Body: body, Test: test}}

	case lexer.TWhile:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return ast.Stmt{Loc: loc, Data: &ast.SWhile{Test: test, Body: body}}

	case lexer.TWith:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		bodyLoc := p.lexer.Loc()
		p.lexer.Expect(lexer.TCloseParen)

		// Push a scope so we make sure to prevent any bare identifiers referenced
		// within the body from being renamed. Renaming them might change the
		// semantics of the code.
		p.pushScopeForParsePass(ast.ScopeWith, bodyLoc)
		body := p.parseStmt(parseStmtOpts{})
		p.popScope()

		return ast.Stmt{Loc: loc, Data: &ast.SWith{Value: test, BodyLoc: bodyLoc, Body: body}}

	case lexer.TSwitch:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)

		bodyLoc := p.lexer.Loc()
		p.pushScopeForParsePass(ast.ScopeBlock, bodyLoc)
		defer p.popScope()

		p.lexer.Expect(lexer.TOpenBrace)
		cases := []ast.Case{}
		foundDefault := false

		for p.lexer.Token != lexer.TCloseBrace {
			var value *ast.Expr = nil
			body := []ast.Stmt{}

			if p.lexer.Token == lexer.TDefault {
				if foundDefault {
					p.log.AddRangeError(&p.source, p.lexer.Range(), "Multiple default clauses are not allowed")
					panic(lexer.LexerPanic{})
				}
				foundDefault = true
				p.lexer.Next()
				p.lexer.Expect(lexer.TColon)
			} else {
				p.lexer.Expect(lexer.TCase)
				expr := p.parseExpr(ast.LLowest)
				value = &expr
				p.lexer.Expect(lexer.TColon)
			}

		caseBody:
			for {
				switch p.lexer.Token {
				case lexer.TCloseBrace, lexer.TCase, lexer.TDefault:
					break caseBody

				default:
					body = append(body, p.parseStmt(parseStmtOpts{lexicalDecl: lexicalDeclAllowAll}))
				}
			}

			cases = append(cases, ast.Case{Value: value, Body: body})
		}

		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Stmt{Loc: loc, Data: &ast.SSwitch{
			Test:    test,
			BodyLoc: bodyLoc,
			Cases:   cases,
		}}

	case lexer.TTry:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenBrace)
		p.pushScopeForParsePass(ast.ScopeBlock, loc)
		body := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
		p.popScope()
		p.lexer.Next()

		var catch *ast.Catch = nil
		var finally *ast.Finally = nil

		if p.lexer.Token == lexer.TCatch {
			catchLoc := p.lexer.Loc()
			p.pushScopeForParsePass(ast.ScopeBlock, catchLoc)
			p.lexer.Next()
			var binding *ast.Binding

			// The catch binding is optional, and can be omitted
			if p.lexer.Token == lexer.TOpenBrace {
				if p.UnsupportedFeatures.Has(compat.OptionalCatchBinding) {
					// Generate a new symbol for the catch binding for older browsers
					ref := p.newSymbol(ast.SymbolOther, "e")
					p.currentScope.Generated = append(p.currentScope.Generated, ref)
					binding = &ast.Binding{Loc: p.lexer.Loc(), Data: &ast.BIdentifier{Ref: ref}}
				}
			} else {
				p.lexer.Expect(lexer.TOpenParen)
				value := p.parseBinding()
				p.lexer.Expect(lexer.TCloseParen)

				// Bare identifiers are a special case
				kind := ast.SymbolOther
				if _, ok := value.Data.(*ast.BIdentifier); ok {
					kind = ast.SymbolCatchIdentifier
				}
				p.declareBinding(kind, value, parseStmtOpts{})
				binding = &value
			}

			p.lexer.Expect(lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
			p.lexer.Next()
			catch = &ast.Catch{Loc: catchLoc, Binding: binding, Body: stmts}
			p.popScope()
		}

		if p.lexer.Token == lexer.TFinally || catch == nil {
			finallyLoc := p.lexer.Loc()
			p.pushScopeForParsePass(ast.ScopeBlock, finallyLoc)
			p.lexer.Expect(lexer.TFinally)
			p.lexer.Expect(lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
			p.lexer.Next()
			finally = &ast.Finally{Loc: finallyLoc, Stmts: stmts}
			p.popScope()
		}

		return ast.Stmt{Loc: loc, Data: &ast.STry{Body: body, Catch: catch, Finally: finally}}

	case lexer.TFor:
		p.pushScopeForParsePass(ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()

		// "for await (let x of y) {}"
		isForAwait := p.lexer.IsContextualKeyword("await")
		if isForAwait {
			awaitRange := p.lexer.Range()
			if !p.fnOptsParse.allowAwait {
				p.log.AddRangeError(&p.source, awaitRange, "Cannot use \"await\" outside an async function")
				isForAwait = false
			} else {
				didGenerateError := p.markSyntaxFeature(compat.ForAwait, awaitRange)
				if p.fnOptsParse.isTopLevel && !didGenerateError {
					p.markSyntaxFeature(compat.TopLevelAwait, awaitRange)
				}
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TOpenParen)

		var init *ast.Stmt = nil
		var test *ast.Expr = nil
		var update *ast.Expr = nil

		// "in" expressions aren't allowed here
		p.allowIn = false

		decls := []ast.Decl{}
		initLoc := p.lexer.Loc()
		isVar := false
		switch p.lexer.Token {
		case lexer.TVar:
			isVar = true
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolHoisted, parseStmtOpts{})
			init = &ast.Stmt{Loc: initLoc, Data: &ast.SLocal{Kind: ast.LocalVar, Decls: decls}}

		case lexer.TLet:
			p.markSyntaxFeature(compat.Let, p.lexer.Range())
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolOther, parseStmtOpts{})
			init = &ast.Stmt{Loc: initLoc, Data: &ast.SLocal{Kind: ast.LocalLet, Decls: decls}}

		case lexer.TConst:
			p.markSyntaxFeature(compat.Const, p.lexer.Range())
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolOther, parseStmtOpts{})
			init = &ast.Stmt{Loc: initLoc, Data: &ast.SLocal{Kind: ast.LocalConst, Decls: decls}}

		case lexer.TSemicolon:

		default:
			init = &ast.Stmt{Loc: initLoc, Data: &ast.SExpr{Value: p.parseExpr(ast.LLowest)}}
		}

		// "in" expressions are allowed again
		p.allowIn = true

		// Detect for-of loops
		if p.lexer.IsContextualKeyword("of") || isForAwait {
			if isForAwait && !p.lexer.IsContextualKeyword("of") {
				if init != nil {
					p.lexer.ExpectedString("\"of\"")
				} else {
					p.lexer.Unexpected()
				}
			}
			p.forbidInitializers(decls, "of", false)
			p.markSyntaxFeature(compat.ForOf, p.lexer.Range())
			p.lexer.Next()
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{Loc: loc, Data: &ast.SForOf{IsAwait: isForAwait, Init: *init, Value: value, Body: body}}
		}

		// Detect for-in loops
		if p.lexer.Token == lexer.TIn {
			p.forbidInitializers(decls, "in", isVar)
			p.lexer.Next()
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{Loc: loc, Data: &ast.SForIn{Init: *init, Value: value, Body: body}}
		}

		// Only require "const" statement initializers when we know we're a normal for loop
		if init != nil {
			if local, ok := init.Data.(*ast.SLocal); ok && local.Kind == ast.LocalConst {
				p.requireInitializers(decls)
			}
		}

		p.lexer.Expect(lexer.TSemicolon)

		if p.lexer.Token != lexer.TSemicolon {
			expr := p.parseExpr(ast.LLowest)
			test = &expr
		}

		p.lexer.Expect(lexer.TSemicolon)

		if p.lexer.Token != lexer.TCloseParen {
			expr := p.parseExpr(ast.LLowest)
			update = &expr
		}

		p.lexer.Expect(lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return ast.Stmt{Loc: loc, Data: &ast.SFor{Init: init, Test: test, Update: update, Body: body}}

	case lexer.TImport:
		p.hasES6ImportSyntax = true
		p.lexer.Next()
		stmt := ast.SImport{}

		// "export import foo = bar"
		// "import foo = bar" in a namespace
		if (opts.isExport || opts.isNamespaceScope) && p.lexer.Token != lexer.TIdentifier {
			p.lexer.Expected(lexer.TIdentifier)
		}

		switch p.lexer.Token {
		case lexer.TOpenParen, lexer.TDot:
			// "import('path')"
			// "import.meta"
			expr := p.parseSuffix(p.parseImportExpr(loc), ast.LLowest, nil, 0)
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: expr}}

		case lexer.TStringLiteral, lexer.TNoSubstitutionTemplateLiteral:
			// "import 'path'"
			if !opts.isModuleScope {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

		case lexer.TAsterisk:
			// "import * as ns from 'path'"
			if !opts.isModuleScope {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			p.lexer.Next()
			p.lexer.ExpectContextualKeyword("as")
			stmt.NamespaceRef = p.storeNameInRef(p.lexer.Identifier)
			starLoc := p.lexer.Loc()
			stmt.StarNameLoc = &starLoc
			p.lexer.Expect(lexer.TIdentifier)
			p.lexer.ExpectContextualKeyword("from")

		case lexer.TOpenBrace:
			// "import {item1, item2} from 'path'"
			if !opts.isModuleScope {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			items, isSingleLine := p.parseImportClause()
			stmt.Items = &items
			stmt.IsSingleLine = isSingleLine
			p.lexer.ExpectContextualKeyword("from")

		case lexer.TIdentifier:
			// "import defaultItem from 'path'"
			// "import foo = bar"
			if !opts.isModuleScope && !opts.isNamespaceScope {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			defaultName := p.lexer.Identifier
			stmt.DefaultName = &ast.LocRef{Loc: p.lexer.Loc(), Ref: p.storeNameInRef(defaultName)}
			p.lexer.Next()

			if p.TS.Parse {
				// Skip over type-only imports
				if defaultName == "type" {
					switch p.lexer.Token {
					case lexer.TIdentifier:
						if p.lexer.Identifier != "from" {
							// "import type foo from 'bar';"
							p.lexer.Next()
							p.lexer.ExpectContextualKeyword("from")
							p.parsePath()
							p.lexer.ExpectOrInsertSemicolon()
							return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
						}

					case lexer.TAsterisk:
						// "import type * as foo from 'bar';"
						p.lexer.Next()
						p.lexer.ExpectContextualKeyword("as")
						p.lexer.Expect(lexer.TIdentifier)
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}

					case lexer.TOpenBrace:
						// "import type {foo} from 'bar';"
						p.parseImportClause()
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
					}
				}

				// Parse TypeScript import assignment statements
				if p.lexer.Token == lexer.TEquals || opts.isExport || opts.isNamespaceScope {
					p.lexer.Expect(lexer.TEquals)
					value := p.parseExpr(ast.LComma)
					p.lexer.ExpectOrInsertSemicolon()
					ref := p.declareSymbol(ast.SymbolOther, stmt.DefaultName.Loc, defaultName)
					decls := []ast.Decl{{
						Binding: ast.Binding{Loc: stmt.DefaultName.Loc, Data: &ast.BIdentifier{Ref: ref}},
						Value:   &value,
					}}
					if opts.isExport {
						p.recordExport(stmt.DefaultName.Loc, defaultName, ref)
					}

					// The kind of statement depends on the expression
					if _, ok := value.Data.(*ast.ECall); ok {
						// "import ns = require('x')"
						return ast.Stmt{Loc: loc, Data: &ast.SLocal{
							Kind:                         ast.LocalConst,
							Decls:                        decls,
							IsExport:                     opts.isExport,
							WasTSImportEqualsInNamespace: opts.isNamespaceScope,
						}}
					} else {
						// "import Foo = Bar"
						return ast.Stmt{Loc: loc, Data: &ast.SLocal{
							Kind:                         ast.LocalVar,
							Decls:                        decls,
							IsExport:                     opts.isExport,
							WasTSImportEqualsInNamespace: opts.isNamespaceScope,
						}}
					}
				}
			}

			if p.lexer.Token == lexer.TComma {
				p.lexer.Next()
				switch p.lexer.Token {
				case lexer.TAsterisk:
					// "import defaultItem, * as ns from 'path'"
					p.lexer.Next()
					p.lexer.ExpectContextualKeyword("as")
					stmt.NamespaceRef = p.storeNameInRef(p.lexer.Identifier)
					starLoc := p.lexer.Loc()
					stmt.StarNameLoc = &starLoc
					p.lexer.Expect(lexer.TIdentifier)

				case lexer.TOpenBrace:
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
			return ast.Stmt{}
		}

		pathLoc, pathText := p.parsePath()
		stmt.ImportRecordIndex = p.addImportRecord(ast.ImportStmt, pathLoc, pathText)
		p.lexer.ExpectOrInsertSemicolon()

		if stmt.StarNameLoc != nil {
			name := p.loadNameFromRef(stmt.NamespaceRef)
			stmt.NamespaceRef = p.declareSymbol(ast.SymbolImport, *stmt.StarNameLoc, name)
		} else {
			// Generate a symbol for the namespace
			name := ast.GenerateNonUniqueNameFromPath(pathText)
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
			itemRefs["default"] = *stmt.DefaultName
		}

		// Link each import item to the namespace
		if stmt.Items != nil {
			for i, item := range *stmt.Items {
				name := p.loadNameFromRef(item.Name.Ref)
				ref := p.declareSymbol(ast.SymbolImport, item.Name.Loc, name)
				p.isImportItem[ref] = true
				(*stmt.Items)[i].Name.Ref = ref
				itemRefs[item.Alias] = ast.LocRef{Loc: item.Name.Loc, Ref: ref}
			}
		}

		// Track the items for this namespace
		p.importItemsForNamespace[stmt.NamespaceRef] = itemRefs

		return ast.Stmt{Loc: loc, Data: &stmt}

	case lexer.TBreak:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{Loc: loc, Data: &ast.SBreak{Label: name}}

	case lexer.TContinue:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{Loc: loc, Data: &ast.SContinue{Label: name}}

	case lexer.TReturn:
		p.lexer.Next()
		var value *ast.Expr
		if p.lexer.Token != lexer.TSemicolon &&
			!p.lexer.HasNewlineBefore &&
			p.lexer.Token != lexer.TCloseBrace &&
			p.lexer.Token != lexer.TEndOfFile {
			expr := p.parseExpr(ast.LLowest)
			value = &expr
		}
		p.latestReturnHadSemicolon = p.lexer.Token == lexer.TSemicolon
		p.lexer.ExpectOrInsertSemicolon()
		if p.fnOptsParse.isOutsideFn {
			p.hasTopLevelReturn = true
		}
		return ast.Stmt{Loc: loc, Data: &ast.SReturn{Value: value}}

	case lexer.TThrow:
		p.lexer.Next()
		if p.lexer.HasNewlineBefore {
			p.log.AddError(&p.source, ast.Loc{Start: loc.Start + 5}, "Unexpected newline after \"throw\"")
			panic(lexer.LexerPanic{})
		}
		expr := p.parseExpr(ast.LLowest)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{Loc: loc, Data: &ast.SThrow{Value: expr}}

	case lexer.TDebugger:
		p.lexer.Next()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{Loc: loc, Data: &ast.SDebugger{}}

	case lexer.TOpenBrace:
		p.pushScopeForParsePass(ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()
		stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
		p.lexer.Next()
		return ast.Stmt{Loc: loc, Data: &ast.SBlock{Stmts: stmts}}

	default:
		isIdentifier := p.lexer.Token == lexer.TIdentifier
		name := p.lexer.Identifier

		// Parse either an async function, an async expression, or a normal expression
		var expr ast.Expr
		if isIdentifier && p.lexer.Raw() == "async" {
			asyncRange := p.lexer.Range()
			p.lexer.Next()
			if p.lexer.Token == lexer.TFunction {
				p.lexer.Next()
				return p.parseFnStmt(asyncRange.Loc, opts, true /* isAsync */, asyncRange)
			}
			expr = p.parseSuffix(p.parseAsyncPrefixExpr(asyncRange), ast.LLowest, nil, 0)
		} else {
			expr = p.parseExpr(ast.LLowest)
		}

		if isIdentifier {
			if ident, ok := expr.Data.(*ast.EIdentifier); ok {
				if p.lexer.Token == lexer.TColon && opts.tsDecorators == nil {
					p.pushScopeForParsePass(ast.ScopeLabel, loc)
					defer p.popScope()

					// Parse a labeled statement
					p.lexer.Next()
					name := ast.LocRef{Loc: expr.Loc, Ref: ident.Ref}
					nestedOpts := parseStmtOpts{}
					if opts.lexicalDecl != lexicalDeclForbid {
						nestedOpts.lexicalDecl = lexicalDeclAllowFnInsideLabel
					}
					stmt := p.parseStmt(nestedOpts)
					return ast.Stmt{Loc: loc, Data: &ast.SLabel{Name: name, Stmt: stmt}}
				}

				if p.TS.Parse {
					switch name {
					case "type":
						if p.lexer.Token == lexer.TIdentifier {
							// "type Foo = any"
							p.skipTypeScriptTypeStmt(parseStmtOpts{isModuleScope: opts.isModuleScope})
							return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
						}

					case "namespace", "module":
						// "namespace Foo {}"
						// "module Foo {}"
						// "declare module 'fs' {}"
						// "declare module 'fs';"
						if (opts.isModuleScope || opts.isNamespaceScope) && (p.lexer.Token == lexer.TIdentifier ||
							(p.lexer.Token == lexer.TStringLiteral && opts.isTypeScriptDeclare)) {
							return p.parseTypeScriptNamespaceStmt(loc, opts)
						}

					case "interface":
						// "interface Foo {}"
						p.skipTypeScriptInterfaceStmt(parseStmtOpts{isModuleScope: opts.isModuleScope})
						return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}

					case "abstract":
						if p.lexer.Token == lexer.TClass || opts.tsDecorators != nil {
							return p.parseClassStmt(loc, opts)
						}

					case "declare":
						opts.lexicalDecl = lexicalDeclAllowAll
						opts.isTypeScriptDeclare = true

						// "@decorator declare class Foo {}"
						// "@decorator declare abstract class Foo {}"
						if opts.tsDecorators != nil && p.lexer.Token != lexer.TClass && !p.lexer.IsContextualKeyword("abstract") {
							p.lexer.Expected(lexer.TClass)
						}

						// "declare global { ... }"
						if p.lexer.IsContextualKeyword("global") {
							p.lexer.Next()
							p.lexer.Expect(lexer.TOpenBrace)
							p.parseStmtsUpTo(lexer.TCloseBrace, opts)
							p.lexer.Next()
							return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
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
							var decls []ast.Decl
							if s, ok := stmt.Data.(*ast.SLocal); ok {
								for _, decl := range s.Decls {
									decls = extractDeclsForBinding(decl.Binding, decls)
								}
							}
							if len(decls) > 0 {
								return ast.Stmt{Loc: loc, Data: &ast.SLocal{
									Kind:     ast.LocalVar,
									IsExport: true,
									Decls:    decls,
								}}
							}
						}

						return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
					}
				}
			}
		}

		p.lexer.ExpectOrInsertSemicolon()

		// Parse a "use strict" directive
		if str, ok := expr.Data.(*ast.EString); ok && lexer.UTF16EqualsString(str.Value, "use strict") {
			return ast.Stmt{Loc: loc, Data: &ast.SDirective{Value: str.Value}}
		}

		return ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: expr}}
	}
}

func extractDeclsForBinding(binding ast.Binding, decls []ast.Decl) []ast.Decl {
	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		decls = append(decls, ast.Decl{Binding: binding})

	case *ast.BArray:
		for _, item := range b.Items {
			decls = extractDeclsForBinding(item.Binding, decls)
		}

	case *ast.BObject:
		for _, property := range b.Properties {
			decls = extractDeclsForBinding(property.Value, decls)
		}

	default:
		panic("Internal error")
	}

	return decls
}

func (p *parser) addImportRecord(kind ast.ImportKind, loc ast.Loc, text string) uint32 {
	index := uint32(len(p.importRecords))
	p.importRecords = append(p.importRecords, ast.ImportRecord{
		Kind:       kind,
		Loc:        loc,
		Path:       ast.Path{Text: text},
		WrapperRef: ast.InvalidRef,
	})
	return index
}

func (p *parser) parseFnBody(opts fnOptsParse) ast.FnBody {
	oldFnOpts := p.fnOptsParse
	oldAllowIn := p.allowIn
	p.fnOptsParse = opts
	p.allowIn = true

	loc := p.lexer.Loc()
	p.pushScopeForParsePass(ast.ScopeFunctionBody, loc)
	defer p.popScope()

	p.lexer.Expect(lexer.TOpenBrace)
	stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
	p.lexer.Next()

	p.allowIn = oldAllowIn
	p.fnOptsParse = oldFnOpts
	return ast.FnBody{Loc: loc, Stmts: stmts}
}

func (p *parser) forbidLexicalDecl(loc ast.Loc) {
	p.log.AddError(&p.source, loc, "Cannot use a declaration in a single-statement context")
}

func (p *parser) parseStmtsUpTo(end lexer.T, opts parseStmtOpts) []ast.Stmt {
	stmts := []ast.Stmt{}
	returnWithoutSemicolonStart := int32(-1)
	opts.lexicalDecl = lexicalDeclAllowAll

	for {
		// Preserve some statement-level comments
		comments := p.lexer.CommentsToPreserveBefore
		if len(comments) > 0 {
			for _, comment := range comments {
				stmts = append(stmts, ast.Stmt{
					Loc:  comment.Loc,
					Data: &ast.SComment{Text: comment.Text},
				})
			}
		}

		if p.lexer.Token == end {
			break
		}

		stmt := p.parseStmt(opts)

		// Skip TypeScript types entirely
		if p.TS.Parse {
			if _, ok := stmt.Data.(*ast.STypeScript); ok {
				continue
			}
		}

		stmts = append(stmts, stmt)

		// Warn about ASI and return statements
		if s, ok := stmt.Data.(*ast.SReturn); ok && s.Value == nil && !p.latestReturnHadSemicolon {
			returnWithoutSemicolonStart = stmt.Loc.Start
		} else {
			if returnWithoutSemicolonStart != -1 {
				if _, ok := stmt.Data.(*ast.SExpr); ok {
					p.log.AddWarning(&p.source, ast.Loc{Start: returnWithoutSemicolonStart + 6},
						"The following expression is not returned because of an automatically-inserted semicolon")
				}
			}
			returnWithoutSemicolonStart = -1
		}
	}

	return stmts
}

type generateTempRefArg uint8

const (
	tempRefNeedsDeclare generateTempRefArg = iota
	tempRefNoDeclare
)

func (p *parser) generateTempRef(declare generateTempRefArg, optionalName string) ast.Ref {
	scope := p.currentScope
	for !scope.Kind.StopsHoisting() {
		scope = scope.Parent
	}
	if optionalName == "" {
		optionalName = "_" + ast.DefaultNameMinifier.NumberToMinifiedName(p.tempRefCount)
		p.tempRefCount++
	}
	ref := p.newSymbol(ast.SymbolOther, optionalName)
	if declare == tempRefNeedsDeclare {
		p.tempRefsToDeclare = append(p.tempRefsToDeclare, ref)
	}
	scope.Generated = append(scope.Generated, ref)
	return ref
}

func (p *parser) pushScopeForVisitPass(kind ast.ScopeKind, loc ast.Loc) {
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
	isInsideWithScope bool
}

func (p *parser) findSymbol(name string) findSymbolResult {
	var ref ast.Ref
	isInsideWithScope := false
	s := p.currentScope

	for {
		// Track if we're inside a "with" statement body
		if s.Kind == ast.ScopeWith {
			isInsideWithScope = true
		}

		// Is the symbol a member of this scope?
		if member, ok := s.Members[name]; ok {
			ref = member.Ref
			break
		}

		s = s.Parent
		if s == nil {
			// Allocate an "unbound" symbol
			ref = p.newSymbol(ast.SymbolUnbound, name)
			p.moduleScope.Members[name] = ast.ScopeMember{Ref: ref, Loc: ast.Loc{Start: -1}}
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
	return findSymbolResult{ref, isInsideWithScope}
}

func (p *parser) findLabelSymbol(loc ast.Loc, name string) (ref ast.Ref, isLoop bool, ok bool) {
	for s := p.currentScope; s != nil && !s.Kind.StopsHoisting(); s = s.Parent {
		if s.Kind == ast.ScopeLabel && name == p.symbols[s.LabelRef.InnerIndex].OriginalName {
			// Track how many times we've referenced this symbol
			p.recordUsage(s.LabelRef)
			ref = s.LabelRef
			isLoop = s.LabelStmtIsLoop
			ok = true
			return
		}
	}

	r := lexer.RangeOfIdentifier(p.source, loc)
	p.log.AddRangeError(&p.source, r, fmt.Sprintf("There is no containing label named %q", name))

	// Allocate an "unbound" symbol
	ref = p.newSymbol(ast.SymbolUnbound, name)

	// Track how many times we've referenced this symbol
	p.recordUsage(ref)
	return
}

func findIdentifiers(binding ast.Binding, identifiers []ast.Decl) []ast.Decl {
	switch b := binding.Data.(type) {
	case *ast.BIdentifier:
		identifiers = append(identifiers, ast.Decl{Binding: binding})

	case *ast.BArray:
		for _, item := range b.Items {
			identifiers = findIdentifiers(item.Binding, identifiers)
		}

	case *ast.BObject:
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
func shouldKeepStmtInDeadControlFlow(stmt ast.Stmt) bool {
	switch s := stmt.Data.(type) {
	case *ast.SEmpty, *ast.SExpr, *ast.SThrow, *ast.SReturn,
		*ast.SBreak, *ast.SContinue, *ast.SClass, *ast.SDebugger:
		// Omit these statements entirely
		return false

	case *ast.SLocal:
		if s.Kind != ast.LocalVar {
			// Omit these statements entirely
			return false
		}

		// Omit everything except the identifiers
		identifiers := []ast.Decl{}
		for _, decl := range s.Decls {
			identifiers = findIdentifiers(decl.Binding, identifiers)
		}
		s.Decls = identifiers
		return true

	default:
		// Everything else must be kept
		return true
	}
}

func (p *parser) visitStmtsAndPrependTempRefs(stmts []ast.Stmt) []ast.Stmt {
	oldTempRefs := p.tempRefsToDeclare
	oldTempRefCount := p.tempRefCount
	p.tempRefsToDeclare = []ast.Ref{}
	p.tempRefCount = 0

	stmts = p.visitStmts(stmts)

	// Prepend the generated temporary variables to the beginning of the statement list
	if len(p.tempRefsToDeclare) > 0 {
		decls := []ast.Decl{}
		for _, ref := range p.tempRefsToDeclare {
			decls = append(decls, ast.Decl{Binding: ast.Binding{Data: &ast.BIdentifier{Ref: ref}}})
			p.recordDeclaredSymbol(ref)
		}

		// If the first statement is a super() call, make sure it stays that way
		stmt := ast.Stmt{Data: &ast.SLocal{Kind: ast.LocalVar, Decls: decls}}
		if len(stmts) > 0 && ast.IsSuperCall(stmts[0]) {
			stmts = append([]ast.Stmt{stmts[0], stmt}, stmts[1:]...)
		} else {
			stmts = append([]ast.Stmt{stmt}, stmts...)
		}
	}

	p.tempRefsToDeclare = oldTempRefs
	p.tempRefCount = oldTempRefCount
	return stmts
}

func (p *parser) visitStmts(stmts []ast.Stmt) []ast.Stmt {
	// Visit all statements first
	visited := make([]ast.Stmt, 0, len(stmts))
	var after []ast.Stmt
	for _, stmt := range stmts {
		if _, ok := stmt.Data.(*ast.SExportEquals); ok {
			// TypeScript "export = value;" becomes "module.exports = value;". This
			// must happen at the end after everything is parsed because TypeScript
			// moves this statement to the end when it generates code.
			after = p.visitAndAppendStmt(after, stmt)
		} else {
			visited = p.visitAndAppendStmt(visited, stmt)
		}
	}
	visited = append(visited, after...)

	// Stop now if we're not mangling
	if !p.MangleSyntax {
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
			if s, ok := stmt.Data.(*ast.SLocal); ok && s.Kind == ast.LocalVar && end > 0 {
				prevStmt := visited[end-1]
				if prevS, ok := prevStmt.Data.(*ast.SLocal); ok && prevS.Kind == ast.LocalVar && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

			visited[end] = stmt
			end++
		}
		return visited[:end]
	}

	// Merge adjacent statements during mangling
	result := make([]ast.Stmt, 0, len(visited))
	isControlFlowDead := false
	for _, stmt := range visited {
		if isControlFlowDead && !shouldKeepStmtInDeadControlFlow(stmt) {
			// Strip unnecessary statements if the control flow is dead here
			continue
		}

		switch s := stmt.Data.(type) {
		case *ast.SEmpty:
			// Strip empty statements
			continue

		case *ast.SLocal:
			// Merge adjacent local statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SLocal); ok && s.Kind == prevS.Kind && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

		case *ast.SExpr:
			// Merge adjacent expression statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok && !ast.IsSuperCall(prevStmt) {
					prevS.Value = ast.JoinWithComma(prevS.Value, s.Value)
					continue
				}
			}

		case *ast.SIf:
			// Absorb a previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok && !ast.IsSuperCall(prevStmt) {
					s.Test = ast.JoinWithComma(prevS.Value, s.Test)
					result = result[:len(result)-1]
				}
			}

			if isJumpStatement(s.Yes.Data) && s.No != nil {
				// "if (a) return b; else if (c) return d; else return e;" => "if (a) return b; if (c) return d; return e;"
				for {
					result = append(result, stmt)
					stmt = *s.No
					s.No = nil
					var ok bool
					s, ok = stmt.Data.(*ast.SIf)
					if !ok || !isJumpStatement(s.Yes.Data) || s.No == nil {
						break
					}
				}
				result = appendIfBodyPreservingScope(result, stmt)
				continue
			}

		case *ast.SReturn:
			// Merge return statements with the previous expression statement
			if len(result) > 0 && s.Value != nil {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					value := ast.JoinWithComma(prevS.Value, *s.Value)
					result[len(result)-1] = ast.Stmt{Loc: prevStmt.Loc, Data: &ast.SReturn{Value: &value}}
					continue
				}
			}

			isControlFlowDead = true

		case *ast.SThrow:
			// Merge throw statements with the previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					result[len(result)-1] = ast.Stmt{Loc: prevStmt.Loc, Data: &ast.SThrow{Value: ast.JoinWithComma(prevS.Value, s.Value)}}
					continue
				}
			}

			isControlFlowDead = true

		case *ast.SFor:
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					// Insert the previous expression into the for loop initializer
					if s.Init == nil {
						result[len(result)-1] = stmt
						s.Init = &ast.Stmt{Loc: prevStmt.Loc, Data: &ast.SExpr{Value: prevS.Value}}
						continue
					} else if s2, ok := s.Init.Data.(*ast.SExpr); ok {
						result[len(result)-1] = stmt
						s.Init = &ast.Stmt{Loc: prevStmt.Loc, Data: &ast.SExpr{Value: ast.JoinWithComma(prevS.Value, s2.Value)}}
						continue
					}
				} else {
					// Insert the previous variable declaration into the for loop
					// initializer if it's a "var" declaration, since the scope
					// doesn't matter due to scope hoisting
					if s.Init == nil {
						if s2, ok := prevStmt.Data.(*ast.SLocal); ok && s2.Kind == ast.LocalVar && !s2.IsExport {
							result[len(result)-1] = stmt
							s.Init = &prevStmt
							continue
						}
					} else {
						if s2, ok := prevStmt.Data.(*ast.SLocal); ok && s2.Kind == ast.LocalVar && !s2.IsExport {
							if s3, ok := s.Init.Data.(*ast.SLocal); ok && s3.Kind == ast.LocalVar {
								result[len(result)-1] = stmt
								s.Init.Data = &ast.SLocal{Kind: ast.LocalVar, Decls: append(s2.Decls, s3.Decls...)}
								continue
							}
						}
					}
				}
			}
		}

		result = append(result, stmt)
	}

	// Merge certain statements in reverse order
	if len(result) >= 2 {
		lastStmt := result[len(result)-1]

		if lastReturn, ok := lastStmt.Data.(*ast.SReturn); ok {
			// "if (a) return b; if (c) return d; return e;" => "return a ? b : c ? d : e;"
		returnLoop:
			for len(result) >= 2 {
				prevIndex := len(result) - 2
				prevStmt := result[prevIndex]

				switch prevS := prevStmt.Data.(type) {
				case *ast.SExpr:
					// This return statement must have a value
					if lastReturn.Value == nil {
						break returnLoop
					}

					// "a(); return b;" => "return a(), b;"
					lastValue := ast.JoinWithComma(prevS.Value, *lastReturn.Value)
					lastReturn = &ast.SReturn{Value: &lastValue}

					// Merge the last two statements
					lastStmt = ast.Stmt{Loc: prevStmt.Loc, Data: lastReturn}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				case *ast.SIf:
					// The previous statement must be an if statement with no else clause
					if prevS.No != nil {
						break returnLoop
					}

					// The then clause must be a return
					prevReturn, ok := prevS.Yes.Data.(*ast.SReturn)
					if !ok {
						break returnLoop
					}

					// Handle some or all of the values being undefined
					left := prevReturn.Value
					right := lastReturn.Value
					if left == nil {
						// "if (a) return; return b;" => "return a ? void 0 : b;"
						left = &ast.Expr{Loc: prevS.Yes.Loc, Data: &ast.EUndefined{}}
					}
					if right == nil {
						// "if (a) return a; return;" => "return a ? b : void 0;"
						right = &ast.Expr{Loc: lastStmt.Loc, Data: &ast.EUndefined{}}
					}

					// "if (!a) return b; return c;" => "return a ? c : b;"
					if not, ok := prevS.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
						prevS.Test = not.Value
						left, right = right, left
					}

					// Handle the returned values being the same
					if boolean, ok := checkEqualityIfNoSideEffects(left.Data, right.Data); ok && boolean {
						// "if (a) return b; return b;" => "return a, b;"
						lastValue := ast.JoinWithComma(prevS.Test, *left)
						lastReturn = &ast.SReturn{Value: &lastValue}
					} else {
						if comma, ok := prevS.Test.Data.(*ast.EBinary); ok && comma.Op == ast.BinOpComma {
							// "if (a, b) return c; return d;" => "return a, b ? c : d;"
							value := ast.JoinWithComma(comma.Left, p.mangleIfExpr(comma.Right.Loc, &ast.EIf{Test: comma.Right, Yes: *left, No: *right}))
							lastReturn = &ast.SReturn{Value: &value}
						} else {
							// "if (a) return b; return c;" => "return a ? b : c;"
							value := p.mangleIfExpr(prevS.Test.Loc, &ast.EIf{Test: prevS.Test, Yes: *left, No: *right})
							lastReturn = &ast.SReturn{Value: &value}
						}
					}

					// Merge the last two statements
					lastStmt = ast.Stmt{Loc: prevStmt.Loc, Data: lastReturn}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				default:
					break returnLoop
				}
			}
		} else if lastThrow, ok := lastStmt.Data.(*ast.SThrow); ok {
			// "if (a) throw b; if (c) throw d; throw e;" => "throw a ? b : c ? d : e;"
		throwLoop:
			for len(result) >= 2 {
				prevIndex := len(result) - 2
				prevStmt := result[prevIndex]

				switch prevS := prevStmt.Data.(type) {
				case *ast.SExpr:
					// "a(); throw b;" => "throw a(), b;"
					lastThrow = &ast.SThrow{Value: ast.JoinWithComma(prevS.Value, lastThrow.Value)}

					// Merge the last two statements
					lastStmt = ast.Stmt{Loc: prevStmt.Loc, Data: lastThrow}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				case *ast.SIf:
					// The previous statement must be an if statement with no else clause
					if prevS.No != nil {
						break throwLoop
					}

					// The then clause must be a throw
					prevThrow, ok := prevS.Yes.Data.(*ast.SThrow)
					if !ok {
						break throwLoop
					}

					left := prevThrow.Value
					right := lastThrow.Value

					// "if (!a) throw b; throw c;" => "throw a ? c : b;"
					if not, ok := prevS.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
						prevS.Test = not.Value
						left, right = right, left
					}

					// Merge the last two statements
					if comma, ok := prevS.Test.Data.(*ast.EBinary); ok && comma.Op == ast.BinOpComma {
						// "if (a, b) return c; return d;" => "return a, b ? c : d;"
						lastThrow = &ast.SThrow{Value: ast.JoinWithComma(comma.Left, p.mangleIfExpr(comma.Right.Loc, &ast.EIf{Test: comma.Right, Yes: left, No: right}))}
					} else {
						// "if (a) return b; return c;" => "return a ? b : c;"
						lastThrow = &ast.SThrow{Value: p.mangleIfExpr(prevS.Test.Loc, &ast.EIf{Test: prevS.Test, Yes: left, No: right})}
					}
					lastStmt = ast.Stmt{Loc: prevStmt.Loc, Data: lastThrow}
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

func (p *parser) visitLoopBody(stmt ast.Stmt) ast.Stmt {
	oldIsInsideLoop := p.fnOptsVisit.isInsideLoop
	p.fnOptsVisit.isInsideLoop = true
	stmt = p.visitSingleStmt(stmt)
	p.fnOptsVisit.isInsideLoop = oldIsInsideLoop
	return stmt
}

func (p *parser) visitSingleStmt(stmt ast.Stmt) ast.Stmt {
	// Introduce a fake block scope for function declarations inside if statements
	fn, ok := stmt.Data.(*ast.SFunction)
	hasIfScope := ok && fn.Fn.HasIfScope
	if hasIfScope {
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
	}

	stmts := p.visitStmts([]ast.Stmt{stmt})

	// Balance the fake block scope introduced above
	if hasIfScope {
		p.popScope()
	}

	// This statement could potentially expand to several statements
	switch len(stmts) {
	case 0:
		return ast.Stmt{Loc: stmt.Loc, Data: &ast.SEmpty{}}
	case 1:
		return stmts[0]
	default:
		return ast.Stmt{Loc: stmt.Loc, Data: &ast.SBlock{Stmts: stmts}}
	}
}

func (p *parser) visitForLoopInit(stmt ast.Stmt, isInOrOf bool) ast.Stmt {
	switch s := stmt.Data.(type) {
	case *ast.SExpr:
		assignTarget := ast.AssignTargetNone
		if isInOrOf {
			assignTarget = ast.AssignTargetReplace
		}
		s.Value, _ = p.visitExprInOut(s.Value, exprIn{assignTarget: assignTarget})

	case *ast.SLocal:
		for _, d := range s.Decls {
			p.visitBinding(d.Binding)
			if d.Value != nil {
				*d.Value = p.visitExpr(*d.Value)
			}
		}
		s.Decls = p.lowerObjectRestInDecls(s.Decls)

	default:
		panic("Internal error")
	}

	return stmt
}

func (p *parser) recordDeclaredSymbol(ref ast.Ref) {
	p.declaredSymbols = append(p.declaredSymbols, ast.DeclaredSymbol{
		Ref:        ref,
		IsTopLevel: p.currentScope == p.moduleScope,
	})
}

func (p *parser) visitBinding(binding ast.Binding) {
	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		p.recordDeclaredSymbol(b.Ref)

	case *ast.BArray:
		for _, item := range b.Items {
			p.visitBinding(item.Binding)
			if item.DefaultValue != nil {
				*item.DefaultValue = p.visitExpr(*item.DefaultValue)
			}
		}

	case *ast.BObject:
		for i, property := range b.Properties {
			if !property.IsSpread {
				property.Key = p.visitExpr(property.Key)
			}
			p.visitBinding(property.Value)
			if property.DefaultValue != nil {
				*property.DefaultValue = p.visitExpr(*property.DefaultValue)
			}
			b.Properties[i] = property
		}

	default:
		panic("Internal error")
	}
}

func statementCaresAboutScope(stmt ast.Stmt) bool {
	switch s := stmt.Data.(type) {
	case *ast.SBlock, *ast.SEmpty, *ast.SDebugger, *ast.SExpr, *ast.SIf,
		*ast.SFor, *ast.SForIn, *ast.SForOf, *ast.SDoWhile, *ast.SWhile,
		*ast.SWith, *ast.STry, *ast.SSwitch, *ast.SReturn, *ast.SThrow,
		*ast.SBreak, *ast.SContinue, *ast.SDirective:
		return false

	case *ast.SLocal:
		return s.Kind != ast.LocalVar

	default:
		return true
	}
}

func dropFirstStatement(body ast.Stmt, replace *ast.Stmt) ast.Stmt {
	if block, ok := body.Data.(*ast.SBlock); ok && len(block.Stmts) > 0 {
		if replace != nil {
			block.Stmts[0] = *replace
		} else if len(block.Stmts) == 2 && !statementCaresAboutScope(block.Stmts[1]) {
			return block.Stmts[1]
		} else {
			block.Stmts = block.Stmts[1:]
		}
		return body
	}
	if replace != nil {
		return *replace
	}
	return ast.Stmt{Loc: body.Loc, Data: &ast.SEmpty{}}
}

func mangleFor(s *ast.SFor) {
	// Get the first statement in the loop
	first := s.Body
	if block, ok := first.Data.(*ast.SBlock); ok && len(block.Stmts) > 0 {
		first = block.Stmts[0]
	}

	if ifS, ok := first.Data.(*ast.SIf); ok {
		// "for (;;) if (x) break;" => "for (; !x;) ;"
		// "for (; a;) if (x) break;" => "for (; a && !x;) ;"
		// "for (;;) if (x) break; else y();" => "for (; !x;) y();"
		// "for (; a;) if (x) break; else y();" => "for (; a && !x;) y();"
		if breakS, ok := ifS.Yes.Data.(*ast.SBreak); ok && breakS.Label == nil {
			var not ast.Expr
			if unary, ok := ifS.Test.Data.(*ast.EUnary); ok && unary.Op == ast.UnOpNot {
				not = unary.Value
			} else {
				not = ast.Not(ifS.Test)
			}
			if s.Test != nil {
				s.Test = &ast.Expr{Loc: s.Test.Loc, Data: &ast.EBinary{Op: ast.BinOpLogicalAnd, Left: *s.Test, Right: not}}
			} else {
				s.Test = &not
			}
			s.Body = dropFirstStatement(s.Body, ifS.No)
			return
		}

		// "for (;;) if (x) y(); else break;" => "for (; x;) y();"
		// "for (; a;) if (x) y(); else break;" => "for (; a && x;) y();"
		if ifS.No != nil {
			if breakS, ok := ifS.No.Data.(*ast.SBreak); ok && breakS.Label == nil {
				if s.Test != nil {
					s.Test = &ast.Expr{Loc: s.Test.Loc, Data: &ast.EBinary{Op: ast.BinOpLogicalAnd, Left: *s.Test, Right: ifS.Test}}
				} else {
					s.Test = &ifS.Test
				}
				s.Body = dropFirstStatement(s.Body, &ifS.Yes)
				return
			}
		}
	}
}

func appendIfBodyPreservingScope(stmts []ast.Stmt, body ast.Stmt) []ast.Stmt {
	if block, ok := body.Data.(*ast.SBlock); ok {
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
		return append(stmts, ast.Stmt{Loc: body.Loc, Data: &ast.SBlock{Stmts: []ast.Stmt{body}}})
	}

	return append(stmts, body)
}

func (p *parser) mangleIf(stmts []ast.Stmt, loc ast.Loc, s *ast.SIf, isTestBooleanConstant bool, testBooleanValue bool) []ast.Stmt {
	// Constant folding using the test expression
	if isTestBooleanConstant {
		if testBooleanValue {
			// The test is true
			if s.No == nil || !shouldKeepStmtInDeadControlFlow(*s.No) {
				// We can drop the "no" branch
				return appendIfBodyPreservingScope(stmts, s.Yes)
			} else {
				// We have to keep the "no" branch
			}
		} else {
			// The test is false
			if !shouldKeepStmtInDeadControlFlow(s.Yes) {
				// We can drop the "yes" branch
				if s.No == nil {
					return stmts
				}
				return appendIfBodyPreservingScope(stmts, *s.No)
			} else {
				// We have to keep the "yes" branch
			}
		}
	}

	if yes, ok := s.Yes.Data.(*ast.SExpr); ok {
		// "yes" is an expression
		if s.No == nil {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) b();" => "a || b();"
				return append(stmts, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpLogicalOr,
					Left:  not.Value,
					Right: yes.Value,
				}}}})
			} else {
				// "if (a) b();" => "a && b();"
				return append(stmts, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpLogicalAnd,
					Left:  s.Test,
					Right: yes.Value,
				}}}})
			}
		} else if no, ok := s.No.Data.(*ast.SExpr); ok {
			// "if (a) b(); else c();" => "a ? b() : c();"
			return append(stmts, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: p.mangleIfExpr(loc, &ast.EIf{
				Test: s.Test,
				Yes:  yes.Value,
				No:   no.Value,
			})}})
		}
	} else if _, ok := s.Yes.Data.(*ast.SEmpty); ok {
		// "yes" is missing
		if s.No == nil {
			// "yes" and "no" are both missing
			if p.exprCanBeRemovedIfUnused(s.Test) {
				// "if (1) {}" => ""
				return stmts
			} else {
				// "if (a) {}" => "a;"
				return append(stmts, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: s.Test}})
			}
		} else if no, ok := s.No.Data.(*ast.SExpr); ok {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) {} else b();" => "a && b();"
				return append(stmts, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpLogicalAnd,
					Left:  not.Value,
					Right: no.Value,
				}}}})
			} else {
				// "if (a) {} else b();" => "a || b();"
				return append(stmts, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpLogicalOr,
					Left:  s.Test,
					Right: no.Value,
				}}}})
			}
		} else {
			// "yes" is missing and "no" is not missing (and is not an expression)
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) {} else throw b;" => "if (a) throw b;"
				s.Test = not.Value
				s.Yes = *s.No
				s.No = nil
			} else {
				// "if (a) {} else throw b;" => "if (!a) throw b;"
				s.Test = ast.Expr{Loc: s.Test.Loc, Data: &ast.EUnary{Op: ast.UnOpNot, Value: s.Test}}
				s.Yes = *s.No
				s.No = nil
			}
		}
	} else {
		// "yes" is not missing (and is not an expression)
		if s.No != nil {
			// "yes" is not missing (and is not an expression) and "no" is not missing
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) return b; else return c;" => "if (a) return c; else return b;"
				s.Test = not.Value
				s.Yes, *s.No = *s.No, s.Yes
			}
		} else {
			// "no" is missing
			if s2, ok := s.Yes.Data.(*ast.SIf); ok && s2.No == nil {
				// "if (a) if (b) return c;" => "if (a && b) return c;"
				s.Test.Data = &ast.EBinary{
					Op:    ast.BinOpLogicalAnd,
					Left:  s.Test,
					Right: s2.Test,
				}
				s.Yes = s2.Yes
			}
		}
	}

	return append(stmts, ast.Stmt{Loc: loc, Data: s})
}

func (p *parser) mangleIfExpr(loc ast.Loc, e *ast.EIf) ast.Expr {
	// "!a ? b : c" => "a ? c : b"
	if not, ok := e.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
		e.Test = not.Value
		e.Yes, e.No = e.No, e.Yes
	}

	if valuesLookTheSame(e.Yes.Data, e.No.Data) {
		// "/* @__PURE__ */ a() ? b : b" => "b"
		if p.exprCanBeRemovedIfUnused(e.Test) {
			return e.Yes
		}

		// "a ? b : b" => "a, b"
		return ast.JoinWithComma(e.Test, e.Yes)
	}

	// "a ? true : false" => "!!a"
	// "a ? false : true" => "!a"
	if yes, ok := e.Yes.Data.(*ast.EBoolean); ok {
		if no, ok := e.No.Data.(*ast.EBoolean); ok {
			if yes.Value && !no.Value {
				return ast.Not(ast.Not(e.Test))
			}
			if !yes.Value && no.Value {
				return ast.Not(e.Test)
			}
		}
	}

	if id, ok := e.Test.Data.(*ast.EIdentifier); ok {
		// "a ? a : b" => "a || b"
		if id2, ok := e.Yes.Data.(*ast.EIdentifier); ok && id.Ref == id2.Ref {
			return ast.Expr{Loc: loc, Data: &ast.EBinary{Op: ast.BinOpLogicalOr, Left: e.Test, Right: e.No}}
		}

		// "a ? b : a" => "a && b"
		if id2, ok := e.No.Data.(*ast.EIdentifier); ok && id.Ref == id2.Ref {
			return ast.Expr{Loc: loc, Data: &ast.EBinary{Op: ast.BinOpLogicalAnd, Left: e.Test, Right: e.Yes}}
		}
	}

	// "a ? b ? c : d : d" => "a && b ? c : d"
	if yesIf, ok := e.Yes.Data.(*ast.EIf); ok && valuesLookTheSame(yesIf.No.Data, e.No.Data) {
		e.Test.Data = &ast.EBinary{Op: ast.BinOpLogicalAnd, Left: e.Test, Right: yesIf.Test}
		e.Yes = yesIf.Yes
		return ast.Expr{Loc: loc, Data: e}
	}

	// "a ? b : c ? b : d" => "a || c ? b : d"
	if noIf, ok := e.No.Data.(*ast.EIf); ok && valuesLookTheSame(e.Yes.Data, noIf.Yes.Data) {
		e.Test.Data = &ast.EBinary{Op: ast.BinOpLogicalOr, Left: e.Test, Right: noIf.Test}
		e.No = noIf.No
		return ast.Expr{Loc: loc, Data: e}
	}

	// "a ? c : (b, c)" => "(a || b), c"
	if comma, ok := e.No.Data.(*ast.EBinary); ok && comma.Op == ast.BinOpComma && valuesLookTheSame(e.Yes.Data, comma.Right.Data) {
		return ast.JoinWithComma(ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    ast.BinOpLogicalOr,
			Left:  e.Test,
			Right: comma.Left,
		}}, comma.Right)
	}

	// "a ? (b, c) : c" => "(a && b), c"
	if comma, ok := e.Yes.Data.(*ast.EBinary); ok && comma.Op == ast.BinOpComma && valuesLookTheSame(comma.Right.Data, e.No.Data) {
		return ast.JoinWithComma(ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    ast.BinOpLogicalAnd,
			Left:  e.Test,
			Right: comma.Left,
		}}, comma.Right)
	}

	// "a ? b || c : c" => "(a && b) || c"
	if binary, ok := e.Yes.Data.(*ast.EBinary); ok && binary.Op == ast.BinOpLogicalOr &&
		valuesLookTheSame(binary.Right.Data, e.No.Data) {
		return ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op: ast.BinOpLogicalOr,
			Left: ast.Expr{Loc: loc, Data: &ast.EBinary{
				Op:    ast.BinOpLogicalAnd,
				Left:  e.Test,
				Right: binary.Left,
			}},
			Right: binary.Right,
		}}
	}

	// "a ? c : b && c" => "(a || b) && c"
	if binary, ok := e.No.Data.(*ast.EBinary); ok && binary.Op == ast.BinOpLogicalAnd &&
		valuesLookTheSame(e.Yes.Data, binary.Right.Data) {
		return ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op: ast.BinOpLogicalAnd,
			Left: ast.Expr{Loc: loc, Data: &ast.EBinary{
				Op:    ast.BinOpLogicalOr,
				Left:  e.Test,
				Right: binary.Left,
			}},
			Right: binary.Right,
		}}
	}

	// "a ? b(c, d) : b(e, d)" => "b(a ? c : e, d)"
	if y, ok := e.Yes.Data.(*ast.ECall); ok && len(y.Args) > 0 {
		if n, ok := e.No.Data.(*ast.ECall); ok && len(n.Args) == len(y.Args) &&
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
					yesSpread, yesIsSpread := y.Args[0].Data.(*ast.ESpread)
					noSpread, noIsSpread := n.Args[0].Data.(*ast.ESpread)

					// "a ? b(...c) : b(...e)" => "b(...a ? c : e)"
					if yesIsSpread && noIsSpread {
						e.Yes = yesSpread.Value
						e.No = noSpread.Value
						y.Args[0] = ast.Expr{Loc: loc, Data: &ast.ESpread{Value: p.mangleIfExpr(loc, e)}}
						return ast.Expr{Loc: loc, Data: y}
					}

					// "a ? b(c) : b(e)" => "b(a ? c : e)"
					if !yesIsSpread && !noIsSpread {
						e.Yes = y.Args[0]
						e.No = n.Args[0]
						y.Args[0] = p.mangleIfExpr(loc, e)
						return ast.Expr{Loc: loc, Data: y}
					}
				}
			}
		}
	}

	// Try using the "??" operator, but only if it's supported
	if !p.UnsupportedFeatures.Has(compat.NullishCoalescing) {
		if binary, ok := e.Test.Data.(*ast.EBinary); ok {
			switch binary.Op {
			case ast.BinOpLooseEq:
				// "a == null ? b : a" => "a ?? b"
				if _, ok := binary.Right.Data.(*ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Left) && valuesLookTheSame(binary.Left.Data, e.No.Data) {
					binary.Op = ast.BinOpNullishCoalescing
					binary.Right = e.Yes
					return ast.Expr{Loc: loc, Data: binary}
				}

				// "null == a ? b : a" => "a ?? b"
				if _, ok := binary.Left.Data.(*ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Right) && valuesLookTheSame(binary.Right.Data, e.No.Data) {
					binary.Op = ast.BinOpNullishCoalescing
					binary.Left = binary.Right
					binary.Right = e.Yes
					return ast.Expr{Loc: loc, Data: binary}
				}

			case ast.BinOpLooseNe:
				// "a != null ? a : b" => "a ?? b"
				if _, ok := binary.Right.Data.(*ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Left) && valuesLookTheSame(binary.Left.Data, e.Yes.Data) {
					binary.Op = ast.BinOpNullishCoalescing
					binary.Right = e.No
					return ast.Expr{Loc: loc, Data: binary}
				}

				// "null != a ? a : b" => "a ?? b"
				if _, ok := binary.Left.Data.(*ast.ENull); ok && p.exprCanBeRemovedIfUnused(binary.Right) && valuesLookTheSame(binary.Right.Data, e.Yes.Data) {
					binary.Op = ast.BinOpNullishCoalescing
					binary.Left = binary.Right
					binary.Right = e.No
					return ast.Expr{Loc: loc, Data: binary}
				}
			}
		}
	}

	return ast.Expr{Loc: loc, Data: e}
}

func (p *parser) visitAndAppendStmt(stmts []ast.Stmt, stmt ast.Stmt) []ast.Stmt {
	switch s := stmt.Data.(type) {
	case *ast.SDebugger, *ast.SEmpty, *ast.SDirective, *ast.SComment:
		// These don't contain anything to traverse

	case *ast.STypeScript:
		// Erase TypeScript constructs from the output completely
		return stmts

	case *ast.SImport:
		p.recordDeclaredSymbol(s.NamespaceRef)

		if s.DefaultName != nil {
			p.recordDeclaredSymbol(s.DefaultName.Ref)
		}

		if s.Items != nil {
			for _, item := range *s.Items {
				p.recordDeclaredSymbol(item.Name.Ref)
			}
		}

	case *ast.SExportClause:
		// "export {foo}"
		for i, item := range s.Items {
			name := p.loadNameFromRef(item.Name.Ref)
			ref := p.findSymbol(name).ref
			s.Items[i].Name.Ref = ref
			p.recordExport(item.AliasLoc, item.Alias, ref)
		}

	case *ast.SExportFrom:
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
			p.recordExport(item.AliasLoc, item.Alias, ref)
		}

	case *ast.SExportStar:
		// "export * from 'path'"
		// "export * as ns from 'path'"
		name := p.loadNameFromRef(s.NamespaceRef)
		s.NamespaceRef = p.newSymbol(ast.SymbolOther, name)
		p.currentScope.Generated = append(p.currentScope.Generated, s.NamespaceRef)
		p.recordDeclaredSymbol(s.NamespaceRef)

		// "export * as ns from 'path'"
		if s.Alias != nil {
			p.recordExport(s.Alias.Loc, s.Alias.Name, s.NamespaceRef)
		}

	case *ast.SExportDefault:
		p.recordDeclaredSymbol(s.DefaultName.Ref)

		switch {
		case s.Value.Expr != nil:
			*s.Value.Expr = p.visitExpr(*s.Value.Expr)

			// Discard type-only export default statements
			if p.TS.Parse {
				if id, ok := (*s.Value.Expr).Data.(*ast.EIdentifier); ok {
					symbol := p.symbols[id.Ref.InnerIndex]
					if symbol.Kind == ast.SymbolUnbound && p.localTypeNames[symbol.OriginalName] {
						return stmts
					}
				}
			}

		case s.Value.Stmt != nil:
			switch s2 := s.Value.Stmt.Data.(type) {
			case *ast.SFunction:
				p.visitFn(&s2.Fn, s2.Fn.OpenParenLoc)

			case *ast.SClass:
				p.visitClass(&s2.Class)

				// Lower class field syntax for browsers that don't support it
				classStmts, _ := p.lowerClass(stmt, ast.Expr{})
				return append(stmts, classStmts...)

			default:
				panic("Internal error")
			}
		}

	case *ast.SExportEquals:
		// "module.exports = value"
		stmts = append(stmts, ast.AssignStmt(
			ast.Expr{Loc: stmt.Loc, Data: &ast.EDot{
				Target:  ast.Expr{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: p.moduleRef}},
				Name:    "exports",
				NameLoc: stmt.Loc,
			}},
			p.visitExpr(s.Value),
		))
		p.recordUsage(p.moduleRef)
		return stmts

	case *ast.SBreak:
		if s.Label != nil {
			name := p.loadNameFromRef(s.Label.Ref)
			s.Label.Ref, _, _ = p.findLabelSymbol(s.Label.Loc, name)
		} else if !p.fnOptsVisit.isInsideLoop && !p.fnOptsVisit.isInsideSwitch {
			r := lexer.RangeOfIdentifier(p.source, stmt.Loc)
			p.log.AddRangeError(&p.source, r, "Cannot use \"break\" here")
		}

	case *ast.SContinue:
		if s.Label != nil {
			name := p.loadNameFromRef(s.Label.Ref)
			var isLoop, ok bool
			s.Label.Ref, isLoop, ok = p.findLabelSymbol(s.Label.Loc, name)
			if ok && !isLoop {
				r := lexer.RangeOfIdentifier(p.source, s.Label.Loc)
				p.log.AddRangeError(&p.source, r, fmt.Sprintf("Cannot continue to label \"%s\"", name))
			}
		} else if !p.fnOptsVisit.isInsideLoop {
			r := lexer.RangeOfIdentifier(p.source, stmt.Loc)
			p.log.AddRangeError(&p.source, r, "Cannot use \"continue\" here")
		}

	case *ast.SLabel:
		p.pushScopeForVisitPass(ast.ScopeLabel, stmt.Loc)
		name := p.loadNameFromRef(s.Name.Ref)
		ref := p.newSymbol(ast.SymbolLabel, name)
		s.Name.Ref = ref
		p.currentScope.LabelRef = ref
		switch s.Stmt.Data.(type) {
		case *ast.SFor, *ast.SForIn, *ast.SForOf, *ast.SWhile, *ast.SDoWhile:
			p.currentScope.LabelStmtIsLoop = true
		}
		s.Stmt = p.visitSingleStmt(s.Stmt)
		p.popScope()

	case *ast.SLocal:
		for i, d := range s.Decls {
			p.visitBinding(d.Binding)
			if d.Value != nil {
				*d.Value = p.visitExpr(*d.Value)

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
				if p.MangleSyntax && s.Kind == ast.LocalLet {
					if _, ok := d.Binding.Data.(*ast.BIdentifier); ok {
						if _, ok := d.Value.Data.(*ast.EUndefined); ok {
							s.Decls[i].Value = nil
						}
					}
				}
			}
		}

		// Handle being exported inside a namespace
		if s.IsExport && p.enclosingNamespaceRef != nil {
			wrapIdentifier := func(loc ast.Loc, ref ast.Ref) ast.Expr {
				p.recordUsage(*p.enclosingNamespaceRef)
				return ast.Expr{Loc: loc, Data: &ast.EDot{
					Target:  ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: *p.enclosingNamespaceRef}},
					Name:    p.symbols[ref.InnerIndex].OriginalName,
					NameLoc: loc,
				}}
			}
			for _, decl := range s.Decls {
				if decl.Value != nil {
					target := p.convertBindingToExpr(decl.Binding, wrapIdentifier)
					if result, ok := p.lowerObjectRestInAssign(target, *decl.Value); ok {
						target = result
					} else {
						target = ast.Assign(target, *decl.Value)
					}
					stmts = append(stmts, ast.Stmt{Loc: stmt.Loc, Data: &ast.SExpr{Value: target}})
				}
			}
			return stmts
		}

		s.Decls = p.lowerObjectRestInDecls(s.Decls)

	case *ast.SExpr:
		s.Value = p.visitExpr(s.Value)

		// Trim expressions without side effects
		if p.MangleSyntax {
			s.Value = p.simplifyUnusedExpr(s.Value)
			if s.Value.Data == nil {
				stmt = ast.Stmt{Loc: stmt.Loc, Data: &ast.SEmpty{}}
			}
		}

	case *ast.SThrow:
		s.Value = p.visitExpr(s.Value)

	case *ast.SReturn:
		if s.Value != nil {
			*s.Value = p.visitExpr(*s.Value)

			// Returning undefined is implicit
			if p.MangleSyntax {
				if _, ok := s.Value.Data.(*ast.EUndefined); ok {
					s.Value = nil
				}
			}
		}

	case *ast.SBlock:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		s.Stmts = p.visitStmts(s.Stmts)
		p.popScope()

		if p.MangleSyntax {
			if len(s.Stmts) == 1 && !statementCaresAboutScope(s.Stmts[0]) {
				// Unwrap blocks containing a single statement
				stmt = s.Stmts[0]
			} else if len(s.Stmts) == 0 {
				// Trim empty blocks
				stmt = ast.Stmt{Loc: stmt.Loc, Data: &ast.SEmpty{}}
			}
		}

	case *ast.SWith:
		s.Value = p.visitExpr(s.Value)
		p.pushScopeForVisitPass(ast.ScopeWith, s.BodyLoc)
		s.Body = p.visitSingleStmt(s.Body)
		p.popScope()

	case *ast.SWhile:
		s.Test = p.visitBooleanExpr(s.Test)
		s.Body = p.visitLoopBody(s.Body)

		if p.MangleSyntax {
			// "while (a) {}" => "for (;a;) {}"
			test := &s.Test
			if boolean, ok := toBooleanWithoutSideEffects(s.Test.Data); ok && boolean {
				test = nil
			}
			forS := &ast.SFor{Test: test, Body: s.Body}
			mangleFor(forS)
			stmt = ast.Stmt{Loc: stmt.Loc, Data: forS}
		}

	case *ast.SDoWhile:
		s.Body = p.visitLoopBody(s.Body)
		s.Test = p.visitBooleanExpr(s.Test)

	case *ast.SIf:
		s.Test = p.visitBooleanExpr(s.Test)

		// Fold constants
		boolean, ok := toBooleanWithoutSideEffects(s.Test.Data)

		// Mark the control flow as dead if the branch is never taken
		if ok && !boolean {
			old := p.isControlFlowDead
			p.isControlFlowDead = true
			s.Yes = p.visitSingleStmt(s.Yes)
			p.isControlFlowDead = old
		} else {
			s.Yes = p.visitSingleStmt(s.Yes)
		}

		// The "else" clause is optional
		if s.No != nil {
			// Mark the control flow as dead if the branch is never taken
			if ok && boolean {
				old := p.isControlFlowDead
				p.isControlFlowDead = true
				*s.No = p.visitSingleStmt(*s.No)
				p.isControlFlowDead = old
			} else {
				*s.No = p.visitSingleStmt(*s.No)
			}

			// Trim unnecessary "else" clauses
			if p.MangleSyntax {
				if _, ok := s.No.Data.(*ast.SEmpty); ok {
					s.No = nil
				}
			}
		}

		if p.MangleSyntax {
			return p.mangleIf(stmts, stmt.Loc, s, ok, boolean)
		}

	case *ast.SFor:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		if s.Init != nil {
			p.visitForLoopInit(*s.Init, false)
		}

		if s.Test != nil {
			*s.Test = p.visitBooleanExpr(*s.Test)

			// A true value is implied
			if p.MangleSyntax {
				if boolean, ok := toBooleanWithoutSideEffects(s.Test.Data); ok && boolean {
					s.Test = nil
				}
			}
		}

		if s.Update != nil {
			*s.Update = p.visitExpr(*s.Update)
		}
		s.Body = p.visitLoopBody(s.Body)
		p.popScope()

		if p.MangleSyntax {
			mangleFor(s)
		}

	case *ast.SForIn:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		p.visitForLoopInit(s.Init, true)
		s.Value = p.visitExpr(s.Value)
		s.Body = p.visitLoopBody(s.Body)
		p.popScope()
		p.lowerObjectRestInForLoopInit(s.Init, &s.Body)

	case *ast.SForOf:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		p.visitForLoopInit(s.Init, true)
		s.Value = p.visitExpr(s.Value)
		s.Body = p.visitLoopBody(s.Body)
		p.popScope()
		p.lowerObjectRestInForLoopInit(s.Init, &s.Body)

	case *ast.STry:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		p.fnOptsVisit.tryBodyCount++
		s.Body = p.visitStmts(s.Body)
		p.fnOptsVisit.tryBodyCount--
		p.popScope()

		if s.Catch != nil {
			p.pushScopeForVisitPass(ast.ScopeBlock, s.Catch.Loc)
			if s.Catch.Binding != nil {
				p.visitBinding(*s.Catch.Binding)
			}
			s.Catch.Body = p.visitStmts(s.Catch.Body)
			p.lowerObjectRestInCatchBinding(s.Catch)
			p.popScope()
		}

		if s.Finally != nil {
			p.pushScopeForVisitPass(ast.ScopeBlock, s.Finally.Loc)
			s.Finally.Stmts = p.visitStmts(s.Finally.Stmts)
			p.popScope()
		}

	case *ast.SSwitch:
		s.Test = p.visitExpr(s.Test)
		p.pushScopeForVisitPass(ast.ScopeBlock, s.BodyLoc)
		oldIsInsideSwitch := p.fnOptsVisit.isInsideSwitch
		p.fnOptsVisit.isInsideSwitch = true
		for i, c := range s.Cases {
			if c.Value != nil {
				*c.Value = p.visitExpr(*c.Value)
				p.warnAboutEqualityCheck("case", *c.Value, c.Value.Loc)
				p.checkForTypeofAndString(s.Test, *c.Value)
			}
			c.Body = p.visitStmts(c.Body)

			// Make sure the assignment to the body above is preserved
			s.Cases[i] = c
		}
		p.fnOptsVisit.isInsideSwitch = oldIsInsideSwitch
		p.popScope()

		// Check for duplicate case values
		p.duplicateCaseChecker.reset()
		for _, c := range s.Cases {
			if c.Value != nil {
				p.duplicateCaseChecker.check(p, *c.Value)
			}
		}

	case *ast.SFunction:
		p.visitFn(&s.Fn, s.Fn.OpenParenLoc)

		// Handle exporting this function from a namespace
		if s.IsExport && p.enclosingNamespaceRef != nil {
			s.IsExport = false
			stmts = append(stmts, stmt, ast.AssignStmt(
				ast.Expr{Loc: stmt.Loc, Data: &ast.EDot{
					Target:  ast.Expr{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: *p.enclosingNamespaceRef}},
					Name:    p.symbols[s.Fn.Name.Ref.InnerIndex].OriginalName,
					NameLoc: s.Fn.Name.Loc,
				}},
				ast.Expr{Loc: s.Fn.Name.Loc, Data: &ast.EIdentifier{Ref: s.Fn.Name.Ref}},
			))
			return stmts
		}

	case *ast.SClass:
		p.visitClass(&s.Class)

		// Remove the export flag inside a namespace
		wasExportInsideNamespace := s.IsExport && p.enclosingNamespaceRef != nil
		if wasExportInsideNamespace {
			s.IsExport = false
		}

		// Lower class field syntax for browsers that don't support it
		classStmts, _ := p.lowerClass(stmt, ast.Expr{})
		stmts = append(stmts, classStmts...)

		// Handle exporting this class from a namespace
		if wasExportInsideNamespace {
			stmts = append(stmts, ast.AssignStmt(
				ast.Expr{Loc: stmt.Loc, Data: &ast.EDot{
					Target:  ast.Expr{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: *p.enclosingNamespaceRef}},
					Name:    p.symbols[s.Class.Name.Ref.InnerIndex].OriginalName,
					NameLoc: s.Class.Name.Loc,
				}},
				ast.Expr{Loc: s.Class.Name.Loc, Data: &ast.EIdentifier{Ref: s.Class.Name.Ref}},
			))
			return stmts
		}

		return stmts

	case *ast.SEnum:
		p.recordDeclaredSymbol(s.Name.Ref)
		p.pushScopeForVisitPass(ast.ScopeEntry, stmt.Loc)
		defer p.popScope()
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
		valueExprs := []ast.Expr{}

		// Track values so they can be used by constant folding. We need to follow
		// links here in case the enum was merged with a preceding namespace.
		valuesSoFar := make(map[string]float64)
		p.knownEnumValues[s.Name.Ref] = valuesSoFar
		p.knownEnumValues[s.Arg] = valuesSoFar

		// We normally don't fold numeric constants because they might increase code
		// size, but it's important to fold numeric constants inside enums since
		// that's what the TypeScript compiler does.
		oldShouldFoldNumericConstants := p.shouldFoldNumericConstants
		p.shouldFoldNumericConstants = true

		// Create an assignment for each enum value
		for _, value := range s.Values {
			name := lexer.UTF16ToString(value.Name)
			var assignTarget ast.Expr
			hasStringValue := false

			if value.Value != nil {
				*value.Value = p.visitExpr(*value.Value)
				hasNumericValue = false
				switch e := value.Value.Data.(type) {
				case *ast.ENumber:
					valuesSoFar[name] = e.Value
					hasNumericValue = true
					nextNumericValue = e.Value + 1
				case *ast.EString:
					hasStringValue = true
				}
			} else if hasNumericValue {
				valuesSoFar[name] = nextNumericValue
				value.Value = &ast.Expr{Loc: value.Loc, Data: &ast.ENumber{Value: nextNumericValue}}
				nextNumericValue++
			} else {
				value.Value = &ast.Expr{Loc: value.Loc, Data: &ast.EUndefined{}}
			}

			if p.MangleSyntax && lexer.IsIdentifier(name) {
				// "Enum.Name = value"
				assignTarget = ast.Assign(
					ast.Expr{Loc: value.Loc, Data: &ast.EDot{
						Target:  ast.Expr{Loc: value.Loc, Data: &ast.EIdentifier{Ref: s.Arg}},
						Name:    name,
						NameLoc: value.Loc,
					}},
					*value.Value,
				)
			} else {
				// "Enum['Name'] = value"
				assignTarget = ast.Assign(
					ast.Expr{Loc: value.Loc, Data: &ast.EIndex{
						Target: ast.Expr{Loc: value.Loc, Data: &ast.EIdentifier{Ref: s.Arg}},
						Index:  ast.Expr{Loc: value.Loc, Data: &ast.EString{Value: value.Name}},
					}},
					*value.Value,
				)
			}
			p.recordUsage(s.Arg)

			// String-valued enums do not form a two-way map
			if hasStringValue {
				valueExprs = append(valueExprs, assignTarget)
			} else {
				// "Enum[assignTarget] = 'Name'"
				valueExprs = append(valueExprs, ast.Assign(
					ast.Expr{Loc: value.Loc, Data: &ast.EIndex{
						Target: ast.Expr{Loc: value.Loc, Data: &ast.EIdentifier{Ref: s.Arg}},
						Index:  assignTarget,
					}},
					ast.Expr{Loc: value.Loc, Data: &ast.EString{Value: value.Name}},
				))
			}
			p.recordUsage(s.Arg)
		}

		p.shouldFoldNumericConstants = oldShouldFoldNumericConstants

		// Generate statements from expressions
		valueStmts := []ast.Stmt{}
		if len(valueExprs) > 0 {
			if p.MangleSyntax {
				// "a; b; c;" => "a, b, c;"
				joined := ast.JoinAllWithComma(valueExprs)
				valueStmts = append(valueStmts, ast.Stmt{Loc: joined.Loc, Data: &ast.SExpr{Value: joined}})
			} else {
				for _, expr := range valueExprs {
					valueStmts = append(valueStmts, ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: expr}})
				}
			}
		}

		// Wrap this enum definition in a closure
		stmts = p.generateClosureForTypeScriptNamespaceOrEnum(
			stmts, stmt.Loc, s.IsExport, s.Name.Loc, s.Name.Ref, s.Arg, valueStmts)
		return stmts

	case *ast.SNamespace:
		p.recordDeclaredSymbol(s.Name.Ref)

		// Scan ahead for any variables inside this namespace. This must be done
		// ahead of time before visiting any statements inside the namespace
		// because we may end up visiting the uses before the declarations.
		// We need to convert the uses into property accesses on the namespace.
		for _, childStmt := range s.Stmts {
			if local, ok := childStmt.Data.(*ast.SLocal); ok {
				if local.IsExport {
					p.markExportedDeclsInsideNamespace(s.Arg, local.Decls)
				}
			}
		}

		oldEnclosingNamespaceRef := p.enclosingNamespaceRef
		p.enclosingNamespaceRef = &s.Arg
		p.pushScopeForVisitPass(ast.ScopeEntry, stmt.Loc)
		p.recordDeclaredSymbol(s.Arg)
		stmtsInsideNamespace := p.visitStmtsAndPrependTempRefs(s.Stmts)
		p.popScope()
		p.enclosingNamespaceRef = oldEnclosingNamespaceRef

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

func (p *parser) markExportedDeclsInsideNamespace(nsRef ast.Ref, decls []ast.Decl) {
	for _, decl := range decls {
		p.markExportedBindingInsideNamespace(nsRef, decl.Binding)
	}
}

func (p *parser) markExportedBindingInsideNamespace(nsRef ast.Ref, binding ast.Binding) {
	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		p.isExportedInsideNamespace[b.Ref] = nsRef

	case *ast.BArray:
		for _, item := range b.Items {
			p.markExportedBindingInsideNamespace(nsRef, item.Binding)
		}

	case *ast.BObject:
		for _, property := range b.Properties {
			p.markExportedBindingInsideNamespace(nsRef, property.Value)
		}

	default:
		panic("Internal error")
	}
}

func maybeJoinWithComma(a ast.Expr, b ast.Expr) ast.Expr {
	if a.Data == nil {
		return b
	}
	if b.Data == nil {
		return a
	}
	return ast.JoinWithComma(a, b)
}

// This is a helper function to use when you need to capture a value that may
// have side effects so you can use it multiple times. It guarantees that the
// side effects take place exactly once.
//
// Example usage:
//
//   // "value" => "value + value"
//   // "value()" => "(_a = value(), _a + _a)"
//   valueFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, value)
//   return wrapFunc(ast.Expr{Loc: loc, Data: &ast.EBinary{
//     Op: ast.BinOpAdd,
//     Left: valueFunc(),
//     Right: valueFunc(),
//   }})
//
// This returns a function for generating references instead of a raw reference
// because AST nodes are supposed to be unique in memory, not aliases of other
// AST nodes. That way you can mutate one during lowering without having to
// worry about messing up other nodes.
func (p *parser) captureValueWithPossibleSideEffects(
	loc ast.Loc, // The location to use for the generated references
	count int, // The expected number of references to generate
	value ast.Expr, // The value that might have side effects
) (
	func() ast.Expr, // Generates reference expressions "_a"
	func(ast.Expr) ast.Expr, // Call this on the final expression
) {
	wrapFunc := func(expr ast.Expr) ast.Expr {
		// Make sure side effects still happen if no expression was generated
		if expr.Data == nil {
			return value
		}
		return expr
	}

	// Referencing certain expressions more than once has no side effects, so we
	// can just create them inline without capturing them in a temporary variable
	var valueFunc func() ast.Expr
	switch e := value.Data.(type) {
	case *ast.ENull:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.ENull{}} }
	case *ast.EUndefined:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EUndefined{}} }
	case *ast.EThis:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EThis{}} }
	case *ast.EBoolean:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: e.Value}} }
	case *ast.ENumber:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.ENumber{Value: e.Value}} }
	case *ast.EBigInt:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EBigInt{Value: e.Value}} }
	case *ast.EString:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EString{Value: e.Value}} }
	case *ast.EIdentifier:
		valueFunc = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: e.Ref}} }
	}
	if valueFunc != nil {
		return valueFunc, wrapFunc
	}

	// We don't need to worry about side effects if the value won't be used
	// multiple times. This special case lets us avoid generating a temporary
	// reference.
	if count < 2 {
		return func() ast.Expr {
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
	if p.currentScope.Kind == ast.ScopeFunctionArgs {
		return func() ast.Expr {
				if tempRef == ast.InvalidRef {
					tempRef = p.generateTempRef(tempRefNoDeclare, "")

					// Assign inline so the order of side effects remains the same
					p.recordUsage(tempRef)
					return ast.Assign(ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tempRef}}, value)
				}
				p.recordUsage(tempRef)
				return ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tempRef}}
			}, func(expr ast.Expr) ast.Expr {
				// Make sure side effects still happen if no expression was generated
				if expr.Data == nil {
					return value
				}

				// Generate a new variable using an arrow function to avoid messing with "this"
				return ast.Expr{Loc: loc, Data: &ast.ECall{
					Target: ast.Expr{Loc: loc, Data: &ast.EArrow{
						Args:       []ast.Arg{{Binding: ast.Binding{Loc: loc, Data: &ast.BIdentifier{Ref: tempRef}}}},
						PreferExpr: true,
						Body:       ast.FnBody{Loc: loc, Stmts: []ast.Stmt{{Loc: loc, Data: &ast.SReturn{Value: &expr}}}},
					}},
					Args: []ast.Expr{},
				}}
			}
	}

	return func() ast.Expr {
		if tempRef == ast.InvalidRef {
			tempRef = p.generateTempRef(tempRefNeedsDeclare, "")
			p.recordUsage(tempRef)
			return ast.Assign(ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tempRef}}, value)
		}
		p.recordUsage(tempRef)
		return ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tempRef}}
	}, wrapFunc
}

func (p *parser) visitTSDecorators(tsDecorators []ast.Expr) []ast.Expr {
	for i, decorator := range tsDecorators {
		tsDecorators[i] = p.visitExpr(decorator)
	}
	return tsDecorators
}

func (p *parser) visitClass(class *ast.Class) {
	class.TSDecorators = p.visitTSDecorators(class.TSDecorators)

	if class.Name != nil {
		p.recordDeclaredSymbol(class.Name.Ref)
	}

	if class.Extends != nil {
		*class.Extends = p.visitExpr(*class.Extends)
	}

	oldIsThisCaptured := p.isThisCaptured
	p.isThisCaptured = true

	// A scope is needed for private identifiers
	p.pushScopeForVisitPass(ast.ScopeClassBody, class.BodyLoc)
	defer p.popScope()

	for i, property := range class.Properties {
		property.TSDecorators = p.visitTSDecorators(property.TSDecorators)

		// Special-case EPrivateIdentifier to allow it here
		if private, ok := property.Key.Data.(*ast.EPrivateIdentifier); ok {
			p.recordDeclaredSymbol(private.Ref)
		} else {
			class.Properties[i].Key = p.visitExpr(property.Key)
		}
		if property.Value != nil {
			*property.Value = p.visitExpr(*property.Value)
		}
		if property.Initializer != nil {
			*property.Initializer = p.visitExpr(*property.Initializer)
		}
	}

	p.isThisCaptured = oldIsThisCaptured
}

func (p *parser) visitArgs(args []ast.Arg) {
	for _, arg := range args {
		arg.TSDecorators = p.visitTSDecorators(arg.TSDecorators)
		p.visitBinding(arg.Binding)
		if arg.Default != nil {
			*arg.Default = p.visitExpr(*arg.Default)
		}
	}
}

func (p *parser) isDotDefineMatch(expr ast.Expr, parts []string) bool {
	if len(parts) > 1 {
		// Intermediates must be dot expressions
		e, ok := expr.Data.(*ast.EDot)
		last := len(parts) - 1
		return ok && parts[last] == e.Name && e.OptionalChain == ast.OptionalChainNone && p.isDotDefineMatch(e.Target, parts[:last])
	}

	// The last expression must be an identifier
	e, ok := expr.Data.(*ast.EIdentifier)
	if !ok {
		return false
	}

	// The name must match
	name := p.loadNameFromRef(e.Ref)
	if name != parts[0] {
		return false
	}

	result := p.findSymbol(name)

	// We must not be in a "with" statement scope
	if result.isInsideWithScope {
		return false
	}

	// The last symbol must be unbound
	return p.symbols[result.ref.InnerIndex].Kind == ast.SymbolUnbound
}

func (p *parser) jsxStringsToMemberExpression(loc ast.Loc, parts []string, assignTarget ast.AssignTarget) ast.Expr {
	// Generate an identifier for the first part
	ref := p.findSymbol(parts[0]).ref
	targetIfLast := ast.AssignTargetNone
	if len(parts) == 1 {
		targetIfLast = assignTarget
	}
	value := p.handleIdentifier(loc, targetIfLast, &ast.EIdentifier{
		Ref: ref,

		// Enable tree shaking
		CanBeRemovedIfUnused: true,
	})

	// Build up a chain of property access expressions for subsequent parts
	for i := 1; i < len(parts); i++ {
		targetIfLast = ast.AssignTargetNone
		if i+1 == len(parts) {
			targetIfLast = assignTarget
		}
		value = p.maybeRewriteDot(loc, targetIfLast, &ast.EDot{
			Target:  value,
			Name:    parts[i],
			NameLoc: loc,

			// Enable tree shaking
			CanBeRemovedIfUnused: true,
		})
	}

	return value
}

func (p *parser) checkForTypeofAndString(a ast.Expr, b ast.Expr) bool {
	if typeof, ok := a.Data.(*ast.EUnary); ok && typeof.Op == ast.UnOpTypeof {
		if str, ok := b.Data.(*ast.EString); ok {
			value := lexer.UTF16ToString(str.Value)
			switch value {
			case "undefined", "object", "boolean", "number", "bigint", "string", "symbol", "function", "unknown":
			default:
				r := p.source.RangeOfString(b.Loc)
				p.log.AddRangeWarning(&p.source, r, fmt.Sprintf("The \"typeof\" operator will never evaluate to %q", value))
			}
			return true
		}
	}
	return false
}

func (p *parser) warnAboutEqualityCheck(op string, value ast.Expr, afterOpLoc ast.Loc) bool {
	switch e := value.Data.(type) {
	case *ast.ENumber:
		// "0 === -0" is true in JavaScript
		if e.Value == 0 && math.Signbit(e.Value) {
			r := ast.Range{Loc: value.Loc, Len: 0}
			if int(r.Loc.Start) < len(p.source.Contents) && p.source.Contents[r.Loc.Start] == '-' {
				zeroRange := p.source.RangeOfNumber(ast.Loc{Start: r.Loc.Start + 1})
				r.Len = zeroRange.Len + 1
			}
			text := fmt.Sprintf("Comparison with -0 using the %s operator will also match 0", op)
			if op == "case" {
				text = "Comparison with -0 using a case clause will also match 0"
			}
			p.log.AddRangeWarning(&p.source, r, text)
			return true
		}

		// "NaN === NaN" is false in JavaScript
		if math.IsNaN(e.Value) {
			index := strings.LastIndex(p.source.Contents[:afterOpLoc.Start], op)
			r := ast.Range{Loc: ast.Loc{Start: int32(index)}, Len: int32(len(op))}
			text := fmt.Sprintf("Comparison with NaN using the %s operator here is always %v", op, op[0] == '!')
			if op == "case" {
				text = "This case clause will never be evaluated because equality with NaN is always false"
			}
			p.log.AddRangeWarning(&p.source, r, text)
			return true
		}

	case *ast.EArray, *ast.EArrow, *ast.EClass,
		*ast.EFunction, *ast.EObject, *ast.ERegExp:
		// This warning only applies to strict equality because loose equality can
		// cause string conversions. For example, "x == []" is true if x is the
		// empty string.
		if len(op) > 2 {
			index := strings.LastIndex(p.source.Contents[:afterOpLoc.Start], op)
			r := ast.Range{Loc: ast.Loc{Start: int32(index)}, Len: int32(len(op))}
			text := fmt.Sprintf("Comparison using the %s operator here is always %v", op, op[0] == '!')
			if op == "case" {
				text = "This case clause will never be evaluated because the comparison is always false"
			}
			p.log.AddRangeWarning(&p.source, r, text)
			return true
		}
	}

	return false
}

// EDot nodes represent a property access. This function may return an
// expression to replace the property access with. It assumes that the
// target of the EDot expression has already been visited.
func (p *parser) maybeRewriteDot(loc ast.Loc, assignTarget ast.AssignTarget, dot *ast.EDot) ast.Expr {
	if id, ok := dot.Target.Data.(*ast.EIdentifier); ok {
		// Rewrite property accesses on explicit namespace imports as an identifier.
		// This lets us replace them easily in the printer to rebind them to
		// something else without paying the cost of a whole-tree traversal during
		// module linking just to rewrite these EDot expressions.
		if importItems, ok := p.importItemsForNamespace[id.Ref]; ok {
			// Cache translation so each property access resolves to the same import
			item, ok := importItems[dot.Name]
			if !ok {
				// Generate a new import item symbol in the module scope
				item = ast.LocRef{Loc: dot.NameLoc, Ref: p.newSymbol(ast.SymbolImport, dot.Name)}
				p.moduleScope.Generated = append(p.moduleScope.Generated, item.Ref)

				// Link the namespace import and the import item together
				importItems[dot.Name] = item
				p.isImportItem[item.Ref] = true

				symbol := &p.symbols[item.Ref.InnerIndex]
				if !p.IsBundling {
					// Make sure the printer prints this as a property access
					symbol.NamespaceAlias = &ast.NamespaceAlias{
						NamespaceRef: id.Ref,
						Alias:        dot.Name,
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
			if p.IsBundling {
				p.ignoreUsage(id.Ref)
			}

			// Track how many times we've referenced this symbol
			p.recordUsage(item.Ref)
			return p.handleIdentifier(dot.NameLoc, assignTarget, &ast.EIdentifier{Ref: item.Ref})
		}

		// If this is a known enum value, inline the value of the enum
		if p.TS.Parse && dot.OptionalChain == ast.OptionalChainNone {
			if enumValueMap, ok := p.knownEnumValues[id.Ref]; ok {
				if number, ok := enumValueMap[dot.Name]; ok {
					return ast.Expr{Loc: loc, Data: &ast.ENumber{Value: number}}
				}
			}
		}
	}

	return ast.Expr{Loc: loc, Data: dot}
}

func joinStrings(a []uint16, b []uint16) []uint16 {
	data := make([]uint16, len(a)+len(b))
	copy(data[:len(a)], a)
	copy(data[len(a):], b)
	return data
}

func foldStringAddition(left ast.Expr, right ast.Expr) *ast.Expr {
	switch l := left.Data.(type) {
	case *ast.EString:
		switch r := right.Data.(type) {
		case *ast.EString:
			return &ast.Expr{Loc: left.Loc, Data: &ast.EString{
				Value:          joinStrings(l.Value, r.Value),
				PreferTemplate: l.PreferTemplate || r.PreferTemplate,
			}}

		case *ast.ETemplate:
			if r.Tag == nil {
				return &ast.Expr{Loc: left.Loc, Data: &ast.ETemplate{Head: joinStrings(l.Value, r.Head), Parts: r.Parts}}
			}
		}

	case *ast.ETemplate:
		if l.Tag == nil {
			switch r := right.Data.(type) {
			case *ast.EString:
				n := len(l.Parts)
				head := l.Head
				parts := make([]ast.TemplatePart, n)
				if n == 0 {
					head = joinStrings(head, r.Value)
				} else {
					copy(parts, l.Parts)
					parts[n-1].Tail = joinStrings(parts[n-1].Tail, r.Value)
				}
				return &ast.Expr{Loc: left.Loc, Data: &ast.ETemplate{Head: head, Parts: parts}}

			case *ast.ETemplate:
				if r.Tag == nil {
					n := len(l.Parts)
					head := l.Head
					parts := make([]ast.TemplatePart, n+len(r.Parts))
					copy(parts[n:], r.Parts)
					if n == 0 {
						head = joinStrings(head, r.Head)
					} else {
						copy(parts[:n], l.Parts)
						parts[n-1].Tail = joinStrings(parts[n-1].Tail, r.Head)
					}
					return &ast.Expr{Loc: left.Loc, Data: &ast.ETemplate{Head: head, Parts: parts}}
				}
			}
		}
	}

	return nil
}

func (p *parser) visitBooleanExpr(expr ast.Expr) ast.Expr {
	expr = p.visitExpr(expr)

	// Simplify syntax when we know it's used inside a boolean context
	if p.MangleSyntax {
		for {
			// "!!a" => "a"
			if not, ok := expr.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				if not2, ok2 := not.Value.Data.(*ast.EUnary); ok2 && not2.Op == ast.UnOpNot {
					expr = not2.Value
					continue
				}
			}

			break
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
	storeThisArgForParentOptionalChain func(ast.Expr) ast.Expr

	// Certain substitutions of identifiers are disallowed for assignment targets.
	// For example, we shouldn't transform "undefined = 1" into "void 0 = 1". This
	// isn't something real-world code would do but it matters for conformance
	// tests.
	assignTarget ast.AssignTarget
}

type exprOut struct {
	// True if the child node is an optional chain node (EDot, EIndex, or ECall
	// with an IsOptionalChain value of true)
	childContainsOptionalChain bool
}

func (p *parser) visitExpr(expr ast.Expr) ast.Expr {
	expr, _ = p.visitExprInOut(expr, exprIn{})
	return expr
}

func (p *parser) valueForThis(loc ast.Loc) (ast.Expr, bool) {
	if p.IsBundling && !p.isThisCaptured {
		if p.hasES6ImportSyntax || p.hasES6ExportSyntax {
			// In an ES6 module, "this" is supposed to be undefined. Instead of
			// doing this at runtime using "fn.call(undefined)", we do it at
			// compile time using expression substitution here.
			return ast.Expr{Loc: loc, Data: &ast.EUndefined{}}, true
		} else {
			// In a CommonJS module, "this" is supposed to be the same as "exports".
			// Instead of doing this at runtime using "fn.call(module.exports)", we
			// do it at compile time using expression substitution here.
			p.recordUsage(p.exportsRef)
			return ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: p.exportsRef}}, true
		}
	}

	return ast.Expr{}, false
}

func isBinaryNullAndUndefined(left ast.Expr, right ast.Expr, op ast.OpCode) (ast.Expr, ast.Expr, bool) {
	if a, ok := left.Data.(*ast.EBinary); ok && a.Op == op {
		if b, ok := right.Data.(*ast.EBinary); ok && b.Op == op {
			if idA, ok := a.Left.Data.(*ast.EIdentifier); ok {
				if idB, ok := b.Left.Data.(*ast.EIdentifier); ok && idA.Ref == idB.Ref {
					// "a === null || a === void 0"
					if _, ok := a.Right.Data.(*ast.ENull); ok {
						if _, ok := b.Right.Data.(*ast.EUndefined); ok {
							return a.Left, a.Right, true
						}
					}

					// "a === void 0 || a === null"
					if _, ok := a.Right.Data.(*ast.EUndefined); ok {
						if _, ok := b.Right.Data.(*ast.ENull); ok {
							return b.Left, b.Right, true
						}
					}
				}
			}
		}
	}

	return ast.Expr{}, ast.Expr{}, false
}

func inlineSpreadsOfArrayLiterals(values []ast.Expr) (results []ast.Expr) {
	for _, value := range values {
		if spread, ok := value.Data.(*ast.ESpread); ok {
			if array, ok := spread.Value.Data.(*ast.EArray); ok {
				for _, item := range array.Items {
					if _, ok := item.Data.(*ast.EMissing); ok {
						results = append(results, ast.Expr{Loc: item.Loc, Data: &ast.EUndefined{}})
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

func locAfterOp(e *ast.EBinary) ast.Loc {
	if e.Left.Loc.Start < e.Right.Loc.Start {
		return e.Right.Loc
	} else {
		// Handle the case when we have transposed the operands
		return e.Left.Loc
	}
}

func canBeDeleted(expr ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *ast.EIdentifier, *ast.EDot, *ast.EIndex:
		return true
	case *ast.ENumber:
		return math.IsInf(e.Value, 1) || math.IsNaN(e.Value)
	}
	return false
}

func (p *parser) isValidAssignmentTarget(expr ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *ast.EIdentifier, *ast.EObject, *ast.EArray:
		// Don't worry about recursive checking for objects and arrays. This will
		// already be handled naturally by passing down the assign target flag.
		return true
	case *ast.EDot:
		return e.OptionalChain == ast.OptionalChainNone
	case *ast.EIndex:
		return e.OptionalChain == ast.OptionalChainNone
	}
	return false
}

func (p *parser) visitExprInOut(expr ast.Expr, in exprIn) (ast.Expr, exprOut) {
	if in.assignTarget != ast.AssignTargetNone && !p.isValidAssignmentTarget(expr) {
		p.log.AddError(&p.source, expr.Loc, "Invalid assignment target")
	}

	switch e := expr.Data.(type) {
	case *ast.ENull, *ast.ESuper, *ast.EString,
		*ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.ERegExp, *ast.ENewTarget, *ast.EUndefined:

	case *ast.EThis:
		if value, ok := p.valueForThis(expr.Loc); ok {
			return value, exprOut{}
		}

	case *ast.EImportMeta:
		if p.importMetaRef != ast.InvalidRef {
			// Replace "import.meta" with a reference to the symbol
			p.recordUsage(p.importMetaRef)
			return ast.Expr{Loc: expr.Loc, Data: &ast.EIdentifier{Ref: p.importMetaRef}}, exprOut{}
		}

	case *ast.ESpread:
		e.Value = p.visitExpr(e.Value)

	case *ast.EIdentifier:
		name := p.loadNameFromRef(e.Ref)
		result := p.findSymbol(name)
		e.Ref = result.ref

		// Substitute user-specified defines for unbound symbols
		if p.symbols[e.Ref.InnerIndex].Kind == ast.SymbolUnbound && !result.isInsideWithScope && e != p.deleteTarget {
			if data, ok := p.Defines.IdentifierDefines[name]; ok {
				if data.DefineFunc != nil {
					new := p.valueForDefine(expr.Loc, in.assignTarget, data.DefineFunc)

					// Don't substitute an identifier for a non-identifier if this is an
					// assignment target, since it'll cause a syntax error
					if _, ok := new.Data.(*ast.EIdentifier); in.assignTarget == ast.AssignTargetNone || ok {
						return new, exprOut{}
					}
				}

				// Copy the call side effect flag over in case this expression is called
				if data.CallCanBeUnwrappedIfUnused {
					e.CallCanBeUnwrappedIfUnused = true
				}

				// All identifier defines that don't have user-specified replacements
				// are known to be side-effect free. Mark them as such if we get here.
				e.CanBeRemovedIfUnused = true
			}
		}

		return p.handleIdentifier(expr.Loc, in.assignTarget, e), exprOut{}

	case *ast.EPrivateIdentifier:
		// We should never get here
		panic("Internal error")

	case *ast.EJSXElement:
		// A missing tag is a fragment
		tag := e.Tag
		if tag == nil {
			value := p.jsxStringsToMemberExpression(expr.Loc, p.JSX.Fragment, in.assignTarget)
			tag = &value
		} else {
			*tag = p.visitExpr(*tag)
		}

		// Visit properties
		for i, property := range e.Properties {
			if property.Kind != ast.PropertySpread {
				property.Key = p.visitExpr(property.Key)
			}
			if property.Value != nil {
				*property.Value = p.visitExpr(*property.Value)
			}
			if property.Initializer != nil {
				*property.Initializer = p.visitExpr(*property.Initializer)
			}
			e.Properties[i] = property
		}

		// Arguments to createElement()
		args := []ast.Expr{*tag}
		if len(e.Properties) > 0 {
			args = append(args, p.lowerObjectSpread(expr.Loc, &ast.EObject{
				Properties: e.Properties,
			}))
		} else {
			args = append(args, ast.Expr{Loc: expr.Loc, Data: &ast.ENull{}})
		}
		if len(e.Children) > 0 {
			for _, child := range e.Children {
				args = append(args, p.visitExpr(child))
			}
		}

		// Call createElement()
		return ast.Expr{Loc: expr.Loc, Data: &ast.ECall{
			Target: p.jsxStringsToMemberExpression(expr.Loc, p.JSX.Factory, in.assignTarget),
			Args:   args,

			// Enable tree shaking
			CanBeUnwrappedIfUnused: true,
		}}, exprOut{}

	case *ast.ETemplate:
		if e.Tag != nil {
			*e.Tag = p.visitExpr(*e.Tag)
		}
		for i, part := range e.Parts {
			e.Parts[i].Value = p.visitExpr(part.Value)
		}

		// "`a${'b'}c`" => "`abc`"
		if p.MangleSyntax && e.Tag == nil {
			end := 0
			for _, part := range e.Parts {
				if str, ok := part.Value.Data.(*ast.EString); ok {
					if end == 0 {
						e.Head = append(append(e.Head, str.Value...), part.Tail...)
					} else {
						prevPart := &e.Parts[end-1]
						prevPart.Tail = append(append(prevPart.Tail, str.Value...), part.Tail...)
					}
				} else {
					e.Parts[end] = part
					end++
				}
			}
			e.Parts = e.Parts[:end]
		}

	case *ast.EBinary:
		isCallTarget := e == p.callTarget
		e.Left, _ = p.visitExprInOut(e.Left, exprIn{assignTarget: e.Op.BinaryAssignTarget()})

		// Pattern-match "typeof require == 'function' && ___" from browserify
		if e.Op == ast.BinOpLogicalAnd && e.Left.Data == p.typeofRequireEqualsFn {
			p.typeofRequireEqualsFnTarget = e.Right.Data
		}

		e.Right = p.visitExpr(e.Right)

		// Always put constants on the right for equality comparisons to help
		// reduce the number of cases we have to check during pattern matching. We
		// can only reorder expressions that do not have any side effects.
		switch e.Op {
		case ast.BinOpLooseEq, ast.BinOpLooseNe, ast.BinOpStrictEq, ast.BinOpStrictNe:
			if isPrimitiveToReorder(e.Left.Data) && !isPrimitiveToReorder(e.Right.Data) {
				e.Left, e.Right = e.Right, e.Left
			}
		}

		// Post-process the binary expression
		switch e.Op {
		case ast.BinOpComma:
			// "(1, 2)" => "2"
			// "(sideEffects(), 2)" => "(sideEffects(), 2)"
			if p.MangleSyntax {
				e.Left = p.simplifyUnusedExpr(e.Left)
				if e.Left.Data == nil {
					// "(1, fn)()" => "fn()"
					// "(1, this.fn)" => "this.fn"
					// "(1, this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Right) {
						return ast.JoinWithComma(ast.Expr{Loc: e.Left.Loc, Data: &ast.ENumber{}}, e.Right), exprOut{}
					}
					return e.Right, exprOut{}
				}
			}

		case ast.BinOpLooseEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				data := &ast.EBoolean{Value: result}

				// Pattern-match "typeof require == 'function'" from browserify. Also
				// match "'function' == typeof require" because some minifiers such as
				// terser transpose the left and right operands to "==" to form a
				// different but equivalent expression.
				if result && (e.Left.Data == p.typeofRequire || e.Right.Data == p.typeofRequire) {
					p.typeofRequireEqualsFn = data
				}

				return ast.Expr{Loc: expr.Loc, Data: data}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("==", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("==", e.Right, afterOpLoc)
			}
			p.checkForTypeofAndString(e.Left, e.Right)

			if p.MangleSyntax {
				// "x == void 0" => "x == null"
				if _, ok := e.Right.Data.(*ast.EUndefined); ok {
					e.Right.Data = &ast.ENull{}
				}
			}

		case ast.BinOpStrictEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.EBoolean{Value: result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("===", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("===", e.Right, afterOpLoc)
			}
			if p.checkForTypeofAndString(e.Left, e.Right) {
				// "typeof x === 'undefined'" => "typeof x == 'undefined'"
				if p.MangleSyntax {
					e.Op = ast.BinOpLooseEq
				}
			}

		case ast.BinOpLooseNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.EBoolean{Value: !result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("!=", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("!=", e.Right, afterOpLoc)
			}
			p.checkForTypeofAndString(e.Left, e.Right)

			if p.MangleSyntax {
				// "x != void 0" => "x != null"
				if _, ok := e.Right.Data.(*ast.EUndefined); ok {
					e.Right.Data = &ast.ENull{}
				}
			}

		case ast.BinOpStrictNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.EBoolean{Value: !result}}, exprOut{}
			}
			afterOpLoc := locAfterOp(e)
			if !p.warnAboutEqualityCheck("!==", e.Left, afterOpLoc) {
				p.warnAboutEqualityCheck("!==", e.Right, afterOpLoc)
			}
			if p.checkForTypeofAndString(e.Left, e.Right) {
				// "typeof x !== 'undefined'" => "typeof x != 'undefined'"
				if p.MangleSyntax {
					e.Op = ast.BinOpLooseNe
				}
			}

		case ast.BinOpNullishCoalescing:
			switch e.Left.Data.(type) {
			case *ast.EBoolean, *ast.ENumber, *ast.EString, *ast.ERegExp,
				*ast.EObject, *ast.EArray, *ast.EFunction, *ast.EArrow, *ast.EClass:
				return e.Left, exprOut{}

			case *ast.ENull, *ast.EUndefined:
				// "(null ?? fn)()" => "fn()"
				// "(null ?? this.fn)" => "this.fn"
				// "(null ?? this.fn)()" => "(0, this.fn)()"
				if isCallTarget && hasValueForThisInCall(e.Right) {
					return ast.JoinWithComma(ast.Expr{Loc: e.Left.Loc, Data: &ast.ENumber{}}, e.Right), exprOut{}
				}
				return e.Right, exprOut{}

			default:
				if p.UnsupportedFeatures.Has(compat.NullishCoalescing) {
					return p.lowerNullishCoalescing(expr.Loc, e.Left, e.Right), exprOut{}
				}
			}

		case ast.BinOpLogicalOr:
			if boolean, ok := toBooleanWithoutSideEffects(e.Left.Data); ok {
				if boolean {
					return e.Left, exprOut{}
				} else {
					// "(0 || fn)()" => "fn()"
					// "(0 || this.fn)" => "this.fn"
					// "(0 || this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Right) {
						return ast.JoinWithComma(ast.Expr{Loc: e.Left.Loc, Data: &ast.ENumber{}}, e.Right), exprOut{}
					}
					return e.Right, exprOut{}
				}
			}

			if p.MangleSyntax {
				// "a || (b || c)" => "a || b || c"
				if right, ok := e.Right.Data.(*ast.EBinary); ok && right.Op == ast.BinOpLogicalOr {
					e.Left.Data = &ast.EBinary{Op: ast.BinOpLogicalOr, Left: e.Left, Right: right.Left}
					e.Right = right.Right
				}

				// "a === null || a === undefined" => "a == null"
				if left, right, ok := isBinaryNullAndUndefined(e.Left, e.Right, ast.BinOpStrictEq); ok {
					e.Op = ast.BinOpLooseEq
					e.Left = left
					e.Right = right
				}
			}

		case ast.BinOpLogicalAnd:
			if boolean, ok := toBooleanWithoutSideEffects(e.Left.Data); ok {
				if boolean {
					// "(1 && fn)()" => "fn()"
					// "(1 && this.fn)" => "this.fn"
					// "(1 && this.fn)()" => "(0, this.fn)()"
					if isCallTarget && hasValueForThisInCall(e.Right) {
						return ast.JoinWithComma(ast.Expr{Loc: e.Left.Loc, Data: &ast.ENumber{}}, e.Right), exprOut{}
					}
					return e.Right, exprOut{}
				} else {
					return e.Left, exprOut{}
				}
			}

			if p.MangleSyntax {
				// "a && (b && c)" => "a && b && c"
				if right, ok := e.Right.Data.(*ast.EBinary); ok && right.Op == ast.BinOpLogicalAnd {
					e.Left.Data = &ast.EBinary{Op: ast.BinOpLogicalAnd, Left: e.Left, Right: right.Left}
					e.Right = right.Right
				}

				// "a !== null && a !== undefined" => "a != null"
				if left, right, ok := isBinaryNullAndUndefined(e.Left, e.Right, ast.BinOpStrictNe); ok {
					e.Op = ast.BinOpLooseNe
					e.Left = left
					e.Right = right
				}
			}

		case ast.BinOpAdd:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: left + right}}, exprOut{}
				}
			}

			// "'abc' + 'xyz'" => "'abcxyz'"
			if result := foldStringAddition(e.Left, e.Right); result != nil {
				return *result, exprOut{}
			}

			if left, ok := e.Left.Data.(*ast.EBinary); ok && left.Op == ast.BinOpAdd {
				// "x + 'abc' + 'xyz'" => "x + 'abcxyz'"
				if result := foldStringAddition(left.Right, e.Right); result != nil {
					return ast.Expr{Loc: expr.Loc, Data: &ast.EBinary{Op: left.Op, Left: left.Left, Right: *result}}, exprOut{}
				}
			}

		case ast.BinOpSub:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: left - right}}, exprOut{}
				}
			}

		case ast.BinOpMul:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: left * right}}, exprOut{}
				}
			}

		case ast.BinOpDiv:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: left / right}}, exprOut{}
				}
			}

		case ast.BinOpRem:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: math.Mod(left, right)}}, exprOut{}
				}
			}

		case ast.BinOpPow:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: math.Pow(left, right)}}, exprOut{}
				}
			}

			// Lower the exponentiation operator for browsers that don't support it
			if p.UnsupportedFeatures.Has(compat.ExponentOperator) {
				return p.callRuntime(expr.Loc, "__pow", []ast.Expr{e.Left, e.Right}), exprOut{}
			}

		case ast.BinOpShl:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: float64(toInt32(left) << (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case ast.BinOpShr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: float64(toInt32(left) >> (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case ast.BinOpUShr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: float64(toUint32(left) >> (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case ast.BinOpBitwiseAnd:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: float64(toInt32(left) & toInt32(right))}}, exprOut{}
				}
			}

		case ast.BinOpBitwiseOr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: float64(toInt32(left) | toInt32(right))}}, exprOut{}
				}
			}

		case ast.BinOpBitwiseXor:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: float64(toInt32(left) ^ toInt32(right))}}, exprOut{}
				}
			}

			////////////////////////////////////////////////////////////////////////////////
			// All assignment operators below here

		case ast.BinOpAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSet(target, loc, private, e.Right), exprOut{}
			}

			// Lower object rest patterns for browsers that don't support them. Note
			// that assignment expressions are used to represent initializers in
			// binding patterns, so only do this if we're not ourselves the target of
			// an assignment. Example: "[a = b] = c"
			if in.assignTarget == ast.AssignTargetNone {
				if result, ok := p.lowerObjectRestInAssign(e.Left, e.Right); ok {
					return result, exprOut{}
				}
			}

		case ast.BinOpAddAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpAdd, e.Right), exprOut{}
			}

		case ast.BinOpSubAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpSub, e.Right), exprOut{}
			}

		case ast.BinOpMulAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpMul, e.Right), exprOut{}
			}

		case ast.BinOpDivAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpDiv, e.Right), exprOut{}
			}

		case ast.BinOpRemAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpRem, e.Right), exprOut{}
			}

		case ast.BinOpPowAssign:
			// Lower the exponentiation operator for browsers that don't support it
			if p.UnsupportedFeatures.Has(compat.ExponentOperator) {
				return p.lowerExponentiationAssignmentOperator(expr.Loc, e), exprOut{}
			}

			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpPow, e.Right), exprOut{}
			}

		case ast.BinOpShlAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpShl, e.Right), exprOut{}
			}

		case ast.BinOpShrAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpShr, e.Right), exprOut{}
			}

		case ast.BinOpUShrAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpUShr, e.Right), exprOut{}
			}

		case ast.BinOpBitwiseOrAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpBitwiseOr, e.Right), exprOut{}
			}

		case ast.BinOpBitwiseAndAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpBitwiseAnd, e.Right), exprOut{}
			}

		case ast.BinOpBitwiseXorAssign:
			if target, loc, private := p.extractPrivateIndex(e.Left); private != nil {
				return p.lowerPrivateSetBinOp(target, loc, private, ast.BinOpBitwiseXor, e.Right), exprOut{}
			}

		case ast.BinOpNullishCoalescingAssign:
			if p.UnsupportedFeatures.Has(compat.LogicalAssignment) {
				return p.lowerNullishCoalescingAssignmentOperator(expr.Loc, e), exprOut{}
			}

		case ast.BinOpLogicalAndAssign:
			if p.UnsupportedFeatures.Has(compat.LogicalAssignment) {
				return p.lowerLogicalAssignmentOperator(expr.Loc, e, ast.BinOpLogicalAnd), exprOut{}
			}

		case ast.BinOpLogicalOrAssign:
			if p.UnsupportedFeatures.Has(compat.LogicalAssignment) {
				return p.lowerLogicalAssignmentOperator(expr.Loc, e, ast.BinOpLogicalOr), exprOut{}
			}
		}

	case *ast.EIndex:
		// "a['b']" => "a.b"
		if p.MangleSyntax {
			if str, ok := e.Index.Data.(*ast.EString); ok && lexer.IsIdentifierUTF16(str.Value) {
				return p.visitExprInOut(ast.Expr{Loc: expr.Loc, Data: &ast.EDot{
					Target:        e.Target,
					Name:          lexer.UTF16ToString(str.Value),
					NameLoc:       e.Index.Loc,
					OptionalChain: e.OptionalChain,
				}}, in)
			}
		}

		isCallTarget := e == p.callTarget
		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == ast.OptionalChainContinue,
		})
		e.Target = target

		// Special-case EPrivateIdentifier to allow it here
		if private, ok := e.Index.Data.(*ast.EPrivateIdentifier); ok {
			name := p.loadNameFromRef(private.Ref)
			result := p.findSymbol(name)
			private.Ref = result.ref

			// Unlike regular identifiers, there are no unbound private identifiers
			kind := p.symbols[result.ref.InnerIndex].Kind
			if !kind.IsPrivate() {
				r := ast.Range{Loc: e.Index.Loc, Len: int32(len(name))}
				p.log.AddRangeError(&p.source, r, fmt.Sprintf("Private name %q must be declared in an enclosing class", name))
			} else if in.assignTarget != ast.AssignTargetNone && (kind == ast.SymbolPrivateGet || kind == ast.SymbolPrivateStaticGet) {
				r := ast.Range{Loc: e.Index.Loc, Len: int32(len(name))}
				p.log.AddRangeWarning(&p.source, r, fmt.Sprintf("Writing to getter-only property %q will throw", name))
			} else if in.assignTarget != ast.AssignTargetReplace && (kind == ast.SymbolPrivateSet || kind == ast.SymbolPrivateStaticSet) {
				r := ast.Range{Loc: e.Index.Loc, Len: int32(len(name))}
				p.log.AddRangeWarning(&p.source, r, fmt.Sprintf("Reading from setter-only property %q will throw", name))
			}

			// Lower private member access only if we're sure the target isn't needed
			// for the value of "this" for a call expression. All other cases will be
			// taken care of by the enclosing call expression.
			if p.UnsupportedFeatures.Has(kind.Feature()) && e.OptionalChain == ast.OptionalChainNone &&
				in.assignTarget == ast.AssignTargetNone && !isCallTarget {
				// "foo.#bar" => "__privateGet(foo, #bar)"
				return p.lowerPrivateGet(e.Target, e.Index.Loc, private), exprOut{}
			}
		} else {
			e.Index = p.visitExpr(e.Index)
		}

		// Create an error for assigning to an import namespace
		if in.assignTarget != ast.AssignTargetNone {
			if id, ok := e.Target.Data.(*ast.EIdentifier); ok && p.symbols[id.Ref.InnerIndex].Kind == ast.SymbolImport {
				if str, ok := e.Index.Data.(*ast.EString); ok && lexer.IsIdentifierUTF16(str.Value) {
					r := p.source.RangeOfString(e.Index.Loc)
					p.log.AddRangeError(&p.source, r, fmt.Sprintf("Cannot assign to import %q", lexer.UTF16ToString(str.Value)))
				} else {
					r := lexer.RangeOfIdentifier(p.source, e.Target.Loc)
					p.log.AddRangeError(&p.source, r, fmt.Sprintf("Cannot assign to property on import %q", p.symbols[id.Ref.InnerIndex].OriginalName))
				}
			}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != ast.OptionalChainNone
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out, nil)
		}

		// If this is a known enum value, inline the value of the enum
		if p.TS.Parse && e.OptionalChain == ast.OptionalChainNone {
			if str, ok := e.Index.Data.(*ast.EString); ok {
				if id, ok := e.Target.Data.(*ast.EIdentifier); ok {
					if enumValueMap, ok := p.knownEnumValues[id.Ref]; ok {
						if number, ok := enumValueMap[lexer.UTF16ToString(str.Value)]; ok {
							return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: number}}, exprOut{}
						}
					}
				}
			}
		}

		return expr, exprOut{
			childContainsOptionalChain: containsOptionalChain,
		}

	case *ast.EUnary:
		switch e.Op {
		case ast.UnOpTypeof:
			p.typeofTarget = e.Value.Data

			_, idBefore := e.Value.Data.(*ast.EIdentifier)
			e.Value, _ = p.visitExprInOut(e.Value, exprIn{assignTarget: e.Op.UnaryAssignTarget()})
			id, idAfter := e.Value.Data.(*ast.EIdentifier)

			// The expression "typeof (0, x)" must not become "typeof x" if "x"
			// is unbound because that could suppress a ReferenceError from "x"
			if !idBefore && idAfter && p.symbols[id.Ref.InnerIndex].Kind == ast.SymbolUnbound {
				e.Value = ast.JoinWithComma(ast.Expr{Loc: e.Value.Loc, Data: &ast.ENumber{}}, e.Value)
			}

			// "typeof require" => "'function'"
			if id, ok := e.Value.Data.(*ast.EIdentifier); ok && id.Ref == p.requireRef {
				p.ignoreUsage(p.requireRef)
				p.typeofRequire = &ast.EString{Value: lexer.StringToUTF16("function")}
				return ast.Expr{Loc: expr.Loc, Data: p.typeofRequire}, exprOut{}
			}

			if typeof, ok := typeofWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.EString{Value: lexer.StringToUTF16(typeof)}}, exprOut{}
			}

			return expr, exprOut{}

		case ast.UnOpDelete:
			// Warn about code that tries to do "delete super.foo"
			var superPropLoc ast.Loc
			switch e2 := e.Value.Data.(type) {
			case *ast.EDot:
				if _, ok := e2.Target.Data.(*ast.ESuper); ok {
					superPropLoc = e2.Target.Loc
				}
			case *ast.EIndex:
				if _, ok := e2.Target.Data.(*ast.ESuper); ok {
					superPropLoc = e2.Target.Loc
				}
			}
			if superPropLoc.Start != 0 {
				r := lexer.RangeOfIdentifier(p.source, superPropLoc)
				p.log.AddRangeWarning(&p.source, r, "Attempting to delete a property of \"super\" will throw a ReferenceError")
			}

			p.deleteTarget = e.Value.Data
			canBeDeletedBefore := canBeDeleted(e.Value)
			value, out := p.visitExprInOut(e.Value, exprIn{hasChainParent: true})
			e.Value = value
			canBeDeletedAfter := canBeDeleted(e.Value)

			// Lower optional chaining if present since we're guaranteed to be the
			// end of the chain
			if out.childContainsOptionalChain {
				return p.lowerOptionalChain(expr, in, out, nil)
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
				e.Value = ast.JoinWithComma(ast.Expr{Loc: e.Value.Loc, Data: &ast.ENumber{}}, e.Value)
			}

			return expr, exprOut{}
		}

		e.Value, _ = p.visitExprInOut(e.Value, exprIn{assignTarget: e.Op.UnaryAssignTarget()})

		// Post-process the unary expression
		switch e.Op {
		case ast.UnOpNot:
			if boolean, ok := toBooleanWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.EBoolean{Value: !boolean}}, exprOut{}
			}

			// "!!!a" => "!a"
			if not, ok := e.Value.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot && ast.IsBooleanValue(not.Value) {
				return not.Value, exprOut{}
			}

		case ast.UnOpVoid:
			if p.exprCanBeRemovedIfUnused(e.Value) {
				return ast.Expr{Loc: expr.Loc, Data: &ast.EUndefined{}}, exprOut{}
			}

		case ast.UnOpPos:
			if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: number}}, exprOut{}
			}

		case ast.UnOpNeg:
			if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{Loc: expr.Loc, Data: &ast.ENumber{Value: -number}}, exprOut{}
			}

			////////////////////////////////////////////////////////////////////////////////
			// All assignment operators below here

		case ast.UnOpPreDec:
			if target, loc, private := p.extractPrivateIndex(e.Value); private != nil {
				return p.lowerPrivateSetUnOp(target, loc, private, ast.BinOpSub, false), exprOut{}
			}

		case ast.UnOpPreInc:
			if target, loc, private := p.extractPrivateIndex(e.Value); private != nil {
				return p.lowerPrivateSetUnOp(target, loc, private, ast.BinOpAdd, false), exprOut{}
			}

		case ast.UnOpPostDec:
			if target, loc, private := p.extractPrivateIndex(e.Value); private != nil {
				return p.lowerPrivateSetUnOp(target, loc, private, ast.BinOpSub, true), exprOut{}
			}

		case ast.UnOpPostInc:
			if target, loc, private := p.extractPrivateIndex(e.Value); private != nil {
				return p.lowerPrivateSetUnOp(target, loc, private, ast.BinOpAdd, true), exprOut{}
			}
		}

	case *ast.EDot:
		// Check both user-specified defines and known globals
		if defines, ok := p.Defines.DotDefines[e.Name]; ok {
			for _, define := range defines {
				if p.isDotDefineMatch(expr, define.Parts) {
					// Substitute user-specified defines
					if define.Data.DefineFunc != nil {
						return p.valueForDefine(expr.Loc, in.assignTarget, define.Data.DefineFunc), exprOut{}
					}

					// Copy the call side effect flag over in case this expression is called
					if define.Data.CallCanBeUnwrappedIfUnused {
						e.CallCanBeUnwrappedIfUnused = true
					}

					// All dot defines that don't have user-specified replacements are
					// known to be side-effect free. Mark them as such if we get here.
					e.CanBeRemovedIfUnused = true
					break
				}
			}
		}

		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent: e.OptionalChain == ast.OptionalChainContinue,
		})
		e.Target = target

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != ast.OptionalChainNone
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out, nil)
		}

		return p.maybeRewriteDot(expr.Loc, in.assignTarget, e), exprOut{
			childContainsOptionalChain: containsOptionalChain,
		}

	case *ast.EIf:
		isCallTarget := e == p.callTarget
		e.Test = p.visitBooleanExpr(e.Test)
		e.Yes = p.visitExpr(e.Yes)
		e.No = p.visitExpr(e.No)

		// Fold constants
		if boolean, ok := toBooleanWithoutSideEffects(e.Test.Data); ok {
			if boolean {
				// "(1 ? fn : 2)()" => "fn()"
				// "(1 ? this.fn : 2)" => "this.fn"
				// "(1 ? this.fn : 2)()" => "(0, this.fn)()"
				if isCallTarget && hasValueForThisInCall(e.Yes) {
					return ast.JoinWithComma(ast.Expr{Loc: e.Test.Loc, Data: &ast.ENumber{}}, e.Yes), exprOut{}
				}
				return e.Yes, exprOut{}
			} else {
				// "(0 ? 1 : fn)()" => "fn()"
				// "(0 ? 1 : this.fn)" => "this.fn"
				// "(0 ? 1 : this.fn)()" => "(0, this.fn)()"
				if isCallTarget && hasValueForThisInCall(e.No) {
					return ast.JoinWithComma(ast.Expr{Loc: e.Test.Loc, Data: &ast.ENumber{}}, e.No), exprOut{}
				}
				return e.No, exprOut{}
			}
		}

		if p.MangleSyntax {
			return p.mangleIfExpr(expr.Loc, e), exprOut{}
		}

	case *ast.EAwait:
		e.Value = p.visitExpr(e.Value)

		// "await" expressions turn into "yield" expressions when lowering
		if p.UnsupportedFeatures.Has(compat.AsyncAwait) {
			return ast.Expr{Loc: expr.Loc, Data: &ast.EYield{Value: &e.Value}}, exprOut{}
		}

	case *ast.EYield:
		if e.Value != nil {
			*e.Value = p.visitExpr(*e.Value)
		}

	case *ast.EArray:
		if in.assignTarget != ast.AssignTargetNone {
			p.markSyntaxFeature(compat.Destructuring, ast.Range{Loc: expr.Loc, Len: 1})
		}
		hasSpread := false
		for i, item := range e.Items {
			switch e2 := item.Data.(type) {
			case *ast.EMissing:
			case *ast.ESpread:
				e2.Value, _ = p.visitExprInOut(e2.Value, exprIn{assignTarget: in.assignTarget})
				hasSpread = true
			case *ast.EBinary:
				if in.assignTarget != ast.AssignTargetNone && e2.Op == ast.BinOpAssign {
					e2.Left, _ = p.visitExprInOut(e2.Left, exprIn{assignTarget: ast.AssignTargetReplace})
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
		if p.MangleSyntax && hasSpread && in.assignTarget == ast.AssignTargetNone {
			e.Items = inlineSpreadsOfArrayLiterals(e.Items)
		}

	case *ast.EObject:
		if in.assignTarget != ast.AssignTargetNone {
			p.markSyntaxFeature(compat.Destructuring, ast.Range{Loc: expr.Loc, Len: 1})
		}
		hasSpread := false
		hasProto := false
		for i := range e.Properties {
			property := &e.Properties[i]

			if property.Kind != ast.PropertySpread {
				key := p.visitExpr(property.Key)
				e.Properties[i].Key = key

				// Forbid duplicate "__proto__" properties according to the specification
				if !property.IsComputed && !property.WasShorthand && !property.IsMethod && in.assignTarget == ast.AssignTargetNone {
					if str, ok := key.Data.(*ast.EString); ok && lexer.UTF16EqualsString(str.Value, "__proto__") {
						if hasProto {
							r := lexer.RangeOfIdentifier(p.source, key.Loc)
							p.log.AddRangeError(&p.source, r, "Cannot specify the \"__proto__\" property more than once per object")
						}
						hasProto = true
					}
				}
			} else {
				hasSpread = true
			}

			// Extract the initializer for expressions like "({ a: b = c } = d)"
			if in.assignTarget != ast.AssignTargetNone && property.Initializer == nil && property.Value != nil {
				if binary, ok := property.Value.Data.(*ast.EBinary); ok && binary.Op == ast.BinOpAssign {
					property.Initializer = &binary.Right
					property.Value = &binary.Left
				}
			}

			if property.Value != nil {
				*property.Value, _ = p.visitExprInOut(*property.Value, exprIn{assignTarget: in.assignTarget})
			}
			if property.Initializer != nil {
				*property.Initializer = p.visitExpr(*property.Initializer)
			}
		}

		if in.assignTarget == ast.AssignTargetNone {
			// "{a, ...{b, c}, d}" => "{a, b, c, d}"
			if p.MangleSyntax && hasSpread {
				var properties []ast.Property
				for _, property := range e.Properties {
					if property.Kind == ast.PropertySpread {
						switch v := property.Value.Data.(type) {
						case *ast.EBoolean, *ast.ENull, *ast.EUndefined, *ast.ENumber,
							*ast.EBigInt, *ast.ERegExp, *ast.EFunction, *ast.EArrow:
							// This value is ignored because it doesn't have any of its own properties
							continue

						case *ast.EObject:
							for i, p := range v.Properties {
								// Getters are evaluated at iteration time. The property
								// descriptor is not inlined into the caller. Since we are not
								// evaluating code at compile time, just bail if we hit one
								// and preserve the spread with the remaining properties.
								if p.Kind == ast.PropertyGet || p.Kind == ast.PropertySet {
									v.Properties = v.Properties[i:]
									properties = append(properties, property)
									break
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

	case *ast.EImport:
		e.Expr = p.visitExpr(e.Expr)

		// The argument must be a string
		if str, ok := e.Expr.Data.(*ast.EString); ok {
			// Ignore calls to import() if the control flow is provably dead here.
			// We don't want to spend time scanning the required files if they will
			// never be used.
			if p.isControlFlowDead {
				return ast.Expr{Loc: expr.Loc, Data: &ast.ENull{}}, exprOut{}
			}

			importRecordIndex := p.addImportRecord(ast.ImportDynamic, e.Expr.Loc, lexer.UTF16ToString(str.Value))
			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)

			e.ImportRecordIndex = &importRecordIndex
		} else if p.IsBundling {
			r := lexer.RangeOfIdentifier(p.source, expr.Loc)
			p.log.AddRangeWarning(&p.source, r,
				"This dynamic import will not be bundled because the argument is not a string literal")
		}

	case *ast.ECall:
		var storeThisArg func(ast.Expr) ast.Expr
		var thisArgFunc func() ast.Expr
		var thisArgWrapFunc func(ast.Expr) ast.Expr
		p.callTarget = e.Target.Data
		if e.OptionalChain == ast.OptionalChainStart {
			// Signal to our child if this is an ECall at the start of an optional
			// chain. If so, the child will need to stash the "this" context for us
			// that we need for the ".call(this, ...args)".
			storeThisArg = func(thisArg ast.Expr) ast.Expr {
				thisArgFunc, thisArgWrapFunc = p.captureValueWithPossibleSideEffects(thisArg.Loc, 2, thisArg)
				return thisArgFunc()
			}
		}
		_, wasIdentifierBeforeVisit := e.Target.Data.(*ast.EIdentifier)
		target, out := p.visitExprInOut(e.Target, exprIn{
			hasChainParent:                     e.OptionalChain == ast.OptionalChainContinue,
			storeThisArgForParentOptionalChain: storeThisArg,
		})
		e.Target = target
		hasSpread := false
		for i, arg := range e.Args {
			arg = p.visitExpr(arg)
			if _, ok := arg.Data.(*ast.ESpread); ok {
				hasSpread = true
			}
			e.Args[i] = arg
		}

		// "foo(1, ...[2, 3], 4)" => "foo(1, 2, 3, 4)"
		if p.MangleSyntax && hasSpread && in.assignTarget == ast.AssignTargetNone {
			e.Args = inlineSpreadsOfArrayLiterals(e.Args)
		}

		// Detect if this is a direct eval. Note that "(1 ? eval : 0)(x)" will
		// become "eval(x)" after we visit the target due to dead code elimination,
		// but that doesn't mean it should become a direct eval.
		if wasIdentifierBeforeVisit {
			if id, ok := e.Target.Data.(*ast.EIdentifier); ok {
				if symbol := p.symbols[id.Ref.InnerIndex]; symbol.OriginalName == "eval" {
					e.IsDirectEval = true

					// Mark this scope and all parent scopes as containing a direct eval.
					// This will prevent us from renaming any symbols.
					for s := p.currentScope; s != nil; s = s.Parent {
						s.ContainsDirectEval = true
					}
				}
			}
		}

		// Copy the call side effect flag over if this is a known target
		switch t := target.Data.(type) {
		case *ast.EIdentifier:
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}
		case *ast.EDot:
			if t.CallCanBeUnwrappedIfUnused {
				e.CanBeUnwrappedIfUnused = true
			}
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != ast.OptionalChainNone
		if containsOptionalChain && !in.hasChainParent {
			result, out := p.lowerOptionalChain(expr, in, out, thisArgFunc)
			if thisArgWrapFunc != nil {
				result = thisArgWrapFunc(result)
			}
			return result, out
		}

		// If this is a plain call expression (instead of an optional chain), lower
		// private member access in the call target now if there is one
		if !containsOptionalChain {
			if target, loc, private := p.extractPrivateIndex(e.Target); private != nil {
				// "foo.#bar(123)" => "__privateGet(foo, #bar).call(foo, 123)"
				targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(target.Loc, 2, target)
				return targetWrapFunc(ast.Expr{Loc: target.Loc, Data: &ast.ECall{
					Target: ast.Expr{Loc: target.Loc, Data: &ast.EDot{
						Target:  p.lowerPrivateGet(targetFunc(), loc, private),
						Name:    "call",
						NameLoc: target.Loc,
					}},
					Args:                   append([]ast.Expr{targetFunc()}, e.Args...),
					CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
				}}), exprOut{}
			}
		}

		// Track calls to require() so we can use them while bundling
		if id, ok := e.Target.Data.(*ast.EIdentifier); ok && id.Ref == p.requireRef && p.IsBundling {
			// There must be one argument
			if len(e.Args) != 1 {
				r := lexer.RangeOfIdentifier(p.source, e.Target.Loc)
				p.log.AddRangeWarning(&p.source, r, fmt.Sprintf(
					"This call to \"require\" will not be bundled because it has %d arguments", len(e.Args)))
			} else {
				arg := e.Args[0]

				// The argument must be a string
				if str, ok := arg.Data.(*ast.EString); ok {
					// Ignore calls to require() if the control flow is provably dead here.
					// We don't want to spend time scanning the required files if they will
					// never be used.
					if p.isControlFlowDead {
						return ast.Expr{Loc: expr.Loc, Data: &ast.ENull{}}, exprOut{}
					}

					importRecordIndex := p.addImportRecord(ast.ImportRequire, arg.Loc, lexer.UTF16ToString(str.Value))
					p.importRecords[importRecordIndex].IsInsideTryBody = p.fnOptsVisit.tryBodyCount != 0
					p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, importRecordIndex)

					// Create a new expression to represent the operation
					p.ignoreUsage(p.requireRef)
					return ast.Expr{Loc: expr.Loc, Data: &ast.ERequire{ImportRecordIndex: importRecordIndex}}, exprOut{}
				}

				r := lexer.RangeOfIdentifier(p.source, e.Target.Loc)
				p.log.AddRangeWarning(&p.source, r,
					"This call to \"require\" will not be bundled because the argument is not a string literal")
			}
		}

		return expr, exprOut{
			childContainsOptionalChain: containsOptionalChain,
		}

	case *ast.ENew:
		e.Target = p.visitExpr(e.Target)
		for i, arg := range e.Args {
			e.Args[i] = p.visitExpr(arg)
		}

	case *ast.EArrow:
		oldFnOpts := p.fnOptsVisit
		p.fnOptsVisit = fnOptsVisit{}

		p.pushScopeForVisitPass(ast.ScopeFunctionArgs, expr.Loc)
		p.visitArgs(e.Args)
		p.pushScopeForVisitPass(ast.ScopeFunctionBody, e.Body.Loc)
		e.Body.Stmts = p.visitStmtsAndPrependTempRefs(e.Body.Stmts)
		p.popScope()
		p.lowerFunction(&e.IsAsync, &e.Args, e.Body.Loc, &e.Body.Stmts, &e.PreferExpr)
		p.popScope()

		if p.MangleSyntax && len(e.Body.Stmts) == 1 {
			if s, ok := e.Body.Stmts[0].Data.(*ast.SReturn); ok {
				if s.Value == nil {
					// "() => { return }" => "() => {}"
					e.Body.Stmts = []ast.Stmt{}
				} else {
					// "() => { return x }" => "() => x"
					e.PreferExpr = true
				}
			}
		}

		p.fnOptsVisit = oldFnOpts

	case *ast.EFunction:
		p.visitFn(&e.Fn, expr.Loc)

		// Remove unused function names when minifying
		if p.MangleSyntax && e.Fn.Name != nil && p.symbols[e.Fn.Name.Ref.InnerIndex].UseCountEstimate == 0 {
			e.Fn.Name = nil
		}

	case *ast.EClass:
		if e.Class.Name != nil {
			p.pushScopeForVisitPass(ast.ScopeClassName, expr.Loc)
		}
		p.visitClass(&e.Class)
		if e.Class.Name != nil {
			p.popScope()
		}

		// Remove unused class names when minifying
		if p.MangleSyntax && e.Class.Name != nil && p.symbols[e.Class.Name.Ref.InnerIndex].UseCountEstimate == 0 {
			e.Class.Name = nil
		}

		// Lower class field syntax for browsers that don't support it
		_, expr = p.lowerClass(ast.Stmt{}, expr)

	default:
		panic("Internal error")
	}

	return expr, exprOut{}
}

func (p *parser) valueForDefine(loc ast.Loc, assignTarget ast.AssignTarget, defineFunc config.DefineFunc) ast.Expr {
	expr := ast.Expr{Loc: loc, Data: defineFunc(p.findSymbolHelper)}
	if id, ok := expr.Data.(*ast.EIdentifier); ok {
		return p.handleIdentifier(loc, assignTarget, id)
	}
	return expr
}

func (p *parser) handleIdentifier(loc ast.Loc, assignTarget ast.AssignTarget, e *ast.EIdentifier) ast.Expr {
	ref := e.Ref

	if assignTarget != ast.AssignTargetNone {
		if p.symbols[ref.InnerIndex].Kind == ast.SymbolImport {
			// Create an error for assigning to an import namespace
			r := lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddRangeError(&p.source, r, fmt.Sprintf("Cannot assign to import %q", p.symbols[ref.InnerIndex].OriginalName))
		} else {
			// Remember that this part assigns to this symbol for code splitting
			use := p.symbolUses[ref]
			use.IsAssigned = true
			p.symbolUses[ref] = use
		}
	}

	// Substitute an EImportIdentifier now if this is an import item
	if p.isImportItem[ref] {
		return ast.Expr{Loc: loc, Data: &ast.EImportIdentifier{Ref: ref}}
	}

	// Substitute a namespace export reference now if appropriate
	if p.TS.Parse {
		if nsRef, ok := p.isExportedInsideNamespace[ref]; ok {
			name := p.symbols[ref.InnerIndex].OriginalName

			// If this is a known enum value, inline the value of the enum
			if enumValueMap, ok := p.knownEnumValues[nsRef]; ok {
				if number, ok := enumValueMap[name]; ok {
					return ast.Expr{Loc: loc, Data: &ast.ENumber{Value: number}}
				}
			}

			// Otherwise, create a property access on the namespace
			p.recordUsage(nsRef)
			return ast.Expr{Loc: loc, Data: &ast.EDot{
				Target:  ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: nsRef}},
				Name:    name,
				NameLoc: loc,
			}}
		}
	}

	// Warn about uses of "require" other than a direct call
	if ref == p.requireRef && e != p.callTarget && e != p.typeofTarget && p.fnOptsVisit.tryBodyCount == 0 {
		// "typeof require == 'function' && require"
		if e == p.typeofRequireEqualsFnTarget {
			// Become "false" in the browser and "require" in node
			if p.Platform == config.PlatformBrowser {
				return ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: false}}
			}
		} else {
			r := lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddRangeWarning(&p.source, r, "Indirect calls to \"require\" will not be bundled")
		}
	}

	return ast.Expr{Loc: loc, Data: e}
}

func extractNumericValues(left ast.Expr, right ast.Expr) (float64, float64, bool) {
	if a, ok := left.Data.(*ast.ENumber); ok {
		if b, ok := right.Data.(*ast.ENumber); ok {
			return a.Value, b.Value, true
		}
	}
	return 0, 0, false
}

func (p *parser) visitFn(fn *ast.Fn, scopeLoc ast.Loc) {
	oldFnOpts := p.fnOptsVisit
	oldIsThisCaptured := p.isThisCaptured
	oldArgumentsRef := p.argumentsRef
	p.fnOptsVisit = fnOptsVisit{}
	p.isThisCaptured = true
	p.argumentsRef = &fn.ArgumentsRef

	if fn.Name != nil {
		p.recordDeclaredSymbol(fn.Name.Ref)
	}

	p.pushScopeForVisitPass(ast.ScopeFunctionArgs, scopeLoc)
	p.visitArgs(fn.Args)
	p.pushScopeForVisitPass(ast.ScopeFunctionBody, fn.Body.Loc)
	fn.Body.Stmts = p.visitStmtsAndPrependTempRefs(fn.Body.Stmts)
	p.popScope()
	p.lowerFunction(&fn.IsAsync, &fn.Args, fn.Body.Loc, &fn.Body.Stmts, nil)
	p.popScope()

	p.fnOptsVisit = oldFnOpts
	p.isThisCaptured = oldIsThisCaptured
	p.argumentsRef = oldArgumentsRef
}

func (p *parser) scanForImportsAndExports(stmts []ast.Stmt, isBundling bool) []ast.Stmt {
	stmtsEnd := 0

	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *ast.SImport:
			// TypeScript always trims unused imports. This is important for
			// correctness since some imports might be fake (only in the type
			// system and used for type-only imports).
			if p.MangleSyntax || p.TS.Parse {
				foundImports := false
				isUnusedInTypeScript := true

				// Remove the default name if it's unused
				if s.DefaultName != nil {
					foundImports = true
					symbol := p.symbols[s.DefaultName.Ref.InnerIndex]

					// TypeScript has a separate definition of unused
					if p.TS.Parse && p.tsUseCounts[s.DefaultName.Ref.InnerIndex] != 0 {
						isUnusedInTypeScript = false
					}

					// Remove the symbol if it's never used outside a dead code region
					if symbol.UseCountEstimate == 0 {
						s.DefaultName = nil
					}
				}

				// Remove the star import if it's unused
				if s.StarNameLoc != nil {
					foundImports = true
					symbol := p.symbols[s.NamespaceRef.InnerIndex]

					// TypeScript has a separate definition of unused
					if p.TS.Parse && p.tsUseCounts[s.NamespaceRef.InnerIndex] != 0 {
						isUnusedInTypeScript = false
					}

					// Remove the symbol if it's never used outside a dead code region
					if symbol.UseCountEstimate == 0 {
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
						if p.TS.Parse && p.tsUseCounts[item.Name.Ref.InnerIndex] != 0 {
							isUnusedInTypeScript = false
						}

						// Remove the symbol if it's never used outside a dead code region
						if symbol.UseCountEstimate != 0 {
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
				if p.TS.Parse && foundImports && isUnusedInTypeScript {
					continue
				}
			}

			if isBundling {
				if s.StarNameLoc != nil {
					// If we're bundling a star import, add any import items we generated
					// for this namespace while parsing as explicit import items instead.
					// That will cause the bundler to bundle them more efficiently when
					// both this module and the imported module are in the same group.
					if importItems, ok := p.importItemsForNamespace[s.NamespaceRef]; ok && len(importItems) > 0 {
						items := s.Items
						if items == nil {
							items = &[]ast.ClauseItem{}
						}

						// Sort keys for determinism
						sorted := make([]string, 0, len(importItems))
						for alias := range importItems {
							sorted = append(sorted, alias)
						}
						sort.Strings(sorted)

						for _, alias := range sorted {
							name := importItems[alias]
							originalName := p.symbols[name.Ref.InnerIndex].OriginalName
							*items = append(*items, ast.ClauseItem{
								Alias:        alias,
								AliasLoc:     name.Loc,
								Name:         name,
								OriginalName: originalName,
							})
							p.declaredSymbols = append(p.declaredSymbols, ast.DeclaredSymbol{
								Ref:        name.Ref,
								IsTopLevel: true,
							})
						}
						s.Items = items
					}

					// Remove the star import if it's not actually used. The parser only
					// counts the star import as used if it was used for something other
					// than a property access.
					//
					// That way if it's only used for property accesses, we can omit the
					// code for the star import entirely and just merge the property
					// accesses directly with the appropriate symbols instead (since both
					// this module and the imported module are in the same group).
					if p.symbols[s.NamespaceRef.InnerIndex].UseCountEstimate == 0 {
						s.StarNameLoc = nil
					}
				}

				if s.DefaultName != nil {
					p.namedImports[s.DefaultName.Ref] = ast.NamedImport{
						Alias:             "default",
						AliasLoc:          s.DefaultName.Loc,
						NamespaceRef:      s.NamespaceRef,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				}

				if s.StarNameLoc != nil {
					p.namedImports[s.NamespaceRef] = ast.NamedImport{
						Alias:             "*",
						AliasLoc:          *s.StarNameLoc,
						NamespaceRef:      ast.InvalidRef,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				}

				if s.Items != nil {
					for _, item := range *s.Items {
						p.namedImports[item.Name.Ref] = ast.NamedImport{
							Alias:             item.Alias,
							AliasLoc:          item.AliasLoc,
							NamespaceRef:      s.NamespaceRef,
							ImportRecordIndex: s.ImportRecordIndex,
						}
					}
				}
			}

			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			// This is true for import statements without imports like "import 'foo'"
			if s.DefaultName == nil && s.StarNameLoc == nil && s.Items == nil {
				p.importRecords[s.ImportRecordIndex].DoesNotUseExports = true
			}

		case *ast.SExportStar:
			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			// Only track import paths if we want dependencies
			if isBundling {
				if s.Alias != nil {
					// "export * as ns from 'path'"
					p.namedImports[s.NamespaceRef] = ast.NamedImport{
						Alias:             "*",
						AliasLoc:          s.Alias.Loc,
						NamespaceRef:      ast.InvalidRef,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				} else {
					// "export * from 'path'"
					p.exportStarImportRecords = append(p.exportStarImportRecords, s.ImportRecordIndex)
				}
			}

		case *ast.SExportFrom:
			p.importRecordsForCurrentPart = append(p.importRecordsForCurrentPart, s.ImportRecordIndex)

			// Only track import paths if we want dependencies
			if isBundling {
				for _, item := range s.Items {
					// Note that the imported alias is not item.Alias, which is the
					// exported alias. This is somewhat confusing because each
					// SExportFrom statement is basically SImport + SExportClause in one.
					p.namedImports[item.Name.Ref] = ast.NamedImport{
						Alias:             p.symbols[item.Name.Ref.InnerIndex].OriginalName,
						AliasLoc:          item.Name.Loc,
						NamespaceRef:      s.NamespaceRef,
						ImportRecordIndex: s.ImportRecordIndex,
						IsExported:        true,
					}
				}
			}

		case *ast.SExportClause:
			// Strip exports of non-local symbols in TypeScript, since those likely
			// correspond to type-only exports
			if p.TS.Parse {
				itemsEnd := 0
				for _, item := range s.Items {
					if p.symbols[item.Name.Ref.InnerIndex].Kind != ast.SymbolUnbound {
						s.Items[itemsEnd] = item
						itemsEnd++

						// Mark re-exported imports as such
						if namedImport, ok := p.namedImports[item.Name.Ref]; ok {
							namedImport.IsExported = true
							p.namedImports[item.Name.Ref] = namedImport
						}
					}
				}
				if itemsEnd == 0 {
					// Remove empty export statements entirely
					continue
				}
				s.Items = s.Items[:itemsEnd]
			}
		}

		// Filter out statements we skipped over
		stmts[stmtsEnd] = stmt
		stmtsEnd++
	}

	return stmts[:stmtsEnd]
}

func (p *parser) appendPart(parts []ast.Part, stmts []ast.Stmt) []ast.Part {
	p.symbolUses = make(map[ast.Ref]ast.SymbolUse)
	p.declaredSymbols = nil
	p.importRecordsForCurrentPart = nil
	p.scopesForCurrentPart = nil
	part := ast.Part{
		Stmts:      p.visitStmtsAndPrependTempRefs(stmts),
		SymbolUses: p.symbolUses,
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

func (p *parser) stmtsCanBeRemovedIfUnused(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *ast.SFunction, *ast.SEmpty:
			// These never have side effects

		case *ast.SImport:
			// Let these be removed if they are unused. Note that we also need to
			// check if the imported file is marked as "sideEffects: false" before we
			// can remove a SImport statement. Otherwise the import must be kept for
			// its side effects.

		case *ast.SClass:
			if !p.classCanBeRemovedIfUnused(s.Class) {
				return false
			}

		case *ast.SExpr:
			if !p.exprCanBeRemovedIfUnused(s.Value) {
				return false
			}

		case *ast.SLocal:
			for _, decl := range s.Decls {
				if !p.bindingCanBeRemovedIfUnused(decl.Binding) {
					return false
				}
				if decl.Value != nil && !p.exprCanBeRemovedIfUnused(*decl.Value) {
					return false
				}
			}

		case *ast.SExportClause:
			// Exports are tracked separately, so this isn't necessary

		case *ast.SExportDefault:
			switch {
			case s.Value.Expr != nil:
				if !p.exprCanBeRemovedIfUnused(*s.Value.Expr) {
					return false
				}

			case s.Value.Stmt != nil:
				switch s2 := s.Value.Stmt.Data.(type) {
				case *ast.SFunction:
					// These never have side effects

				case *ast.SClass:
					if !p.classCanBeRemovedIfUnused(s2.Class) {
						return false
					}

				default:
					panic("Internal error")
				}
			}

		default:
			// Assume that all statements not explicitly special-cased here have side
			// effects, and cannot be removed even if unused
			return false
		}
	}

	return true
}

func (p *parser) classCanBeRemovedIfUnused(class ast.Class) bool {
	if class.Extends != nil && !p.exprCanBeRemovedIfUnused(*class.Extends) {
		return false
	}

	for _, property := range class.Properties {
		if !p.exprCanBeRemovedIfUnused(property.Key) {
			return false
		}
		if property.Value != nil && !p.exprCanBeRemovedIfUnused(*property.Value) {
			return false
		}
		if property.Initializer != nil && !p.exprCanBeRemovedIfUnused(*property.Initializer) {
			return false
		}
	}

	return true
}

func (p *parser) bindingCanBeRemovedIfUnused(binding ast.Binding) bool {
	switch b := binding.Data.(type) {
	case *ast.BArray:
		for _, item := range b.Items {
			if !p.bindingCanBeRemovedIfUnused(item.Binding) {
				return false
			}
			if item.DefaultValue != nil && !p.exprCanBeRemovedIfUnused(*item.DefaultValue) {
				return false
			}
		}

	case *ast.BObject:
		for _, property := range b.Properties {
			if !property.IsSpread && !p.exprCanBeRemovedIfUnused(property.Key) {
				return false
			}
			if !p.bindingCanBeRemovedIfUnused(property.Value) {
				return false
			}
			if property.DefaultValue != nil && !p.exprCanBeRemovedIfUnused(*property.DefaultValue) {
				return false
			}
		}
	}

	return true
}

func (p *parser) exprCanBeRemovedIfUnused(expr ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *ast.ENull, *ast.EUndefined, *ast.EMissing, *ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.EString, *ast.EThis, *ast.ERegExp, *ast.EFunction, *ast.EArrow, *ast.EImportMeta:
		return true

	case *ast.EDot:
		return e.CanBeRemovedIfUnused

	case *ast.EClass:
		return p.classCanBeRemovedIfUnused(e.Class)

	case *ast.EIdentifier:
		if e.CanBeRemovedIfUnused || p.symbols[e.Ref.InnerIndex].Kind != ast.SymbolUnbound {
			return true
		}

	case *ast.EIf:
		return p.exprCanBeRemovedIfUnused(e.Test) && p.exprCanBeRemovedIfUnused(e.Yes) && p.exprCanBeRemovedIfUnused(e.No)

	case *ast.EArray:
		for _, item := range e.Items {
			if !p.exprCanBeRemovedIfUnused(item) {
				return false
			}
		}
		return true

	case *ast.EObject:
		for _, property := range e.Properties {
			// The key must still be evaluated if it's computed or a spread
			if property.Kind == ast.PropertySpread || property.IsComputed {
				return false
			}
			if property.Value != nil && !p.exprCanBeRemovedIfUnused(*property.Value) {
				return false
			}
		}
		return true

	case *ast.ECall:
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

	case *ast.ENew:
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
	}

	// Assume all other expression types have side effects and cannot be removed
	return false
}

// This will return a nil expression if the expression can be totally removed
func (p *parser) simplifyUnusedExpr(expr ast.Expr) ast.Expr {
	switch e := expr.Data.(type) {
	case *ast.ENull, *ast.EUndefined, *ast.EMissing, *ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.EString, *ast.EThis, *ast.ERegExp, *ast.EFunction, *ast.EArrow, *ast.EImportMeta:
		return ast.Expr{}

	case *ast.EDot:
		if e.CanBeRemovedIfUnused {
			return ast.Expr{}
		}

	case *ast.EIdentifier:
		if e.CanBeRemovedIfUnused || p.symbols[e.Ref.InnerIndex].Kind != ast.SymbolUnbound {
			return ast.Expr{}
		}

	case *ast.ETemplate:
		if e.Tag == nil {
			var result ast.Expr
			for _, part := range e.Parts {
				// Make sure "ToString" is still evaluated on the value
				if result.Data == nil {
					result = ast.Expr{Loc: part.Value.Loc, Data: &ast.EString{}}
				}
				result = ast.Expr{Loc: part.Value.Loc, Data: &ast.EBinary{
					Op:    ast.BinOpAdd,
					Left:  result,
					Right: part.Value,
				}}
			}
			return result
		}

	case *ast.EArray:
		// Arrays with "..." spread expressions can't be unwrapped because the
		// "..." triggers code evaluation via iterators. In that case, just trim
		// the other items instead and leave the array expression there.
		for _, spread := range e.Items {
			if _, ok := spread.Data.(*ast.ESpread); ok {
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
		var result ast.Expr
		for _, item := range e.Items {
			result = maybeJoinWithComma(result, p.simplifyUnusedExpr(item))
		}
		return result

	case *ast.EObject:
		// Objects with "..." spread expressions can't be unwrapped because the
		// "..." triggers code evaluation via getters. In that case, just trim
		// the other items instead and leave the object expression there.
		for _, spread := range e.Properties {
			if spread.Kind == ast.PropertySpread {
				end := 0
				for _, property := range e.Properties {
					// Spread properties must always be evaluated
					if property.Kind != ast.PropertySpread {
						value := p.simplifyUnusedExpr(*property.Value)
						if value.Data != nil {
							// Keep the value
							*property.Value = value
						} else if !property.IsComputed {
							// Skip this property if the key doesn't need to be computed
							continue
						} else {
							// Replace values without side effects with "0" because it's short
							property.Value.Data = &ast.ENumber{}
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
		var result ast.Expr
		for _, property := range e.Properties {
			if property.IsComputed {
				// Make sure "ToString" is still evaluated on the key
				result = maybeJoinWithComma(result, ast.Expr{Loc: property.Key.Loc, Data: &ast.EBinary{
					Op:    ast.BinOpAdd,
					Left:  property.Key,
					Right: ast.Expr{Loc: property.Key.Loc, Data: &ast.EString{}},
				}})
			}
			result = maybeJoinWithComma(result, p.simplifyUnusedExpr(*property.Value))
		}
		return result

	case *ast.EIf:
		e.Yes = p.simplifyUnusedExpr(e.Yes)
		e.No = p.simplifyUnusedExpr(e.No)

		// "foo() ? 1 : 2" => "foo()"
		if e.Yes.Data == nil && e.No.Data == nil {
			return p.simplifyUnusedExpr(e.Test)
		}

		// "foo() ? 1 : bar()" => "foo() || bar()"
		if e.Yes.Data == nil {
			return ast.Expr{Loc: expr.Loc, Data: &ast.EBinary{
				Op:    ast.BinOpLogicalOr,
				Left:  e.Test,
				Right: e.No,
			}}
		}

		// "foo() ? bar() : 2" => "foo() && bar()"
		if e.No.Data == nil {
			return ast.Expr{Loc: expr.Loc, Data: &ast.EBinary{
				Op:    ast.BinOpLogicalAnd,
				Left:  e.Test,
				Right: e.Yes,
			}}
		}

	case *ast.EBinary:
		switch e.Op {
		case ast.BinOpComma:
			e.Left = p.simplifyUnusedExpr(e.Left)
			e.Right = p.simplifyUnusedExpr(e.Right)
			if e.Left.Data == nil {
				return e.Right
			}
			if e.Right.Data == nil {
				return e.Left
			}

		case ast.BinOpLogicalAnd, ast.BinOpLogicalOr, ast.BinOpNullishCoalescing:
			e.Right = p.simplifyUnusedExpr(e.Right)
			if e.Right.Data == nil {
				return p.simplifyUnusedExpr(e.Left)
			}

		case ast.BinOpAdd:
			if result, isStringAddition := simplifyUnusedStringAdditionChain(expr); isStringAddition {
				return result
			}
		}

	case *ast.ECall:
		// A call that has been marked "__PURE__" can be removed if all arguments
		// can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			expr = ast.Expr{}
			for _, arg := range e.Args {
				expr = maybeJoinWithComma(expr, p.simplifyUnusedExpr(arg))
			}
		}

	case *ast.ENew:
		// A constructor call that has been marked "__PURE__" can be removed if all
		// arguments can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			expr = ast.Expr{}
			for _, arg := range e.Args {
				expr = maybeJoinWithComma(expr, p.simplifyUnusedExpr(arg))
			}
		}
	}

	return expr
}

func simplifyUnusedStringAdditionChain(expr ast.Expr) (ast.Expr, bool) {
	switch e := expr.Data.(type) {
	case *ast.EString:
		// "'x' + y" => "'' + y"
		return ast.Expr{Loc: expr.Loc, Data: &ast.EString{}}, true

	case *ast.EBinary:
		if e.Op == ast.BinOpAdd {
			left, leftIsStringAddition := simplifyUnusedStringAdditionChain(e.Left)
			e.Left = left

			if _, rightIsString := e.Right.Data.(*ast.EString); rightIsString {
				// "('' + x) + 'y'" => "'' + x"
				if leftIsStringAddition {
					return left, true
				}

				// "x + 'y'" => "x + ''"
				if !leftIsStringAddition {
					e.Right.Data = &ast.EString{}
					return expr, true
				}
			}

			return expr, leftIsStringAddition
		}
	}

	return expr, false
}

func newParser(log logging.Log, source logging.Source, lexer lexer.Lexer, options *config.Options) *parser {
	if options.Defines == nil {
		defaultDefines := config.ProcessDefines(nil)
		options.Defines = &defaultDefines
	}

	p := &parser{
		log:            log,
		source:         source,
		lexer:          lexer,
		allowIn:        true,
		Options:        *options,
		fnOptsParse:    fnOptsParse{isOutsideFn: true},
		runtimeImports: make(map[string]ast.Ref),

		// For lowering private methods
		weakMapRef:     ast.InvalidRef,
		weakSetRef:     ast.InvalidRef,
		privateGetters: make(map[ast.Ref]ast.Ref),
		privateSetters: make(map[ast.Ref]ast.Ref),

		// These are for TypeScript
		emittedNamespaceVars:      make(map[ast.Ref]bool),
		isExportedInsideNamespace: make(map[ast.Ref]ast.Ref),
		knownEnumValues:           make(map[ast.Ref]map[string]float64),
		localTypeNames:            make(map[string]bool),

		// These are for handling ES6 imports and exports
		importItemsForNamespace: make(map[ast.Ref]map[string]ast.LocRef),
		isImportItem:            make(map[ast.Ref]bool),
		namedImports:            make(map[ast.Ref]ast.NamedImport),
		namedExports:            make(map[string]ast.Ref),
	}

	p.findSymbolHelper = func(name string) ast.Ref { return p.findSymbol(name).ref }
	p.pushScopeForParsePass(ast.ScopeEntry, ast.Loc{Start: locModuleScope})

	return p
}

func Parse(log logging.Log, source logging.Source, options config.Options) (result ast.AST, ok bool) {
	ok = true
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			ok = false
		} else if r != nil {
			panic(r)
		}
	}()

	// Default options for JSX elements
	if len(options.JSX.Factory) == 0 {
		options.JSX.Factory = []string{"React", "createElement"}
	}
	if len(options.JSX.Fragment) == 0 {
		options.JSX.Fragment = []string{"React", "Fragment"}
	}

	p := newParser(log, source, lexer.NewLexer(log, source), &options)

	// Consume a leading hashbang comment
	hashbang := ""
	if p.lexer.Token == lexer.THashbang {
		hashbang = p.lexer.Identifier
		p.lexer.Next()
	}

	// Allow top-level await
	p.fnOptsParse.allowAwait = true
	p.fnOptsParse.isTopLevel = true

	// Parse the file in the first pass, but do not bind symbols
	stmts := p.parseStmtsUpTo(lexer.TEndOfFile, parseStmtOpts{isModuleScope: true})
	p.prepareForVisitPass(&options)

	// Strip off a leading "use strict" directive when not bundling
	directive := ""
	if !options.IsBundling && len(stmts) > 0 {
		if s, ok := stmts[0].Data.(*ast.SDirective); ok {
			directive = lexer.UTF16ToString(s.Value)
			stmts = stmts[1:]
		}
	}

	// Insert a variable for "import.meta" at the top of the file if it was used.
	// We don't need to worry about "use strict" directives because this only
	// happens when bundling, in which case we are flatting the module scopes of
	// all modules together anyway so such directives are meaningless.
	if p.importMetaRef != ast.InvalidRef {
		importMetaStmt := ast.Stmt{Data: &ast.SLocal{
			Kind: ast.LocalConst,
			Decls: []ast.Decl{{
				Binding: ast.Binding{Data: &ast.BIdentifier{Ref: p.importMetaRef}},
				Value:   &ast.Expr{Data: &ast.EObject{}},
			}},
		}}
		stmts = append(append(make([]ast.Stmt, 0, len(stmts)+1), importMetaStmt), stmts...)
	}

	// Bind symbols in a second pass over the AST. I started off doing this in a
	// single pass, but it turns out it's pretty much impossible to do this
	// correctly while handling arrow functions because of the grammar
	// ambiguities.
	parts := []ast.Part{}
	if !p.IsBundling {
		// When not bundling, everything comes in a single part
		parts = p.appendPart(parts, stmts)
	} else {
		// When bundling, each top-level statement is potentially a separate part
		var after []ast.Part
		for _, stmt := range stmts {
			switch s := stmt.Data.(type) {
			case *ast.SLocal:
				// Split up top-level multi-declaration variable statements
				for _, decl := range s.Decls {
					clone := *s
					clone.Decls = []ast.Decl{decl}
					parts = p.appendPart(parts, []ast.Stmt{{Loc: stmt.Loc, Data: &clone}})
				}

			case *ast.SExportEquals:
				// TypeScript "export = value;" becomes "module.exports = value;". This
				// must happen at the end after everything is parsed because TypeScript
				// moves this statement to the end when it generates code.
				after = p.appendPart(after, []ast.Stmt{stmt})

			default:
				parts = p.appendPart(parts, []ast.Stmt{stmt})
			}
		}
		parts = append(parts, after...)
	}

	result = p.toAST(source, parts, hashbang, directive)
	result.SourceMapComment = p.lexer.SourceMappingURL
	return
}

func LazyExportAST(log logging.Log, source logging.Source, options config.Options, expr ast.Expr, apiCall string) ast.AST {
	// Don't create a new lexer using lexer.NewLexer() here since that will
	// actually attempt to parse the first token, which might cause a syntax
	// error.
	p := newParser(log, source, lexer.Lexer{}, &options)
	p.prepareForVisitPass(&options)

	// Optionally call a runtime API function to transform the expression
	if apiCall != "" {
		p.symbolUses = make(map[ast.Ref]ast.SymbolUse)
		expr = p.callRuntime(expr.Loc, apiCall, []ast.Expr{expr})
	}

	// Defer the actual code generation until linking
	part := ast.Part{
		Stmts:      []ast.Stmt{ast.Stmt{Loc: expr.Loc, Data: &ast.SLazyExport{Value: expr}}},
		SymbolUses: p.symbolUses,
	}
	p.symbolUses = nil

	ast := p.toAST(source, []ast.Part{part}, "", "")
	ast.HasLazyExport = true
	return ast
}

func (p *parser) validateJSX(span ast.Span, name string) []string {
	if span.Text == "" {
		return nil
	}
	parts := strings.Split(span.Text, ".")
	for _, part := range parts {
		if !lexer.IsIdentifier(part) {
			p.log.AddRangeWarning(&p.source, span.Range, fmt.Sprintf("Invalid JSX %s: %s", name, span.Text))
			return nil
		}
	}
	return parts
}

func (p *parser) prepareForVisitPass(options *config.Options) {
	p.pushScopeForVisitPass(ast.ScopeEntry, ast.Loc{Start: locModuleScope})
	p.moduleScope = p.currentScope
	p.hoistSymbols(p.moduleScope)

	if options.IsBundling {
		p.exportsRef = p.declareCommonJSSymbol(ast.SymbolHoisted, "exports")
		p.requireRef = p.declareCommonJSSymbol(ast.SymbolUnbound, "require")
		p.moduleRef = p.declareCommonJSSymbol(ast.SymbolHoisted, "module")
	} else {
		p.exportsRef = p.newSymbol(ast.SymbolHoisted, "exports")
		p.requireRef = p.newSymbol(ast.SymbolUnbound, "require")
		p.moduleRef = p.newSymbol(ast.SymbolHoisted, "module")
	}

	// Convert "import.meta" to a variable if it's not supported in the output format
	if p.hasImportMeta && (p.UnsupportedFeatures.Has(compat.ImportMeta) || (options.IsBundling && !p.OutputFormat.KeepES6ImportExportSyntax())) {
		p.importMetaRef = p.newSymbol(ast.SymbolOther, "import_meta")
		p.moduleScope.Generated = append(p.moduleScope.Generated, p.importMetaRef)
	} else {
		p.importMetaRef = ast.InvalidRef
	}

	// Handle "@jsx" and "@jsxFrag" pragmas now that lexing is done
	if p.JSX.Parse {
		if value := p.validateJSX(p.lexer.JSXFactoryPragmaComment, "factory"); value != nil {
			p.JSX.Factory = value
		}
		if value := p.validateJSX(p.lexer.JSXFragmentPragmaComment, "fragment"); value != nil {
			p.JSX.Fragment = value
		}
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
		kind == ast.SymbolHoisted && !p.hasES6ImportSyntax && !p.hasES6ExportSyntax {
		return member.Ref
	}

	// Create a new symbol if we didn't merge with an existing one above
	ref := p.newSymbol(kind, name)

	// If the variable wasn't declared, declare it now. This means any references
	// to this name will become bound to this symbol after this (since we haven't
	// run the visit pass yet).
	if !ok {
		p.moduleScope.Members[name] = ast.ScopeMember{Ref: ref, Loc: ast.Loc{Start: -1}}
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
	if !p.MinifyIdentifiers || p.source.Index == runtime.SourceIndex {
		return nil
	}

	// Add everything in the file to the histogram
	charFreq := &ast.CharFreq{}
	charFreq.Scan(p.source.Contents, 1)

	// Subtract out all symbols that will be minified
	var visit func(*ast.Scope)
	visit = func(scope *ast.Scope) {
		for _, member := range scope.Members {
			symbol := &p.symbols[member.Ref.InnerIndex]
			if symbol.SlotNamespace() != ast.SlotMustNotBeRenamed {
				charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate))
			}
		}
		if scope.LabelRef != ast.InvalidRef {
			symbol := &p.symbols[scope.LabelRef.InnerIndex]
			if symbol.SlotNamespace() != ast.SlotMustNotBeRenamed {
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

func (p *parser) toAST(source logging.Source, parts []ast.Part, hashbang string, directive string) ast.AST {
	// Insert an import statement for any runtime imports we generated
	if len(p.runtimeImports) > 0 {
		// Sort the imports for determinism
		keys := make([]string, 0, len(p.runtimeImports))
		for key := range p.runtimeImports {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		namespaceRef := p.newSymbol(ast.SymbolOther, "runtime")
		p.moduleScope.Generated = append(p.moduleScope.Generated, namespaceRef)
		declaredSymbols := make([]ast.DeclaredSymbol, len(keys))
		clauseItems := make([]ast.ClauseItem, len(keys))
		importRecordIndex := p.addImportRecord(ast.ImportStmt, ast.Loc{}, "<runtime>")
		sourceIndex := runtime.SourceIndex
		p.importRecords[importRecordIndex].SourceIndex = &sourceIndex

		// Create per-import information
		for i, key := range keys {
			ref := p.runtimeImports[key]
			declaredSymbols[i] = ast.DeclaredSymbol{Ref: ref, IsTopLevel: true}
			clauseItems[i] = ast.ClauseItem{Alias: key, Name: ast.LocRef{Ref: ref}}
			p.namedImports[ref] = ast.NamedImport{
				Alias:             key,
				NamespaceRef:      namespaceRef,
				ImportRecordIndex: importRecordIndex,
			}
		}

		// Append a single import to the end of the file (ES6 imports are hoisted
		// so we don't need to worry about where the import statement goes)
		parts = append(parts, ast.Part{
			DeclaredSymbols:     declaredSymbols,
			ImportRecordIndices: []uint32{importRecordIndex},
			Stmts: []ast.Stmt{{Data: &ast.SImport{
				NamespaceRef:      namespaceRef,
				Items:             &clauseItems,
				ImportRecordIndex: importRecordIndex,
			}}},
		})
	}

	// Handle import paths after the whole file has been visited because we need
	// symbol usage counts to be able to remove unused type-only imports in
	// TypeScript code.
	partsEnd := 0
	for _, part := range parts {
		p.importRecordsForCurrentPart = nil
		p.declaredSymbols = nil
		part.Stmts = p.scanForImportsAndExports(part.Stmts, p.IsBundling)
		part.ImportRecordIndices = append(part.ImportRecordIndices, p.importRecordsForCurrentPart...)
		part.DeclaredSymbols = append(part.DeclaredSymbols, p.declaredSymbols...)
		if len(part.Stmts) > 0 {
			parts[partsEnd] = part
			partsEnd++
		}
	}
	parts = parts[:partsEnd]

	// Analyze cross-part dependencies for tree shaking and code splitting
	{
		// Map locals to parts
		p.topLevelSymbolToParts = make(map[ast.Ref][]uint32)
		for partIndex, part := range parts {
			for _, declared := range part.DeclaredSymbols {
				if declared.IsTopLevel {
					p.topLevelSymbolToParts[declared.Ref] = append(
						p.topLevelSymbolToParts[declared.Ref], uint32(partIndex))
				}
			}
		}

		// Each part tracks the other parts it depends on within this file
		for partIndex, part := range parts {
			localDependencies := make(map[uint32]bool)
			for ref := range part.SymbolUses {
				for _, otherPart := range p.topLevelSymbolToParts[ref] {
					localDependencies[otherPart] = true
				}

				// Also map from imports to parts that use them
				if namedImport, ok := p.namedImports[ref]; ok {
					namedImport.LocalPartsWithUses = append(namedImport.LocalPartsWithUses, uint32(partIndex))
					p.namedImports[ref] = namedImport
				}
			}
			parts[partIndex].LocalDependencies = localDependencies
		}
	}

	// Make a wrapper symbol in case we need to be wrapped in a closure
	wrapperRef := p.newSymbol(ast.SymbolOther, "require_"+p.source.IdentifierName)

	// Assign slots to symbols in nested scopes. This is some precomputation for
	// the symbol renaming pass that will happen later in the linker. It's done
	// now in the parser because we want it to be done in parallel per file and
	// we're already executing code in a dedicated goroutine for this file.
	var nestedScopeSlotCounts ast.SlotCounts
	if p.MinifyIdentifiers {
		nestedScopeSlotCounts = renamer.AssignNestedScopeSlots(p.moduleScope, p.symbols)
	}

	return ast.AST{
		Parts:                   parts,
		ModuleScope:             p.moduleScope,
		CharFreq:                p.computeCharacterFrequency(),
		Symbols:                 p.symbols,
		ExportsRef:              p.exportsRef,
		ModuleRef:               p.moduleRef,
		WrapperRef:              wrapperRef,
		Hashbang:                hashbang,
		Directive:               directive,
		NamedImports:            p.namedImports,
		NamedExports:            p.namedExports,
		NestedScopeSlotCounts:   nestedScopeSlotCounts,
		TopLevelSymbolToParts:   p.topLevelSymbolToParts,
		ExportStarImportRecords: p.exportStarImportRecords,
		ImportRecords:           p.importRecords,
		ApproximateLineCount:    int32(p.lexer.ApproximateNewlineCount) + 1,

		// CommonJS features
		HasTopLevelReturn: p.hasTopLevelReturn,
		UsesExportsRef:    p.symbols[p.exportsRef.InnerIndex].UseCountEstimate > 0,
		UsesModuleRef:     p.symbols[p.moduleRef.InnerIndex].UseCountEstimate > 0,

		// ES6 features
		HasES6Imports: p.hasES6ImportSyntax,
		HasES6Exports: p.hasES6ExportSyntax,
	}
}
