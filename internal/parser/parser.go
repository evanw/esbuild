package parser

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
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
	log                      logging.Log
	source                   logging.Source
	lexer                    lexer.Lexer
	importPaths              []ast.ImportPath
	exportStars              []ast.Path
	allowIn                  bool
	allowPrivateIdentifiers  bool
	hasTopLevelReturn        bool
	currentFnOpts            fnOpts
	target                   LanguageTarget
	ts                       TypeScriptOptions
	jsx                      JSXOptions
	latestReturnHadSemicolon bool
	allocatedNames           []string
	latestArrowArgLoc        ast.Loc
	currentScope             *ast.Scope
	symbols                  []ast.Symbol
	tsUseCounts              []uint32
	exportsRef               ast.Ref
	requireRef               ast.Ref
	moduleRef                ast.Ref
	findSymbolHelper         FindSymbol
	useCountEstimates        map[ast.Ref]uint32
	declaredSymbols          []ast.DeclaredSymbol
	runtimeImports           map[string]ast.Ref

	// These are for TypeScript
	shouldFoldNumericConstants bool
	enclosingNamespaceRef      *ast.Ref
	emittedNamespaceVars       map[ast.Ref]bool
	isExportedInsideNamespace  map[ast.Ref]ast.Ref
	knownEnumValues            map[ast.Ref]map[string]float64

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
	mangleSyntax      bool
	isBundling        bool
	tryBodyCount      int
	isThisCaptured    bool
	callTarget        ast.E
	typeofTarget      ast.E
	moduleScope       *ast.Scope
	isControlFlowDead bool
	processedDefines  ProcessedDefines

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

type fnOpts struct {
	asyncRange     ast.Range
	isOutsideFn    bool
	allowAwait     bool
	allowYield     bool
	allowSuperCall bool

	// In TypeScript, forward declarations of functions have no bodies
	allowMissingBodyForTypeScript bool
}

func isJumpStatement(data ast.S) bool {
	switch data.(type) {
	case *ast.SBreak, *ast.SContinue, *ast.SReturn, *ast.SThrow:
		return true
	}

	return false
}

func hasNoSideEffects(data ast.E) bool {
	switch e := data.(type) {
	case *ast.ENull, *ast.EUndefined, *ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.EString, *ast.EThis, *ast.ERegExp, *ast.EFunction, *ast.EArrow:
		return true

	case *ast.EDot:
		return e.CanBeRemovedIfUnused
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

func checkEqualityIfNoSideEffects(left ast.E, right ast.E) (bool, bool) {
	switch l := left.(type) {
	case *ast.ENull:
		if _, ok := right.(*ast.ENull); ok {
			return true, true
		}

	case *ast.EUndefined:
		if _, ok := right.(*ast.EUndefined); ok {
			return true, true
		}

	case *ast.EBoolean:
		if r, ok := right.(*ast.EBoolean); ok {
			return l.Value == r.Value, true
		}

	case *ast.ENumber:
		if r, ok := right.(*ast.ENumber); ok {
			return l.Value == r.Value, true
		}

	case *ast.EBigInt:
		if r, ok := right.(*ast.EBigInt); ok {
			return l.Value == r.Value, true
		}

	case *ast.EString:
		if r, ok := right.(*ast.EString); ok {
			lv := l.Value
			rv := r.Value
			if len(lv) != len(rv) {
				return false, true
			}
			for i := 0; i < len(lv); i++ {
				if lv[i] != rv[i] {
					return false, true
				}
			}
			return true, true
		}
	}

	return false, false
}

func (p *parser) pushScopeForParsePass(kind ast.ScopeKind, loc ast.Loc) int {
	parent := p.currentScope
	scope := &ast.Scope{
		Kind:     kind,
		Parent:   parent,
		Members:  make(map[string]ast.Ref),
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
		for name, ref := range scope.Parent.Members {
			// Don't copy down the optional function expression name. Re-declaring
			// the name of a function expression is allowed.
			if p.symbols[ref.InnerIndex].Kind != ast.SymbolHoistedFunction {
				scope.Members[name] = ref
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
		for _, ref := range p.currentScope.Members {
			p.symbols[ref.InnerIndex].MustNotBeRenamed = true
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

func (p *parser) newSymbol(kind ast.SymbolKind, name string) ast.Ref {
	ref := ast.Ref{p.source.Index, uint32(len(p.symbols))}
	p.symbols = append(p.symbols, ast.Symbol{
		Kind: kind,
		Name: name,
		Link: ast.InvalidRef,
	})
	if p.ts.Parse {
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
)

func canMergeSymbols(existing ast.SymbolKind, new ast.SymbolKind) mergeResult {
	if existing == ast.SymbolUnbound {
		return mergeReplaceWithNew
	}

	// "import {Foo} from 'bar'; class Foo {}"
	if existing == ast.SymbolTSImport {
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

	return mergeForbidden
}

func (p *parser) declareSymbol(kind ast.SymbolKind, loc ast.Loc, name string) ast.Ref {
	scope := p.currentScope

	// Check for collisions that would prevent to hoisting "var" symbols up to the enclosing function scope
	if kind.IsHoisted() {
		for !scope.Kind.StopsHoisting() {
			if existing, ok := scope.Members[name]; ok {
				symbol := p.symbols[existing.InnerIndex]
				switch symbol.Kind {
				case ast.SymbolUnbound, ast.SymbolHoisted, ast.SymbolHoistedFunction:
					// Continue on to the parent scope
				case ast.SymbolCatchIdentifier:
					// This is a weird special case. Silently merge the existing symbol
					// into this one. The merging will happen later on after the new
					// symbol exists.
				default:
					r := lexer.RangeOfIdentifier(p.source, loc)
					p.log.AddRangeError(p.source, r, fmt.Sprintf("%q has already been declared", name))
					return existing
				}
			}
			scope = scope.Parent
		}
	}

	// Allocate a new symbol
	ref := p.newSymbol(kind, name)

	// Check for a collision in the declaring scope
	if existing, ok := scope.Members[name]; ok {
		symbol := &p.symbols[existing.InnerIndex]

		switch canMergeSymbols(symbol.Kind, kind) {
		case mergeForbidden:
			r := lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddRangeError(p.source, r, fmt.Sprintf("%q has already been declared", name))
			return existing

		case mergeKeepExisting:
			ref = existing

		case mergeReplaceWithNew:
			symbol.Link = ref

		case mergeBecomePrivateGetSetPair:
			ref = existing
			symbol.Kind = ast.SymbolPrivateGetSetPair
		}
	}

	// Hoist "var" symbols up to the enclosing function scope
	if kind.IsHoisted() {
		for s := p.currentScope; !s.Kind.StopsHoisting(); s = s.Parent {
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
				p.symbols[ref.InnerIndex].MustNotBeRenamed = true
			}

			if existing, ok := s.Members[name]; ok {
				symbol := p.symbols[existing.InnerIndex]

				// See "VariableStatements in Catch blocks" in the spec for why we
				// special-case catch identifiers here:
				//
				//   http://www.ecma-international.org/ecma-262/6.0/#sec-variablestatements-in-catch-blocks
				//
				if symbol.Kind == ast.SymbolUnbound || symbol.Kind == ast.SymbolCatchIdentifier {
					p.symbols[existing.InnerIndex].Link = ref
				}
			}

			// Add the symbol to all parent scopes, not just the one it's declared
			// in. This prevents us from later on declaring a symbol that would
			// interfere with the hoisting:
			//
			//   {
			//     {
			//       var x;
			//     }
			//     let x; // SyntaxError: Identifier 'x' has already been declared
			//   }
			//
			s.Members[name] = ref
		}
	}

	// Overwrite this name in the declaring scope
	scope.Members[name] = ref
	return ref
}

func (p *parser) declareBinding(kind ast.SymbolKind, binding ast.Binding, opts parseStmtOpts) {
	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		name := p.loadNameFromRef(b.Ref)
		if !opts.isTypeScriptDeclare {
			b.Ref = p.declareSymbol(kind, binding.Loc, name)
			if opts.isExport && p.enclosingNamespaceRef == nil {
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
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

func (p *parser) recordExport(loc ast.Loc, alias string, ref ast.Ref) {
	// This is only an ES6 export if we're not inside a TypeScript namespace
	if p.enclosingNamespaceRef == nil {
		if _, ok := p.namedExports[alias]; ok {
			// Warn about duplicate exports
			p.log.AddRangeError(p.source, lexer.RangeOfIdentifier(p.source, loc),
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
		p.useCountEstimates[ref]++
	}

	// The correctness of TypeScript-to-JavaScript conversion relies on accurate
	// symbol use counts for the whole file, including dead code regions. This is
	// tracked separately in a parser-only data structure.
	if p.ts.Parse {
		p.tsUseCounts[ref.InnerIndex]++
	}
}

func (p *parser) ignoreUsage(ref ast.Ref) {
	// Roll back the use count increment in recordUsage()
	if !p.isControlFlowDead {
		p.symbols[ref.InnerIndex].UseCountEstimate--
		p.useCountEstimates[ref]--
	}

	// Don't roll back the "tsUseCounts" increment. This must be counted even if
	// the value is ignored because that's what the TypeScript compiler does.
}

func (p *parser) addError(loc ast.Loc, text string) {
	p.log.AddError(p.source, loc, text)
}

func (p *parser) addRangeError(r ast.Range, text string) {
	p.log.AddRangeError(p.source, r, text)
}

func (p *parser) callRuntime(loc ast.Loc, name string, args []ast.Expr) ast.Expr {
	ref, ok := p.runtimeImports[name]
	if !ok {
		ref = p.newSymbol(ast.SymbolOther, name)
		p.runtimeImports[name] = ref
	}
	p.recordUsage(ref)
	return ast.Expr{loc, &ast.ECall{
		Target: ast.Expr{loc, &ast.EIdentifier{ref}},
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
		return ast.Ref{-uint32(n.Len), uint32(n.Data - c.Data)}
	} else {
		// The name is some memory allocated elsewhere. This is either an inline
		// string constant in the parser or an identifier with escape sequences
		// in the source code, which is very unusual. Stash it away for later.
		// This uses allocations but it should hopefully be very uncommon.
		ref := ast.Ref{0x80000000, uint32(len(p.allocatedNames))}
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

func (p *parser) skipTypeScriptBinding() {
	switch p.lexer.Token {
	case lexer.TIdentifier, lexer.TThis:
		p.lexer.Next()

	case lexer.TOpenBracket:
		p.lexer.Next()

		// "[, , a]"
		for p.lexer.Token == lexer.TComma {
			p.lexer.Next()
		}

		// "[a, b]"
		for p.lexer.Token != lexer.TCloseBracket {
			p.skipTypeScriptBinding()
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBracket)

	case lexer.TOpenBrace:
		p.lexer.Next()

		for p.lexer.Token != lexer.TCloseBrace {
			foundIdentifier := false

			switch p.lexer.Token {
			case lexer.TIdentifier:
				// "{x}"
				// "{x: y}"
				foundIdentifier = true
				p.lexer.Next()

				// "{1: y}"
				// "{'x': y}"
			case lexer.TStringLiteral, lexer.TNumericLiteral:
				p.lexer.Next()

			default:
				if p.lexer.IsIdentifierOrKeyword() {
					// "{if: x}"
					p.lexer.Next()
				} else {
					p.lexer.Unexpected()
				}
			}

			if p.lexer.Token == lexer.TColon || !foundIdentifier {
				p.lexer.Expect(lexer.TColon)
				p.skipTypeScriptBinding()
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBrace)

	default:
		p.lexer.Unexpected()
	}
}

func (p *parser) skipTypeScriptFnArgs() {
	p.lexer.Expect(lexer.TOpenParen)

	for p.lexer.Token != lexer.TCloseParen {
		// "(...a)"
		if p.lexer.Token == lexer.TDotDotDot {
			p.lexer.Next()
		}

		p.skipTypeScriptBinding()

		// "(a?)"
		if p.lexer.Token == lexer.TQuestion {
			p.lexer.Next()
		}

		// "(a: any)"
		if p.lexer.Token == lexer.TColon {
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)
		}

		// "(a, b)"
		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)
}

// This is a spot where the TypeScript grammar is highly ambiguous. Here are
// some cases that are valid:
//
//     let x = (y: any): (() => {}) => { };
//     let x = (y: any): () => {} => { };
//     let x = (y: any): (y) => {} => { };
//     let x = (y: any): (y[]) => {};
//     let x = (y: any): (a | b) => {};
//
// Here are some cases that aren't valid:
//
//     let x = (y: any): (y) => {};
//     let x = (y: any): (y) => {return 0};
//     let x = (y: any): asserts y is (y) => {};
//
func (p *parser) skipTypeScriptParenOrFnType() {
	if p.trySkipTypeScriptArrowArgsWithBacktracking() {
		p.skipTypeScriptReturnType()
	} else {
		p.lexer.Expect(lexer.TOpenParen)
		p.skipTypeScriptType(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
	}
}

func (p *parser) skipTypeScriptReturnType() {
	// Skip over "function assert(x: boolean): asserts x"
	if p.lexer.IsContextualKeyword("asserts") {
		p.lexer.Next()

		// "function assert(x: boolean): asserts" is also valid
		if p.lexer.Token != lexer.TIdentifier && p.lexer.Token != lexer.TThis {
			return
		}
		p.lexer.Next()

		// Continue on to the "is" check below to handle something like
		// "function assert(x: any): asserts x is boolean"
	} else {
		p.skipTypeScriptType(ast.LLowest)
	}

	if p.lexer.IsContextualKeyword("is") && !p.lexer.HasNewlineBefore {
		p.lexer.Next()
		p.skipTypeScriptType(ast.LLowest)
	}
}

func (p *parser) skipTypeScriptType(level ast.L) {
	p.skipTypeScriptTypePrefix()
	p.skipTypeScriptTypeSuffix(level)
}

func (p *parser) skipTypeScriptTypePrefix() {
	switch p.lexer.Token {
	case lexer.TNumericLiteral, lexer.TBigIntegerLiteral, lexer.TStringLiteral,
		lexer.TNoSubstitutionTemplateLiteral, lexer.TThis, lexer.TTrue, lexer.TFalse,
		lexer.TNull, lexer.TVoid, lexer.TConst:
		p.lexer.Next()

	case lexer.TMinus:
		p.lexer.Next()
		if p.lexer.Token == lexer.TBigIntegerLiteral {
			p.lexer.Next()
		} else {
			p.lexer.Expect(lexer.TNumericLiteral)
		}

	case lexer.TAmpersand:
	case lexer.TBar:
		// Support things like "type Foo = | A | B" and "type Foo = & A & B"
		p.lexer.Next()
		p.skipTypeScriptTypePrefix()

	case lexer.TImport:
		// "import('fs')"
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		p.lexer.Expect(lexer.TStringLiteral)
		p.lexer.Expect(lexer.TCloseParen)

	case lexer.TNew:
		// "new () => Foo"
		// "new <T>() => Foo<T>"
		p.lexer.Next()
		p.skipTypeScriptTypeParameters()
		p.skipTypeScriptParenOrFnType()

	case lexer.TLessThan:
		// "<T>() => Foo<T>"
		p.skipTypeScriptTypeParameters()
		p.skipTypeScriptParenOrFnType()

	case lexer.TOpenParen:
		p.skipTypeScriptParenOrFnType()

	case lexer.TIdentifier:
		switch p.lexer.Identifier {
		case "keyof", "readonly", "infer":
			p.lexer.Next()
			p.skipTypeScriptType(ast.LPrefix)

		case "unique":
			p.lexer.Next()
			if p.lexer.IsContextualKeyword("symbol") {
				p.lexer.Next()
			}

		default:
			p.lexer.Next()
		}

	case lexer.TTypeof:
		p.lexer.Next()
		p.skipTypeScriptType(ast.LPrefix)

	case lexer.TOpenBracket:
		p.lexer.Next()
		for p.lexer.Token != lexer.TCloseBracket {
			if p.lexer.Token == lexer.TDotDotDot {
				p.lexer.Next()
			}
			p.skipTypeScriptType(ast.LLowest)
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}
		p.lexer.Expect(lexer.TCloseBracket)

	case lexer.TOpenBrace:
		p.skipTypeScriptObjectType()

	default:
		p.lexer.Unexpected()
	}
}

func (p *parser) skipTypeScriptTypeSuffix(level ast.L) {
	for {
		switch p.lexer.Token {
		case lexer.TBar:
			if level >= ast.LBitwiseOr {
				return
			}
			p.lexer.Next()
			p.skipTypeScriptType(ast.LBitwiseOr)

		case lexer.TAmpersand:
			if level >= ast.LBitwiseAnd {
				return
			}
			p.lexer.Next()
			p.skipTypeScriptType(ast.LBitwiseAnd)

		case lexer.TDot:
			p.lexer.Next()
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.Next()

		case lexer.TOpenBracket:
			// "{ ['x']: string \n ['y']: string }" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.Next()
			if p.lexer.Token != lexer.TCloseBracket {
				p.skipTypeScriptType(ast.LLowest)
			}
			p.lexer.Expect(lexer.TCloseBracket)

		case lexer.TLessThan, lexer.TLessThanEquals,
			lexer.TLessThanLessThan, lexer.TLessThanLessThanEquals:
			// "let foo: any \n <number>foo" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.ExpectLessThan(false /* isInsideJSXElement */)
			for {
				p.skipTypeScriptType(ast.LLowest)
				if p.lexer.Token != lexer.TComma {
					break
				}
				p.lexer.Next()
			}
			p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)

		case lexer.TExtends:
			// "{ x: number \n extends: boolean }" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.Next()
			p.skipTypeScriptType(ast.LCompare)

		case lexer.TQuestion:
			if level >= ast.LConditional {
				return
			}
			p.lexer.Next()

			switch p.lexer.Token {
			// Stop now if we're parsing one of these:
			// "(a?: b) => void"
			// "(a?, b?) => void"
			// "(a?) => void"
			// "[string?]"
			case lexer.TColon, lexer.TComma, lexer.TCloseParen, lexer.TCloseBracket:
				return
			}

			p.skipTypeScriptType(ast.LLowest)
			p.lexer.Expect(lexer.TColon)
			p.skipTypeScriptType(ast.LLowest)

		default:
			return
		}
	}
}

func (p *parser) skipTypeScriptObjectType() {
	p.lexer.Expect(lexer.TOpenBrace)

	for p.lexer.Token != lexer.TCloseBrace {
		// "{ -readonly [K in keyof T]: T[K] }"
		// "{ +readonly [K in keyof T]: T[K] }"
		if p.lexer.Token == lexer.TPlus || p.lexer.Token == lexer.TMinus {
			p.lexer.Next()
		}

		// Skip over modifiers and the property identifier
		foundKey := false
		for p.lexer.IsIdentifierOrKeyword() ||
			p.lexer.Token == lexer.TStringLiteral ||
			p.lexer.Token == lexer.TNumericLiteral {
			p.lexer.Next()
			foundKey = true
		}

		if p.lexer.Token == lexer.TOpenBracket {
			// Index signature or computed property
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)

			// "{ [key: string]: number }"
			// "{ readonly [K in keyof T]: T[K] }"
			if p.lexer.Token == lexer.TColon || p.lexer.Token == lexer.TIn {
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
			}

			p.lexer.Expect(lexer.TCloseBracket)

			// "{ [K in keyof T]+?: T[K] }"
			// "{ [K in keyof T]-?: T[K] }"
			if p.lexer.Token == lexer.TPlus || p.lexer.Token == lexer.TMinus {
				p.lexer.Next()
			}

			foundKey = true
		}

		// "?" indicates an optional property
		// "!" indicates an initialization assertion
		if foundKey && (p.lexer.Token == lexer.TQuestion || p.lexer.Token == lexer.TExclamation) {
			p.lexer.Next()
		}

		// Type parameters come right after the optional mark
		p.skipTypeScriptTypeParameters()

		switch p.lexer.Token {
		case lexer.TColon:
			// Regular property
			if !foundKey {
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)

		case lexer.TOpenParen:
			// Method signature
			p.skipTypeScriptFnArgs()
			if p.lexer.Token == lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptReturnType()
			}

		default:
			if !foundKey {
				p.lexer.Unexpected()
			}
		}

		switch p.lexer.Token {
		case lexer.TCloseBrace:

		case lexer.TComma, lexer.TSemicolon:
			p.lexer.Next()

		default:
			if !p.lexer.HasNewlineBefore {
				p.lexer.Unexpected()
			}
		}
	}

	p.lexer.Expect(lexer.TCloseBrace)
}

// This is the type parameter declarations that go with other symbol
// declarations (class, function, type, etc.)
func (p *parser) skipTypeScriptTypeParameters() {
	if p.lexer.Token == lexer.TLessThan {
		p.lexer.Next()

		for {
			p.lexer.Expect(lexer.TIdentifier)

			// "class Foo<T extends number> {}"
			if p.lexer.Token == lexer.TExtends {
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
			}

			// "class Foo<T = void> {}"
			if p.lexer.Token == lexer.TEquals {
				p.lexer.Next()
				p.skipTypeScriptType(ast.LLowest)
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
			if p.lexer.Token == lexer.TGreaterThan {
				break
			}
		}

		p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)
	}
}

func (p *parser) skipTypeScriptTypeArguments(isInsideJSXElement bool) bool {
	if p.lexer.Token != lexer.TLessThan {
		return false
	}

	p.lexer.Next()

	for {
		p.skipTypeScriptType(ast.LLowest)
		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	// This type argument list must end with a ">"
	p.lexer.ExpectGreaterThan(isInsideJSXElement)
	return true
}

func (p *parser) trySkipTypeScriptTypeArgumentsWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)

	// Check the token after this and backtrack if it's the wrong one
	if !p.canFollowTypeArgumentsInExpression() {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipTypeScriptTypeParameters()
	if p.lexer.Token != lexer.TOpenParen {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipTypeScriptArrowReturnTypeWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.lexer.Expect(lexer.TColon)
	p.skipTypeScriptReturnType()

	// Check the token after this and backtrack if it's the wrong one
	if p.lexer.Token != lexer.TEqualsGreaterThan {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipTypeScriptArrowArgsWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipTypeScriptFnArgs()
	p.lexer.Expect(lexer.TEqualsGreaterThan)

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

// This function is taken from the official TypeScript compiler source code:
// https://github.com/microsoft/TypeScript/blob/master/src/compiler/parser.ts
func (p *parser) canFollowTypeArgumentsInExpression() bool {
	switch p.lexer.Token {
	case
		// These are the only tokens can legally follow a type argument list. So we
		// definitely want to treat them as type arg lists.
		lexer.TOpenParen,                     // foo<x>(
		lexer.TNoSubstitutionTemplateLiteral, // foo<T> `...`
		lexer.TTemplateHead:                  // foo<T> `...${100}...`
		return true

	case
		// These cases can't legally follow a type arg list. However, they're not
		// legal expressions either. The user is probably in the middle of a
		// generic type. So treat it as such.
		lexer.TDot,                     // foo<x>.
		lexer.TCloseParen,              // foo<x>)
		lexer.TCloseBracket,            // foo<x>]
		lexer.TColon,                   // foo<x>:
		lexer.TSemicolon,               // foo<x>;
		lexer.TQuestion,                // foo<x>?
		lexer.TEqualsEquals,            // foo<x> ==
		lexer.TEqualsEqualsEquals,      // foo<x> ===
		lexer.TExclamationEquals,       // foo<x> !=
		lexer.TExclamationEqualsEquals, // foo<x> !==
		lexer.TAmpersandAmpersand,      // foo<x> &&
		lexer.TBarBar,                  // foo<x> ||
		lexer.TQuestionQuestion,        // foo<x> ??
		lexer.TCaret,                   // foo<x> ^
		lexer.TAmpersand,               // foo<x> &
		lexer.TBar,                     // foo<x> |
		lexer.TCloseBrace,              // foo<x> }
		lexer.TEndOfFile:               // foo<x>
		return true

	case
		// We don't want to treat these as type arguments. Otherwise we'll parse
		// this as an invocation expression. Instead, we want to parse out the
		// expression in isolation from the type arguments.
		lexer.TComma,     // foo<x>,
		lexer.TOpenBrace: // foo<x> {
		return false

	default:
		// Anything else treat as an expression.
		return false
	}
}

func (p *parser) skipTypeScriptTypeStmt(opts parseStmtOpts) {
	if opts.isExport && p.lexer.Token == lexer.TOpenBrace {
		// "export type {foo}"
		// "export type {foo} from 'bar'"
		p.parseExportClause()
		if p.lexer.IsContextualKeyword("from") {
			p.lexer.Next()
			p.parsePath()
		}
		p.lexer.ExpectOrInsertSemicolon()
		return
	}

	p.lexer.Expect(lexer.TIdentifier)
	p.skipTypeScriptTypeParameters()
	p.lexer.Expect(lexer.TEquals)
	p.skipTypeScriptType(ast.LLowest)
	p.lexer.ExpectOrInsertSemicolon()
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
		p.addRangeError(errors.invalidExprDefaultValue, "Unexpected \"=\"")
	}

	if errors.invalidExprAfterQuestion.Len > 0 {
		r := errors.invalidExprAfterQuestion
		p.addRangeError(r, fmt.Sprintf("Unexpected %q", p.source.Contents[r.Loc.Start:r.Loc.Start+r.Len]))
	}
}

func (p *parser) logBindingErrors(errors *deferredErrors) {
	if errors.invalidBindingCommaAfterSpread.Len > 0 {
		p.addRangeError(errors.invalidBindingCommaAfterSpread, "Unexpected \",\" after rest pattern")
	}
}

type propertyContext int

const (
	propertyContextObject = iota
	propertyContextClass
)

type propertyOpts struct {
	asyncRange      ast.Range
	isAsync         bool
	isGenerator     bool
	isStatic        bool
	classHasExtends bool
}

func (p *parser) parseProperty(
	context propertyContext, kind ast.PropertyKind, opts propertyOpts, errors *deferredErrors,
) (ast.Property, bool) {
	var key ast.Expr
	keyRange := p.lexer.Range()
	isComputed := false

	switch p.lexer.Token {
	case lexer.TNumericLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.ENumber{p.lexer.Number}}
		p.lexer.Next()

	case lexer.TStringLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
		p.lexer.Next()

	case lexer.TPrivateIdentifier:
		if context != propertyContextClass {
			p.lexer.Expected(lexer.TIdentifier)
		}
		p.markFutureSyntax(futureSyntaxPrivateName, p.lexer.Range())
		key = ast.Expr{p.lexer.Loc(), &ast.EPrivateIdentifier{p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Next()

	case lexer.TOpenBracket:
		isComputed = true
		p.lexer.Next()
		wasIdentifier := p.lexer.Token == lexer.TIdentifier
		expr := p.parseExpr(ast.LComma)

		// Handle index signatures
		if p.ts.Parse && p.lexer.Token == lexer.TColon && wasIdentifier &&
			context == propertyContextClass {
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
		return p.parseProperty(context, ast.PropertyNormal, opts, errors)

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
						return p.parseProperty(context, ast.PropertyGet, opts, nil)
					}

				case "set":
					if !opts.isAsync {
						return p.parseProperty(context, ast.PropertySet, opts, nil)
					}

				case "async":
					if !opts.isAsync {
						opts.isAsync = true
						opts.asyncRange = nameRange
						return p.parseProperty(context, kind, opts, nil)
					}

				case "static":
					if !opts.isStatic && !opts.isAsync && context == propertyContextClass {
						opts.isStatic = true
						return p.parseProperty(context, kind, opts, nil)
					}

				case "private", "protected", "public", "readonly", "abstract":
					// Skip over TypeScript keywords
					if context == propertyContextClass && p.ts.Parse {
						return p.parseProperty(context, kind, opts, nil)
					}
				}
			}
		}

		key = ast.Expr{nameRange.Loc, &ast.EString{lexer.StringToUTF16(name)}}

		// Parse a shorthand property
		if context == propertyContextObject && kind == ast.PropertyNormal && p.lexer.Token != lexer.TColon &&
			p.lexer.Token != lexer.TOpenParen && p.lexer.Token != lexer.TLessThan && !opts.isGenerator {
			ref := p.storeNameInRef(name)
			value := ast.Expr{key.Loc, &ast.EIdentifier{ref}}

			// Destructuring patterns have an optional default value
			var initializer *ast.Expr = nil
			if errors != nil && p.lexer.Token == lexer.TEquals {
				errors.invalidExprDefaultValue = p.lexer.Range()
				p.lexer.Next()
				value := p.parseExpr(ast.LComma)
				initializer = &value
			}

			return ast.Property{
				Kind:        kind,
				Key:         key,
				Value:       &value,
				Initializer: initializer,
			}, true
		}
	}

	if p.ts.Parse {
		// "class X { foo?: number }"
		// "class X { foo!: number }"
		if context == propertyContextClass && (p.lexer.Token == lexer.TQuestion || p.lexer.Token == lexer.TExclamation) {
			p.lexer.Next()
		}

		// "class X { foo?<T>(): T }"
		// "const x = { foo<T>(): T {} }"
		p.skipTypeScriptTypeParameters()
	}

	// Parse a class field with an optional initial value
	if context == propertyContextClass && kind == ast.PropertyNormal &&
		!opts.isAsync && !opts.isGenerator && p.lexer.Token != lexer.TOpenParen {
		var initializer *ast.Expr

		// Forbid the names "constructor" and "prototype" in some cases
		if !isComputed {
			if str, ok := key.Data.(*ast.EString); ok && (lexer.UTF16EqualsString(str.Value, "constructor") ||
				(opts.isStatic && lexer.UTF16EqualsString(str.Value, "prototype"))) {
				p.addRangeError(keyRange, fmt.Sprintf("Invalid field name %q", lexer.UTF16ToString(str.Value)))
			}
		}

		// Skip over types
		if p.ts.Parse && p.lexer.Token == lexer.TColon {
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
				p.addRangeError(keyRange, fmt.Sprintf("Invalid field name %q", name))
			}
			private.Ref = p.declareSymbol(ast.SymbolPrivate, key.Loc, name)
		}

		p.lexer.ExpectOrInsertSemicolon()
		return ast.Property{
			Kind:        kind,
			IsComputed:  isComputed,
			IsStatic:    opts.isStatic,
			Key:         key,
			Initializer: initializer,
		}, true
	}

	// Parse a method expression
	if p.lexer.Token == lexer.TOpenParen || kind != ast.PropertyNormal ||
		context == propertyContextClass || opts.isAsync || opts.isGenerator {
		loc := p.lexer.Loc()
		scopeIndex := p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)
		isConstructor := false

		// Forbid the names "constructor" and "prototype" in some cases
		if context == propertyContextClass && !isComputed {
			if str, ok := key.Data.(*ast.EString); ok {
				if !opts.isStatic && lexer.UTF16EqualsString(str.Value, "constructor") {
					switch {
					case kind == ast.PropertyGet:
						p.addRangeError(keyRange, "Class constructor cannot be a getter")
					case kind == ast.PropertySet:
						p.addRangeError(keyRange, "Class constructor cannot be a setter")
					case opts.isAsync:
						p.addRangeError(keyRange, "Class constructor cannot be an async function")
					case opts.isGenerator:
						p.addRangeError(keyRange, "Class constructor cannot be a generator")
					default:
						isConstructor = true
					}
				} else if opts.isStatic && lexer.UTF16EqualsString(str.Value, "prototype") {
					p.addRangeError(keyRange, "Invalid static method name \"prototype\"")
				}
			}
		}

		fn, hadBody := p.parseFn(nil, fnOpts{
			asyncRange:     opts.asyncRange,
			allowAwait:     opts.isAsync,
			allowYield:     opts.isGenerator,
			allowSuperCall: opts.classHasExtends && isConstructor,

			// Only allow omitting the body if we're parsing TypeScript class
			allowMissingBodyForTypeScript: p.ts.Parse && context == propertyContextClass,
		})

		// "class Foo { foo(): void; foo(): void {} }"
		if !hadBody {
			// Skip this property entirely
			p.popAndDiscardScope(scopeIndex)
			return ast.Property{}, false
		}

		p.popScope()
		value := ast.Expr{loc, &ast.EFunction{fn}}

		// Special-case private identifiers
		if private, ok := key.Data.(*ast.EPrivateIdentifier); ok {
			var declare ast.SymbolKind
			switch kind {
			case ast.PropertyGet:
				declare = ast.SymbolPrivateGet
			case ast.PropertySet:
				declare = ast.SymbolPrivateSet
			default:
				declare = ast.SymbolPrivate
			}
			name := p.loadNameFromRef(private.Ref)
			if name == "#constructor" {
				p.addRangeError(keyRange, fmt.Sprintf("Invalid method name %q", name))
			}
			private.Ref = p.declareSymbol(declare, key.Loc, name)
		}

		return ast.Property{
			Kind:       kind,
			IsComputed: isComputed,
			IsMethod:   true,
			IsStatic:   opts.isStatic,
			Key:        key,
			Value:      &value,
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
		p.markFutureSyntax(futureSyntaxRestProperty, p.lexer.Range())
		p.lexer.Next()
		value := ast.Binding{p.lexer.Loc(), &ast.BIdentifier{p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Expect(lexer.TIdentifier)
		return ast.PropertyBinding{
			IsSpread: true,
			Value:    value,
		}

	case lexer.TNumericLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.ENumber{p.lexer.Number}}
		p.lexer.Next()

	case lexer.TStringLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
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
		key = ast.Expr{loc, &ast.EString{lexer.StringToUTF16(name)}}

		if p.lexer.Token != lexer.TColon && p.lexer.Token != lexer.TOpenParen {
			ref := p.storeNameInRef(name)
			value := ast.Binding{loc, &ast.BIdentifier{ref}}

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

// This assumes that the "=>" token has already been parsed by the caller
func (p *parser) parseArrowBody(args []ast.Arg, opts fnOpts) *ast.EArrow {
	arrowLoc := p.lexer.Loc()

	// Newlines are not allowed before "=>"
	if p.lexer.HasNewlineBefore {
		p.log.AddRangeError(p.source, p.lexer.Range(), "Unexpected newline before \"=>\"")
		panic(lexer.LexerPanic{})
	}

	p.lexer.Expect(lexer.TEqualsGreaterThan)

	for _, arg := range args {
		p.declareBinding(ast.SymbolHoisted, arg.Binding, parseStmtOpts{})
	}

	// The ability to call "super()" is inherited by arrow functions
	opts.allowSuperCall = p.currentFnOpts.allowSuperCall

	if p.lexer.Token == lexer.TOpenBrace {
		return &ast.EArrow{
			Args: args,
			Body: p.parseFnBody(opts),
		}
	}

	p.pushScopeForParsePass(ast.ScopeFunctionBody, arrowLoc)
	defer p.popScope()

	oldFnOpts := p.currentFnOpts
	p.currentFnOpts = opts
	expr := p.parseExpr(ast.LComma)
	p.currentFnOpts = oldFnOpts
	return &ast.EArrow{
		Args:       args,
		PreferExpr: true,
		Body:       ast.FnBody{arrowLoc, []ast.Stmt{ast.Stmt{expr.Loc, &ast.SReturn{&expr}}}},
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
		p.markFutureSyntax(futureSyntaxAsync, asyncRange)
		return p.parseFnExpr(asyncRange.Loc, true /* isAsync */, asyncRange)

		// "async => {}"
	case lexer.TEqualsGreaterThan:
		arg := ast.Arg{Binding: ast.Binding{asyncRange.Loc, &ast.BIdentifier{p.storeNameInRef("async")}}}

		p.pushScopeForParsePass(ast.ScopeFunctionArgs, asyncRange.Loc)
		defer p.popScope()

		return ast.Expr{asyncRange.Loc, p.parseArrowBody([]ast.Arg{arg}, fnOpts{})}

		// "async x => {}"
	case lexer.TIdentifier:
		p.markFutureSyntax(futureSyntaxAsync, asyncRange)
		ref := p.storeNameInRef(p.lexer.Identifier)
		arg := ast.Arg{Binding: ast.Binding{p.lexer.Loc(), &ast.BIdentifier{ref}}}
		p.lexer.Next()

		p.pushScopeForParsePass(ast.ScopeFunctionArgs, asyncRange.Loc)
		defer p.popScope()

		arrow := p.parseArrowBody([]ast.Arg{arg}, fnOpts{allowAwait: true})
		arrow.IsAsync = true
		return ast.Expr{asyncRange.Loc, arrow}

		// "async()"
		// "async () => {}"
	case lexer.TOpenParen:
		p.lexer.Next()
		expr := p.parseParenExpr(asyncRange.Loc, parenExprOpts{isAsync: true})
		if _, ok := expr.Data.(*ast.EArrow); ok {
			p.markFutureSyntax(futureSyntaxAsync, asyncRange)
		}
		return expr

		// "async"
		// "async + 1"
	default:
		// Distinguish between a call like "async<T>()" and an arrow like "async <T>() => {}"
		if p.ts.Parse && p.lexer.Token == lexer.TLessThan && p.trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() {
			p.lexer.Next()
			expr := p.parseParenExpr(asyncRange.Loc, parenExprOpts{isAsync: true})
			if _, ok := expr.Data.(*ast.EArrow); ok {
				p.markFutureSyntax(futureSyntaxAsync, asyncRange)
			}
			return expr
		}

		return ast.Expr{asyncRange.Loc, &ast.EIdentifier{p.storeNameInRef("async")}}
	}
}

func (p *parser) parseFnExpr(loc ast.Loc, isAsync bool, asyncRange ast.Range) ast.Expr {
	p.lexer.Next()
	isGenerator := p.lexer.Token == lexer.TAsterisk
	if isGenerator {
		p.lexer.Next()
	}
	var name *ast.LocRef

	p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)
	defer p.popScope()

	// The name is optional
	if p.lexer.Token == lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		name = &ast.LocRef{nameLoc, p.declareSymbol(ast.SymbolHoistedFunction, nameLoc, p.lexer.Identifier)}
		p.lexer.Next()
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.ts.Parse {
		p.skipTypeScriptTypeParameters()
	}

	fn, _ := p.parseFn(name, fnOpts{
		asyncRange: asyncRange,
		allowAwait: isAsync,
		allowYield: isGenerator,
	})
	return ast.Expr{loc, &ast.EFunction{fn}}
}

type parenExprOpts struct {
	isAsync      bool
	forceArrowFn bool
}

// This assumes that the open parenthesis has already been parsed by the caller
func (p *parser) parseParenExpr(loc ast.Loc, opts parenExprOpts) ast.Expr {
	items := []ast.Expr{}
	errors := deferredErrors{}
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
			item = ast.Expr{itemLoc, &ast.ESpread{item}}
		}

		// Skip over types
		if p.ts.Parse && p.lexer.Token == lexer.TColon {
			typeColonRange = p.lexer.Range()
			p.lexer.Next()
			p.skipTypeScriptType(ast.LLowest)
		}

		// There may be a "=" after the type
		if p.ts.Parse && p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			item = ast.Expr{item.Loc, &ast.EBinary{ast.BinOpAssign, item, p.parseExpr(ast.LComma)}}
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

	// Are these arguments to an arrow function?
	if p.lexer.Token == lexer.TEqualsGreaterThan || opts.forceArrowFn || (p.ts.Parse && p.lexer.Token == lexer.TColon) {
		invalidLog := []ast.Loc{}
		args := []ast.Arg{}

		// First, try converting the expressions to bindings
		for _, item := range items {
			if spread, ok := item.Data.(*ast.ESpread); ok {
				item = spread.Value
			}
			binding, initializer, log := p.convertExprToBindingAndInitializer(item, invalidLog)
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

			// Now that we've decided we're an arrow function, report binding pattern
			// conversion errors
			if len(invalidLog) > 0 {
				for _, loc := range invalidLog {
					p.addError(loc, "Invalid binding pattern")
				}
				panic(lexer.LexerPanic{})
			}

			arrow := p.parseArrowBody(args, fnOpts{allowAwait: opts.isAsync})
			arrow.IsAsync = opts.isAsync
			arrow.HasRestArg = spreadRange.Len > 0
			p.popScope()
			return ast.Expr{loc, arrow}
		}
	}

	// If we get here, it's not an arrow function so undo the pushing of the
	// scope we did earlier. This needs to flatten any child scopes into the
	// parent scope as if the scope was never pushed in the first place.
	p.popAndFlattenScope(scopeIndex)

	// If this isn't an arrow function, then types aren't allowed
	if typeColonRange.Len > 0 {
		p.addRangeError(typeColonRange, "Unexpected \":\"")
		panic(lexer.LexerPanic{})
	}

	// Are these arguments for a call to a function named "async"?
	if opts.isAsync {
		p.logExprErrors(&errors)
		async := ast.Expr{loc, &ast.EIdentifier{p.storeNameInRef("async")}}
		return ast.Expr{loc, &ast.ECall{
			Target: async,
			Args:   items,
		}}
	}

	// Is this a chain of expressions and comma operators?
	if len(items) > 0 {
		p.logExprErrors(&errors)
		if spreadRange.Len > 0 {
			p.addRangeError(spreadRange, "Unexpected \"...\"")
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
		return ast.Binding{expr.Loc, &ast.BMissing{}}, invalidLog

	case *ast.EIdentifier:
		return ast.Binding{expr.Loc, &ast.BIdentifier{e.Ref}}, invalidLog

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
			items = append(items, ast.ArrayBinding{binding, initializer})
		}
		return ast.Binding{expr.Loc, &ast.BArray{
			Items:     items,
			HasSpread: isSpread,
		}}, invalidLog

	case *ast.EObject:
		items := []ast.PropertyBinding{}
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
			items = append(items, ast.PropertyBinding{
				IsSpread:     item.Kind == ast.PropertySpread,
				IsComputed:   item.IsComputed,
				Key:          item.Key,
				Value:        binding,
				DefaultValue: initializer,
			})
		}
		return ast.Binding{expr.Loc, &ast.BObject{items}}, invalidLog

	default:
		invalidLog = append(invalidLog, expr.Loc)
		return ast.Binding{}, invalidLog
	}
}

func (p *parser) parsePrefix(level ast.L, errors *deferredErrors) ast.Expr {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TSuper:
		p.lexer.Next()

		switch p.lexer.Token {
		case lexer.TOpenParen:
			if level < ast.LCall && p.currentFnOpts.allowSuperCall {
				return ast.Expr{loc, &ast.ESuper{}}
			}

		case lexer.TDot, lexer.TOpenBracket:
			return ast.Expr{loc, &ast.ESuper{}}
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
		return ast.Expr{loc, &ast.EBoolean{false}}

	case lexer.TTrue:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EBoolean{true}}

	case lexer.TNull:
		p.lexer.Next()
		return ast.Expr{loc, &ast.ENull{}}

	case lexer.TThis:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EThis{}}

	case lexer.TYield:
		if !p.currentFnOpts.allowYield {
			p.addRangeError(p.lexer.Range(), "Cannot use \"yield\" outside a generator function")
			panic(lexer.LexerPanic{})
		}

		if level > ast.LAssign {
			p.addRangeError(p.lexer.Range(), "Cannot use a \"yield\" expression here without parentheses")
			panic(lexer.LexerPanic{})
		}

		p.lexer.Next()

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

		return ast.Expr{loc, &ast.EYield{value, isStar}}

	case lexer.TIdentifier:
		name := p.lexer.Identifier
		nameRange := p.lexer.Range()
		p.lexer.Next()

		// Handle async and await expressions
		if name == "async" {
			return p.parseAsyncPrefixExpr(nameRange)
		} else if p.currentFnOpts.allowAwait && name == "await" {
			return ast.Expr{loc, &ast.EAwait{p.parseExpr(ast.LPrefix)}}
		}

		// Handle the start of an arrow expression
		if p.lexer.Token == lexer.TEqualsGreaterThan {
			ref := p.storeNameInRef(name)
			arg := ast.Arg{Binding: ast.Binding{loc, &ast.BIdentifier{ref}}}

			p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)
			defer p.popScope()

			return ast.Expr{loc, p.parseArrowBody([]ast.Arg{arg}, fnOpts{})}
		}

		ref := p.storeNameInRef(name)
		return ast.Expr{loc, &ast.EIdentifier{ref}}

	case lexer.TStringLiteral:
		value := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{loc, &ast.EString{value}}

	case lexer.TNoSubstitutionTemplateLiteral:
		head := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{loc, &ast.ETemplate{nil, head, "", []ast.TemplatePart{}}}

	case lexer.TTemplateHead:
		head := p.lexer.StringLiteral
		parts := p.parseTemplateParts(false /* includeRaw */)
		return ast.Expr{loc, &ast.ETemplate{nil, head, "", parts}}

	case lexer.TNumericLiteral:
		value := p.lexer.Number
		p.lexer.Next()
		return ast.Expr{loc, &ast.ENumber{value}}

	case lexer.TBigIntegerLiteral:
		value := p.lexer.Identifier
		p.markFutureSyntax(futureSyntaxBigInteger, p.lexer.Range())
		p.lexer.Next()
		return ast.Expr{p.lexer.Loc(), &ast.EBigInt{value}}

	case lexer.TSlash, lexer.TSlashEquals:
		p.lexer.ScanRegExp()
		value := p.lexer.Raw()
		p.lexer.Next()
		return ast.Expr{loc, &ast.ERegExp{value}}

	case lexer.TVoid:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpVoid, value}}

	case lexer.TTypeof:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpTypeof, value}}

	case lexer.TDelete:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		if index, ok := value.Data.(*ast.EIndex); ok {
			if private, ok := index.Index.Data.(*ast.EPrivateIdentifier); ok {
				name := p.loadNameFromRef(private.Ref)
				r := ast.Range{index.Index.Loc, int32(len(name))}
				p.addRangeError(r, fmt.Sprintf("Deleting the private name %q is forbidden", name))
			}
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpDelete, value}}

	case lexer.TPlus:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpPos, value}}

	case lexer.TMinus:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpNeg, value}}

	case lexer.TTilde:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpCpl, value}}

	case lexer.TExclamation:
		p.lexer.Next()
		value := p.parseExpr(ast.LPrefix)
		if p.lexer.Token == lexer.TAsteriskAsterisk {
			p.lexer.Unexpected()
		}
		return ast.Expr{loc, &ast.EUnary{ast.UnOpNot, value}}

	case lexer.TMinusMinus:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpPreDec, p.parseExpr(ast.LPrefix)}}

	case lexer.TPlusPlus:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpPreInc, p.parseExpr(ast.LPrefix)}}

	case lexer.TFunction:
		return p.parseFnExpr(loc, false /* isAsync */, ast.Range{})

	case lexer.TClass:
		p.lexer.Next()
		var name *ast.LocRef

		if p.lexer.Token == lexer.TIdentifier {
			p.pushScopeForParsePass(ast.ScopeClassName, loc)
			nameLoc := p.lexer.Loc()
			name = &ast.LocRef{loc, p.declareSymbol(ast.SymbolOther, nameLoc, p.lexer.Identifier)}
			p.lexer.Next()
		}

		// Even anonymous classes can have TypeScript type parameters
		if p.ts.Parse {
			p.skipTypeScriptTypeParameters()
		}

		class := p.parseClass(name, parseClassOpts{})

		if name != nil {
			p.popScope()
		}

		return ast.Expr{loc, &ast.EClass{class}}

	case lexer.TNew:
		p.lexer.Next()

		// Special-case the weird "new.target" expression here
		if p.lexer.Token == lexer.TDot {
			p.lexer.Next()
			if p.lexer.Token != lexer.TIdentifier || p.lexer.Identifier != "target" {
				p.lexer.Unexpected()
			}
			p.lexer.Next()
			return ast.Expr{loc, &ast.ENewTarget{}}
		}

		target := p.parseExpr(ast.LCall)
		args := []ast.Expr{}

		if p.ts.Parse {
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

		return ast.Expr{loc, &ast.ENew{target, args}}

	case lexer.TOpenBracket:
		lineCountAtStart := p.lexer.ApproximateLineCount
		p.lexer.Next()
		items := []ast.Expr{}
		selfErrors := deferredErrors{}

		// Allow "in" inside arrays
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBracket {
			switch p.lexer.Token {
			case lexer.TComma:
				items = append(items, ast.Expr{loc, &ast.EMissing{}})

			case lexer.TDotDotDot:
				dotsLoc := p.lexer.Loc()
				p.lexer.Next()
				item := p.parseExprOrBindings(ast.LComma, &selfErrors)
				items = append(items, ast.Expr{dotsLoc, &ast.ESpread{item}})

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
			p.lexer.Next()
		}

		isSingleLine := p.lexer.ApproximateLineCount == lineCountAtStart
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

		return ast.Expr{loc, &ast.EArray{
			Items:        items,
			IsSingleLine: isSingleLine,
		}}

	case lexer.TOpenBrace:
		lineCountAtStart := p.lexer.ApproximateLineCount
		p.lexer.Next()
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
				if property, ok := p.parseProperty(propertyContextObject, ast.PropertyNormal, propertyOpts{}, &selfErrors); ok {
					properties = append(properties, property)
				}
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		isSingleLine := p.lexer.ApproximateLineCount == lineCountAtStart
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

		return ast.Expr{loc, &ast.EObject{
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

		if p.ts.Parse && p.jsx.Parse {
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

		if p.jsx.Parse {
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

		if p.ts.Parse {
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
			return p.parsePrefix(level, errors)
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

type futureSyntax uint8

const (
	futureSyntaxAsync futureSyntax = iota
	futureSyntaxAsyncGenerator
	futureSyntaxRestProperty
	futureSyntaxForAwait
	futureSyntaxBigInteger
	futureSyntaxNonIdentifierArrayRest
	futureSyntaxPrivateName
	futureSyntaxLogicalAssign
)

func (p *parser) markFutureSyntax(syntax futureSyntax, r ast.Range) {
	var target LanguageTarget

	switch syntax {
	case futureSyntaxAsync:
		target = ES2017
	case futureSyntaxAsyncGenerator:
		target = ES2018
	case futureSyntaxRestProperty:
		target = ES2018
	case futureSyntaxForAwait:
		target = ES2018
	case futureSyntaxBigInteger:
		target = ES2020
	case futureSyntaxNonIdentifierArrayRest:
		target = ES2016
	case futureSyntaxPrivateName:
		target = ESNext
	case futureSyntaxLogicalAssign:
		target = ESNext
	}

	if p.target < target {
		var name string
		yet := " yet"

		switch syntax {
		case futureSyntaxAsync:
			name = "Async functions"
		case futureSyntaxAsyncGenerator:
			name = "Async generator functions"
		case futureSyntaxRestProperty:
			name = "Rest properties"
		case futureSyntaxForAwait:
			name = "For-await loops"
		case futureSyntaxBigInteger:
			name = "Big integer literals"
			yet = "" // This will never be supported
		case futureSyntaxNonIdentifierArrayRest:
			name = "Non-identifier array rest patterns"
		case futureSyntaxPrivateName:
			name = "Private names"
		case futureSyntaxLogicalAssign:
			name = "Logical assignment operators"
		}

		p.log.AddRangeError(p.source, r,
			fmt.Sprintf("%s are from %s and transforming them to %s is not supported%s",
				name, targetTable[target], targetTable[p.target], yet))
	}
}

func (p *parser) parseImportExpr(loc ast.Loc) ast.Expr {
	// Parse an "import.meta" expression
	if p.lexer.Token == lexer.TDot {
		p.lexer.Next()
		if p.lexer.IsContextualKeyword("meta") {
			p.lexer.Next()
			return ast.Expr{loc, &ast.EImportMeta{}}
		} else {
			p.lexer.ExpectedString("\"meta\"")
		}
	}

	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	p.lexer.Expect(lexer.TOpenParen)
	value := p.parseExpr(ast.LComma)
	p.lexer.Expect(lexer.TCloseParen)

	p.allowIn = oldAllowIn
	return ast.Expr{loc, &ast.EImport{value}}
}

func (p *parser) parseExprOrBindings(level ast.L, errors *deferredErrors) ast.Expr {
	return p.parseSuffix(p.parsePrefix(level, errors), level, errors)
}

func (p *parser) parseExpr(level ast.L) ast.Expr {
	return p.parseSuffix(p.parsePrefix(level, nil), level, nil)
}

func (p *parser) parseSuffix(left ast.Expr, level ast.L, errors *deferredErrors) ast.Expr {
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
					left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpComma, left, p.parseExpr(ast.LComma)}}

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
				left = ast.Expr{left.Loc, &ast.EIndex{
					Target:        left,
					Index:         ast.Expr{nameLoc, &ast.EPrivateIdentifier{ref}},
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
				left = ast.Expr{left.Loc, &ast.EDot{
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
				p.lexer.Next()

				// Allow "in" inside the brackets
				oldAllowIn := p.allowIn
				p.allowIn = true

				index := p.parseExpr(ast.LLowest)

				p.allowIn = oldAllowIn

				p.lexer.Expect(lexer.TCloseBracket)
				left = ast.Expr{left.Loc, &ast.EIndex{
					Target:        left,
					Index:         index,
					OptionalChain: ast.OptionalChainStart,
				}}

			case lexer.TOpenParen:
				if level >= ast.LCall {
					return left
				}
				left = ast.Expr{left.Loc, &ast.ECall{
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
					left = ast.Expr{left.Loc, &ast.EIndex{
						Target:        left,
						Index:         ast.Expr{nameLoc, &ast.EPrivateIdentifier{ref}},
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
					left = ast.Expr{left.Loc, &ast.EDot{
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
			head := p.lexer.StringLiteral
			headRaw := p.lexer.RawTemplateContents()
			p.lexer.Next()
			tag := left
			left = ast.Expr{left.Loc, &ast.ETemplate{&tag, head, headRaw, []ast.TemplatePart{}}}

		case lexer.TTemplateHead:
			if level >= ast.LPrefix {
				return left
			}
			head := p.lexer.StringLiteral
			headRaw := p.lexer.RawTemplateContents()
			parts := p.parseTemplateParts(true /* includeRaw */)
			tag := left
			left = ast.Expr{left.Loc, &ast.ETemplate{&tag, head, headRaw, parts}}

		case lexer.TOpenBracket:
			p.lexer.Next()

			// Allow "in" inside the brackets
			oldAllowIn := p.allowIn
			p.allowIn = true

			index := p.parseExpr(ast.LLowest)

			p.allowIn = oldAllowIn

			p.lexer.Expect(lexer.TCloseBracket)
			left = ast.Expr{left.Loc, &ast.EIndex{
				Target:        left,
				Index:         index,
				OptionalChain: oldOptionalChain,
			}}
			optionalChain = oldOptionalChain

		case lexer.TOpenParen:
			if level >= ast.LCall {
				return left
			}
			left = ast.Expr{left.Loc, &ast.ECall{
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
			if p.ts.Parse && left.Loc == p.latestArrowArgLoc && (p.lexer.Token == lexer.TColon ||
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
			left = ast.Expr{left.Loc, &ast.EIf{left, yes, no}}

		case lexer.TExclamation:
			// Skip over TypeScript non-null assertions
			if p.lexer.HasNewlineBefore {
				return left
			}
			if !p.ts.Parse {
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
			left = ast.Expr{left.Loc, &ast.EUnary{ast.UnOpPostDec, left}}

		case lexer.TPlusPlus:
			if p.lexer.HasNewlineBefore || level >= ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EUnary{ast.UnOpPostInc, left}}

		case lexer.TComma:
			if level >= ast.LComma {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpComma, left, p.parseExpr(ast.LComma)}}

		case lexer.TPlus:
			if level >= ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpAdd, left, p.parseExpr(ast.LAdd)}}

		case lexer.TPlusEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpAddAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TMinus:
			if level >= ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpSub, left, p.parseExpr(ast.LAdd)}}

		case lexer.TMinusEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpSubAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAsterisk:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpMul, left, p.parseExpr(ast.LMultiply)}}

		case lexer.TAsteriskAsterisk:
			if level >= ast.LExponentiation {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpPow, left, p.parseExpr(ast.LExponentiation - 1)}}

		case lexer.TAsteriskAsteriskEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpPowAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAsteriskEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpMulAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TPercent:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpRem, left, p.parseExpr(ast.LMultiply)}}

		case lexer.TPercentEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpRemAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TSlash:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpDiv, left, p.parseExpr(ast.LMultiply)}}

		case lexer.TSlashEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpDivAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLooseEq, left, p.parseExpr(ast.LEquals)}}

		case lexer.TExclamationEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLooseNe, left, p.parseExpr(ast.LEquals)}}

		case lexer.TEqualsEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpStrictEq, left, p.parseExpr(ast.LEquals)}}

		case lexer.TExclamationEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpStrictNe, left, p.parseExpr(ast.LEquals)}}

		case lexer.TLessThan:
			// TypeScript allows type arguments to be specified with angle brackets
			// inside an expression. Unlike in other languages, this unfortunately
			// appears to require backtracking to parse.
			if p.ts.Parse && p.trySkipTypeScriptTypeArgumentsWithBacktracking() {
				optionalChain = oldOptionalChain
				continue
			}

			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLt, left, p.parseExpr(ast.LCompare)}}

		case lexer.TLessThanEquals:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLe, left, p.parseExpr(ast.LCompare)}}

		case lexer.TGreaterThan:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpGt, left, p.parseExpr(ast.LCompare)}}

		case lexer.TGreaterThanEquals:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpGe, left, p.parseExpr(ast.LCompare)}}

		case lexer.TLessThanLessThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShl, left, p.parseExpr(ast.LShift)}}

		case lexer.TLessThanLessThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShlAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TGreaterThanGreaterThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShr, left, p.parseExpr(ast.LShift)}}

		case lexer.TGreaterThanGreaterThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TGreaterThanGreaterThanGreaterThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpUShr, left, p.parseExpr(ast.LShift)}}

		case lexer.TGreaterThanGreaterThanGreaterThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpUShrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TQuestionQuestion:
			if level >= ast.LNullishCoalescing {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpNullishCoalescing, left, p.parseExpr(ast.LNullishCoalescing)}}

		case lexer.TQuestionQuestionEquals:
			if level >= ast.LAssign {
				return left
			}
			p.markFutureSyntax(futureSyntaxLogicalAssign, p.lexer.Range())
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpNullishCoalescingAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TBarBar:
			if level >= ast.LLogicalOr {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLogicalOr, left, p.parseExpr(ast.LLogicalOr)}}

		case lexer.TBarBarEquals:
			if level >= ast.LAssign {
				return left
			}
			p.markFutureSyntax(futureSyntaxLogicalAssign, p.lexer.Range())
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLogicalOrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAmpersandAmpersand:
			if level >= ast.LLogicalAnd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLogicalAnd, left, p.parseExpr(ast.LLogicalAnd)}}

		case lexer.TAmpersandAmpersandEquals:
			if level >= ast.LAssign {
				return left
			}
			p.markFutureSyntax(futureSyntaxLogicalAssign, p.lexer.Range())
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLogicalAndAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TBar:
			if level >= ast.LBitwiseOr {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseOr, left, p.parseExpr(ast.LBitwiseOr)}}

		case lexer.TBarEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseOrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAmpersand:
			if level >= ast.LBitwiseAnd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseAnd, left, p.parseExpr(ast.LBitwiseAnd)}}

		case lexer.TAmpersandEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseAndAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TCaret:
			if level >= ast.LBitwiseXor {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseXor, left, p.parseExpr(ast.LBitwiseXor)}}

		case lexer.TCaretEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseXorAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TIn:
			if level >= ast.LCompare || !p.allowIn {
				return left
			}

			// Warn about "!a in b" instead of "!(a in b)"
			if e, ok := left.Data.(*ast.EUnary); ok && e.Op == ast.UnOpNot {
				p.log.AddWarning(p.source, left.Loc,
					"Suspicious use of the \"!\" operator inside the \"in\" operator")
			}

			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpIn, left, p.parseExpr(ast.LCompare)}}

		case lexer.TInstanceof:
			if level >= ast.LCompare {
				return left
			}

			// Warn about "!a instanceof b" instead of "!(a instanceof b)"
			if e, ok := left.Data.(*ast.EUnary); ok && e.Op == ast.UnOpNot {
				p.log.AddWarning(p.source, left.Loc,
					"Suspicious use of the \"!\" operator inside the \"instanceof\" operator")
			}

			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpInstanceof, left, p.parseExpr(ast.LCompare)}}

		default:
			// Handle the TypeScript "as" operator
			if p.ts.Parse && p.lexer.IsContextualKeyword("as") {
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
			p.lexer.Next()
		}
		arg := p.parseExpr(ast.LComma)
		if isSpread {
			arg = ast.Expr{loc, &ast.ESpread{arg}}
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
		return ast.Range{loc, 0}, "", nil
	}

	// The tag is an identifier
	name := p.lexer.Identifier
	tagRange := p.lexer.Range()
	p.lexer.ExpectInsideJSXElement(lexer.TIdentifier)

	// Certain identifiers are strings
	if strings.ContainsRune(name, '-') || (p.lexer.Token != lexer.TDot && name[0] >= 'a' && name[0] <= 'z') {
		return tagRange, name, &ast.Expr{loc, &ast.EString{lexer.StringToUTF16(name)}}
	}

	// Otherwise, this is an identifier
	tag := &ast.Expr{loc, &ast.EIdentifier{p.storeNameInRef(name)}}

	// Parse a member expression chain
	for p.lexer.Token == lexer.TDot {
		p.lexer.NextInsideJSXElement()
		memberRange := p.lexer.Range()
		member := p.lexer.Identifier
		p.lexer.ExpectInsideJSXElement(lexer.TIdentifier)

		// Dashes are not allowed in member expression chains
		index := strings.IndexByte(member, '-')
		if index >= 0 {
			p.addError(ast.Loc{memberRange.Loc.Start + int32(index)}, "Unexpected \"-\"")
			panic(lexer.LexerPanic{})
		}

		name += "." + member
		tag = &ast.Expr{loc, &ast.EDot{
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
	if p.ts.Parse {
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
				key := ast.Expr{keyRange.Loc, &ast.EString{lexer.StringToUTF16(p.lexer.Identifier)}}
				p.lexer.NextInsideJSXElement()

				// Parse the value
				var value ast.Expr
				if p.lexer.Token != lexer.TEquals {
					// Implicitly true value
					value = ast.Expr{ast.Loc{keyRange.Loc.Start + keyRange.Len}, &ast.EBoolean{true}}
				} else {
					// Use NextInsideJSXElement() not Next() so we can parse a JSX-style string literal
					p.lexer.NextInsideJSXElement()
					if p.lexer.Token == lexer.TStringLiteral {
						value = ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
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
		return ast.Expr{loc, &ast.EJSXElement{startTag, properties, []ast.Expr{}}}
	}

	// Use ExpectJSXElementChild() so we parse child strings
	p.lexer.ExpectJSXElementChild(lexer.TGreaterThan)

	// Parse the children of this element
	children := []ast.Expr{}
	for {
		switch p.lexer.Token {
		case lexer.TStringLiteral:
			children = append(children, ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}})
			p.lexer.NextJSXElementChild()

		case lexer.TOpenBrace:
			// Use Next() instead of NextJSXElementChild() here since the next token is an expression
			p.lexer.Next()

			// The "..." here is ignored (it's used to signal an array type in TypeScript)
			if p.lexer.Token == lexer.TDotDotDot && p.ts.Parse {
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
				p.addRangeError(endRange, fmt.Sprintf("Expected closing tag %q to match opening tag %q", endText, startText))
			}
			if p.lexer.Token != lexer.TGreaterThan {
				p.lexer.Expected(lexer.TGreaterThan)
			}

			return ast.Expr{loc, &ast.EJSXElement{startTag, properties, children}}

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
		p.lexer.RescanCloseBraceAsTemplateToken()
		tail := p.lexer.StringLiteral
		tailRaw := ""
		if includeRaw {
			tailRaw = p.lexer.RawTemplateContents()
		}
		parts = append(parts, ast.TemplatePart{value, tail, tailRaw})
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
		if p.ts.Parse {
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

		decls = append(decls, ast.Decl{local, value})

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
				p.addError(d.Binding.Loc, "This constant must be initialized")
			}
		}
	}
}

func (p *parser) forbidInitializers(decls []ast.Decl, loopType string, isVar bool) {
	if len(decls) > 1 {
		p.addError(decls[0].Binding.Loc, fmt.Sprintf("for-%s loops must have a single declaration", loopType))
	} else if len(decls) == 1 && decls[0].Value != nil {
		if isVar {
			if _, ok := decls[0].Binding.Data.(*ast.BIdentifier); ok {
				// This is a weird special case. Initializers are allowed in "var"
				// statements with identifier bindings.
				return
			}
		}
		p.addError(decls[0].Value.Loc, fmt.Sprintf("for-%s loop variables cannot have an initializer", loopType))
	}
}

func (p *parser) parseImportClause() []ast.ClauseItem {
	items := []ast.ClauseItem{}
	p.lexer.Expect(lexer.TOpenBrace)

	for p.lexer.Token != lexer.TCloseBrace {
		alias := p.lexer.Identifier
		aliasLoc := p.lexer.Loc()
		name := ast.LocRef{aliasLoc, p.storeNameInRef(alias)}

		// The alias may be a keyword
		isIdentifier := p.lexer.Token == lexer.TIdentifier
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		if p.lexer.IsContextualKeyword("as") {
			p.lexer.Next()
			name = ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
			p.lexer.Expect(lexer.TIdentifier)
		} else if !isIdentifier {
			// An import where the name is a keyword must have an alias
			p.lexer.Unexpected()
		}

		items = append(items, ast.ClauseItem{alias, aliasLoc, name})

		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseBrace)
	return items
}

func (p *parser) parseExportClause() []ast.ClauseItem {
	items := []ast.ClauseItem{}
	firstKeywordItemLoc := ast.Loc{}
	p.lexer.Expect(lexer.TOpenBrace)

	for p.lexer.Token != lexer.TCloseBrace {
		alias := p.lexer.Identifier
		aliasLoc := p.lexer.Loc()
		name := ast.LocRef{aliasLoc, p.storeNameInRef(alias)}

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

		items = append(items, ast.ClauseItem{alias, aliasLoc, name})

		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseBrace)

	// Throw an error here if we found a keyword earlier and this isn't an
	// "export from" statement after all
	if firstKeywordItemLoc.Start != 0 && !p.lexer.IsContextualKeyword("from") {
		r := lexer.RangeOfIdentifier(p.source, firstKeywordItemLoc)
		p.addRangeError(r, fmt.Sprintf("Expected identifier but found %q", p.source.TextForRange(r)))
		panic(lexer.LexerPanic{})
	}

	return items
}

func (p *parser) parseBinding() ast.Binding {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TIdentifier:
		ref := p.storeNameInRef(p.lexer.Identifier)
		p.lexer.Next()
		return ast.Binding{loc, &ast.BIdentifier{ref}}

	case lexer.TOpenBracket:
		p.lexer.Next()
		items := []ast.ArrayBinding{}
		hasSpread := false

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBracket {
			if p.lexer.Token == lexer.TComma {
				binding := ast.Binding{p.lexer.Loc(), &ast.BMissing{}}
				items = append(items, ast.ArrayBinding{binding, nil})
			} else {
				if p.lexer.Token == lexer.TDotDotDot {
					p.lexer.Next()
					hasSpread = true

					// This was a bug in the ES2015 spec that was fixed in ES2016
					if p.lexer.Token != lexer.TIdentifier {
						p.markFutureSyntax(futureSyntaxNonIdentifierArrayRest, p.lexer.Range())
					}
				}

				binding := p.parseBinding()

				var defaultValue *ast.Expr
				if !hasSpread && p.lexer.Token == lexer.TEquals {
					p.lexer.Next()
					value := p.parseExpr(ast.LComma)
					defaultValue = &value
				}

				items = append(items, ast.ArrayBinding{binding, defaultValue})

				// Commas after spread elements are not allowed
				if hasSpread && p.lexer.Token == lexer.TComma {
					p.addRangeError(p.lexer.Range(), "Unexpected \",\" after rest pattern")
					panic(lexer.LexerPanic{})
				}
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.allowIn = oldAllowIn

		p.lexer.Expect(lexer.TCloseBracket)
		return ast.Binding{loc, &ast.BArray{items, hasSpread}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		properties := []ast.PropertyBinding{}

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBrace {
			property := p.parsePropertyBinding()
			properties = append(properties, property)

			// Commas after spread elements are not allowed
			if property.IsSpread && p.lexer.Token == lexer.TComma {
				p.addRangeError(p.lexer.Range(), "Unexpected \",\" after rest pattern")
				panic(lexer.LexerPanic{})
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.allowIn = oldAllowIn

		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Binding{loc, &ast.BObject{properties}}
	}

	p.lexer.Expect(lexer.TIdentifier)
	return ast.Binding{}
}

func (p *parser) parseFn(name *ast.LocRef, opts fnOpts) (fn ast.Fn, hadBody bool) {
	if opts.allowAwait {
		if opts.allowYield {
			p.markFutureSyntax(futureSyntaxAsyncGenerator, opts.asyncRange)
		} else {
			p.markFutureSyntax(futureSyntaxAsync, opts.asyncRange)
		}
	}

	args := []ast.Arg{}
	hasRestArg := false
	p.lexer.Expect(lexer.TOpenParen)

	// Reserve the special name "arguments" in this scope. This ensures that it
	// shadows any variable called "arguments" in any parent scopes.
	argumentsRef := p.declareSymbol(ast.SymbolHoisted, ast.Loc{}, "arguments")
	p.symbols[argumentsRef.InnerIndex].MustNotBeRenamed = true

	for p.lexer.Token != lexer.TCloseParen {
		// Skip over "this" type annotations
		if p.ts.Parse && p.lexer.Token == lexer.TThis {
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

		// Skip past decorators and recover even though they aren't supported
		if p.ts.Parse {
			p.parseAndSkipDecorators()
		}

		if !hasRestArg && p.lexer.Token == lexer.TDotDotDot {
			p.lexer.Next()
			hasRestArg = true
		}

		// Potentially parse a TypeScript accessibility modifier
		isTypeScriptField := false
		if p.ts.Parse {
			switch p.lexer.Token {
			case lexer.TPrivate, lexer.TProtected, lexer.TPublic:
				isTypeScriptField = true
				p.lexer.Next()

				// TypeScript requires an identifier binding
				if p.lexer.Token != lexer.TIdentifier {
					p.lexer.Expect(lexer.TIdentifier)
				}
			}
		}

		isIdentifier := p.lexer.Token == lexer.TIdentifier
		identifierText := p.lexer.Identifier
		arg := p.parseBinding()

		if p.ts.Parse {
			// Skip over "readonly"
			isBeforeBinding := p.lexer.Token == lexer.TIdentifier || p.lexer.Token == lexer.TOpenBrace || p.lexer.Token == lexer.TOpenBracket
			if isBeforeBinding && isIdentifier && identifierText == "readonly" {
				isTypeScriptField = true

				// TypeScript requires an identifier binding
				if p.lexer.Token != lexer.TIdentifier {
					p.lexer.Expect(lexer.TIdentifier)
				}

				// Re-parse the binding (the current binding is the "readonly" keyword)
				arg = p.parseBinding()
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
		if !hasRestArg && p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			value := p.parseExpr(ast.LComma)
			defaultValue = &value
		}

		args = append(args, ast.Arg{
			Binding: arg,
			Default: defaultValue,

			// We need to track this because it affects code generation
			IsTypeScriptCtorField: isTypeScriptField,
		})

		if p.lexer.Token != lexer.TComma {
			break
		}
		if hasRestArg {
			p.lexer.Expect(lexer.TCloseParen)
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)

	fn = ast.Fn{
		Name:        name,
		Args:        args,
		HasRestArg:  hasRestArg,
		IsAsync:     opts.allowAwait,
		IsGenerator: opts.allowYield,
	}

	// "function foo(): any {}"
	if p.ts.Parse && p.lexer.Token == lexer.TColon {
		p.lexer.Next()
		p.skipTypeScriptReturnType()
	}

	// "function foo(): any;"
	if opts.allowMissingBodyForTypeScript && p.lexer.Token != lexer.TOpenBrace {
		return
	}

	fn.Body = p.parseFnBody(opts)
	hadBody = true
	return
}

func (p *parser) parseClassStmt(loc ast.Loc, opts parseStmtOpts) ast.Stmt {
	var name *ast.LocRef
	p.lexer.Expect(lexer.TClass)

	if !opts.isNameOptional || p.lexer.Token == lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		nameText := p.lexer.Identifier
		p.lexer.Expect(lexer.TIdentifier)
		name = &ast.LocRef{nameLoc, ast.InvalidRef}
		if !opts.isTypeScriptDeclare {
			name.Ref = p.declareSymbol(ast.SymbolClass, nameLoc, nameText)
			if opts.isExport {
				p.recordExport(nameLoc, nameText, name.Ref)
			}
		}
	}

	// Even anonymous classes can have TypeScript type parameters
	if p.ts.Parse {
		p.skipTypeScriptTypeParameters()
	}

	class := p.parseClass(name, parseClassOpts{isTypeScriptDeclare: opts.isTypeScriptDeclare})
	return ast.Stmt{loc, &ast.SClass{class, opts.isExport}}
}

func (p *parser) parseAndSkipDecorators() {
	for p.lexer.Token == lexer.TAt {
		r := p.lexer.Range()
		p.lexer.Next()

		// Eat an identifier
		r.Len = p.lexer.Range().End() - r.Loc.Start
		p.lexer.Expect(lexer.TIdentifier)

		// Eat an property access chain
		for p.lexer.Token == lexer.TDot {
			p.lexer.Next()
			r.Len = p.lexer.Range().End() - r.Loc.Start
			p.lexer.Expect(lexer.TIdentifier)
		}

		// Eat call expression arguments
		if p.lexer.Token == lexer.TOpenParen {
			p.lexer.Next()
			depth := 0

			// Skip to the matching close parenthesis. This doesn't have to be super
			// accurate because we're in error recovery mode.
			for p.lexer.Token != lexer.TEndOfFile && (p.lexer.Token != lexer.TCloseParen || depth > 0) {
				switch p.lexer.Token {
				case lexer.TOpenParen:
					depth++
				case lexer.TCloseParen:
					depth--
				}
				p.lexer.Next()
			}

			r.Len = p.lexer.Range().End() - r.Loc.Start
			p.lexer.Expect(lexer.TCloseParen)
		}

		p.addRangeError(r, "Decorators are not supported yet")
	}
}

type parseClassOpts struct {
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
		if p.ts.Parse {
			p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
		}
	}

	if p.ts.Parse && p.lexer.Token == lexer.TImplements {
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
	opts := propertyOpts{
		classHasExtends: extends != nil,
	}

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

		// Skip past decorators and recover even though they aren't supported
		if p.ts.Parse {
			p.parseAndSkipDecorators()
		}

		// This property may turn out to be a type in TypeScript, which should be ignored
		if property, ok := p.parseProperty(propertyContextClass, ast.PropertyNormal, opts, nil); ok {
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
	return ast.Class{name, extends, bodyLoc, properties}
}

func (p *parser) parseLabelName() *ast.LocRef {
	if p.lexer.Token != lexer.TIdentifier || p.lexer.HasNewlineBefore {
		return nil
	}

	name := ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
	p.lexer.Next()
	return &name
}

func (p *parser) parsePath() ast.Path {
	path := ast.Path{
		Loc:  p.lexer.Loc(),
		Text: lexer.UTF16ToString(p.lexer.StringLiteral),
	}
	if p.lexer.Token == lexer.TNoSubstitutionTemplateLiteral {
		p.lexer.Next()
	} else {
		p.lexer.Expect(lexer.TStringLiteral)
	}
	return path
}

// This assumes the "function" token has already been parsed
func (p *parser) parseFnStmt(loc ast.Loc, opts parseStmtOpts, isAsync bool, asyncRange ast.Range) ast.Stmt {
	isGenerator := p.lexer.Token == lexer.TAsterisk
	if !opts.allowLexicalDecl && (isGenerator || isAsync) {
		p.forbidLexicalDecl(loc)
	}
	if isGenerator {
		p.lexer.Next()
	}

	var name *ast.LocRef
	var nameText string

	// The name is optional for "export default function() {}" pseudo-statements
	if !opts.isNameOptional || p.lexer.Token == lexer.TIdentifier {
		nameLoc := p.lexer.Loc()
		nameText = p.lexer.Identifier
		p.lexer.Expect(lexer.TIdentifier)
		name = &ast.LocRef{nameLoc, ast.InvalidRef}
	}

	// Even anonymous functions can have TypeScript type parameters
	if p.ts.Parse {
		p.skipTypeScriptTypeParameters()
	}

	scopeIndex := p.pushScopeForParsePass(ast.ScopeFunctionArgs, loc)

	fn, hadBody := p.parseFn(name, fnOpts{
		asyncRange: asyncRange,
		allowAwait: isAsync,
		allowYield: isGenerator,

		// Only allow omitting the body if we're parsing TypeScript
		allowMissingBodyForTypeScript: p.ts.Parse,
	})

	// Don't output anything if it's just a forward declaration of a function
	if !hadBody {
		p.popAndDiscardScope(scopeIndex)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.STypeScript{}}
	}

	p.popScope()

	// Only declare the function after we know if it had a body or not. Otherwise
	// TypeScript code such as this will double-declare the symbol:
	//
	//     function foo(): void;
	//     function foo(): void {}
	//
	if !opts.isTypeScriptDeclare && name != nil {
		name.Ref = p.declareSymbol(ast.SymbolHoistedFunction, name.Loc, nameText)
		if opts.isExport {
			p.recordExport(name.Loc, nameText, name.Ref)
		}
	}

	return ast.Stmt{loc, &ast.SFunction{fn, opts.isExport}}
}

type parseStmtOpts struct {
	allowLexicalDecl    bool
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
		return ast.Stmt{loc, &ast.SEmpty{}}

	case lexer.TExport:
		if opts.isModuleScope {
			p.hasES6ExportSyntax = true
		} else if !opts.isNamespaceScope {
			p.lexer.Unexpected()
		}
		p.lexer.Next()

		switch p.lexer.Token {
		case lexer.TClass, lexer.TConst, lexer.TFunction, lexer.TLet, lexer.TVar:
			opts.isExport = true
			return p.parseStmt(opts)

		case lexer.TImport:
			// "export import foo = bar"
			if p.ts.Parse && (opts.isModuleScope || opts.isNamespaceScope) {
				opts.isExport = true
				return p.parseStmt(opts)
			}

			p.lexer.Unexpected()
			return ast.Stmt{}

		case lexer.TEnum:
			if !p.ts.Parse {
				p.lexer.Unexpected()
			}
			opts.isExport = true
			return p.parseStmt(opts)

		case lexer.TInterface:
			if p.ts.Parse {
				opts.isExport = true
				return p.parseStmt(opts)
			}
			p.lexer.Unexpected()
			return ast.Stmt{}

		case lexer.TIdentifier:
			if p.lexer.IsContextualKeyword("async") {
				// "export async function foo() {}"
				asyncRange := p.lexer.Range()
				p.lexer.Next()
				p.lexer.Expect(lexer.TFunction)
				opts.isExport = true
				return p.parseFnStmt(loc, opts, true /* isAsync */, asyncRange)
			}

			if p.ts.Parse {
				switch p.lexer.Identifier {
				case "type":
					// "export type foo = ..."
					p.lexer.Next()
					p.skipTypeScriptTypeStmt(parseStmtOpts{isExport: true})
					return ast.Stmt{loc, &ast.STypeScript{}}

				case "namespace", "abstract", "module":
					// "export namespace Foo {}"
					// "export abstract class Foo {}"
					// "export module Foo {}"
					opts.isExport = true
					return p.parseStmt(opts)

				case "declare":
					// "export declare class Foo {}"
					opts.isExport = true
					opts.allowLexicalDecl = true
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

			defaultIdentifier := ast.GenerateNonUniqueNameFromPath(p.source.AbsolutePath) + "_default"
			defaultName := ast.LocRef{p.lexer.Loc(), p.newSymbol(ast.SymbolOther, defaultIdentifier)}
			p.currentScope.Generated = append(p.currentScope.Generated, defaultName.Ref)
			p.lexer.Next()

			if p.lexer.IsContextualKeyword("async") {
				asyncRange := p.lexer.Range()
				p.lexer.Next()

				if p.lexer.Token == lexer.TFunction {
					p.lexer.Expect(lexer.TFunction)
					stmt := p.parseFnStmt(loc, parseStmtOpts{
						isNameOptional:   true,
						allowLexicalDecl: true,
					}, true /* isAsync */, asyncRange)
					if _, ok := stmt.Data.(*ast.STypeScript); ok {
						return stmt // This was just a type annotation
					}

					// Use the statement name if present, since it's a better name
					if s, ok := stmt.Data.(*ast.SFunction); ok && s.Fn.Name != nil {
						defaultName.Ref = s.Fn.Name.Ref
					}

					p.recordExport(defaultName.Loc, "default", defaultName.Ref)
					return ast.Stmt{loc, &ast.SExportDefault{defaultName, ast.ExprOrStmt{Stmt: &stmt}}}
				}

				p.recordExport(defaultName.Loc, "default", defaultName.Ref)
				expr := p.parseSuffix(p.parseAsyncPrefixExpr(asyncRange), ast.LComma, nil)
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{loc, &ast.SExportDefault{defaultName, ast.ExprOrStmt{Expr: &expr}}}
			}

			if p.lexer.Token == lexer.TFunction || p.lexer.Token == lexer.TClass || p.lexer.Token == lexer.TInterface {
				stmt := p.parseStmt(parseStmtOpts{
					isNameOptional:   true,
					allowLexicalDecl: true,
				})
				if _, ok := stmt.Data.(*ast.STypeScript); ok {
					return stmt // This was just a type annotation
				}

				// Use the statement name if present, since it's a better name
				switch s := stmt.Data.(type) {
				case *ast.SFunction:
					if s.Fn.Name != nil {
						defaultName.Ref = s.Fn.Name.Ref
					}
				case *ast.SClass:
					if s.Class.Name != nil {
						defaultName.Ref = s.Class.Name.Ref
					}
				}

				p.recordExport(defaultName.Loc, "default", defaultName.Ref)
				return ast.Stmt{loc, &ast.SExportDefault{defaultName, ast.ExprOrStmt{Stmt: &stmt}}}
			}

			isIdentifier := p.lexer.Token == lexer.TIdentifier
			name := p.lexer.Identifier
			expr := p.parseExpr(ast.LComma)

			// Handle the default export of an abstract class in TypeScript
			if p.ts.Parse && isIdentifier && name == "abstract" && p.lexer.Token == lexer.TClass {
				if _, ok := expr.Data.(*ast.EIdentifier); ok {
					stmt := p.parseClassStmt(loc, parseStmtOpts{isNameOptional: true})

					// Use the statement name if present, since it's a better name
					if s, ok := stmt.Data.(*ast.SClass); ok && s.Class.Name != nil {
						defaultName.Ref = s.Class.Name.Ref
					}

					p.recordExport(defaultName.Loc, "default", defaultName.Ref)
					return ast.Stmt{loc, &ast.SExportDefault{defaultName, ast.ExprOrStmt{Stmt: &stmt}}}
				}
			}

			p.lexer.ExpectOrInsertSemicolon()
			p.recordExport(defaultName.Loc, "default", defaultName.Ref)
			return ast.Stmt{loc, &ast.SExportDefault{defaultName, ast.ExprOrStmt{Expr: &expr}}}

		case lexer.TAsterisk:
			if !opts.isModuleScope {
				p.lexer.Unexpected()
			}

			p.lexer.Next()
			var item *ast.ClauseItem
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				item = &ast.ClauseItem{name, nameLoc, ast.LocRef{nameLoc, p.storeNameInRef(name)}}
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.ExpectContextualKeyword("from")
			path := p.parsePath()
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{loc, &ast.SExportStar{item, path}}

		case lexer.TOpenBrace:
			if !opts.isModuleScope {
				p.lexer.Unexpected()
			}

			items := p.parseExportClause()
			if p.lexer.IsContextualKeyword("from") {
				p.lexer.Next()
				path := p.parsePath()
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{loc, &ast.SExportFrom{items, ast.InvalidRef, path}}
			}
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{loc, &ast.SExportClause{items}}

		case lexer.TEquals:
			// "export = value;"
			if p.ts.Parse {
				p.lexer.Next()
				value := p.parseExpr(ast.LLowest)
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{loc, &ast.SExportEquals{value}}
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
		if !p.ts.Parse {
			p.lexer.Unexpected()
		}
		return p.parseEnumStmt(loc, opts)

	case lexer.TInterface:
		if !p.ts.Parse {
			p.lexer.Unexpected()
		}

		p.lexer.Next()
		p.lexer.Expect(lexer.TIdentifier)
		p.skipTypeScriptTypeParameters()

		if p.lexer.Token == lexer.TExtends {
			p.lexer.Next()
			for {
				p.skipTypeScriptType(ast.LLowest)
				if p.lexer.Token != lexer.TComma {
					break
				}
				p.lexer.Next()
			}
		}

		if p.lexer.Token == lexer.TImplements {
			p.lexer.Next()
			for {
				p.skipTypeScriptType(ast.LLowest)
				if p.lexer.Token != lexer.TComma {
					break
				}
				p.lexer.Next()
			}
		}

		p.skipTypeScriptObjectType()
		return ast.Stmt{loc, &ast.STypeScript{}}

	case lexer.TAt:
		// Skip past decorators and recover even though they aren't supported
		if p.ts.Parse {
			p.parseAndSkipDecorators()
			if p.lexer.Token == lexer.TExport {
				p.lexer.Next()
			}
			if p.lexer.Token != lexer.TClass {
				p.lexer.Expected(lexer.TClass)
			}
			return p.parseStmt(opts)
		}

		p.lexer.Unexpected()
		return ast.Stmt{}

	case lexer.TClass:
		if !opts.allowLexicalDecl {
			p.forbidLexicalDecl(loc)
		}
		return p.parseClassStmt(loc, opts)

	case lexer.TVar:
		p.lexer.Next()
		decls := p.parseAndDeclareDecls(ast.SymbolHoisted, opts)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SLocal{
			Kind:     ast.LocalVar,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case lexer.TLet:
		if !opts.allowLexicalDecl {
			p.forbidLexicalDecl(loc)
		}
		p.lexer.Next()
		decls := p.parseAndDeclareDecls(ast.SymbolOther, opts)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SLocal{
			Kind:     ast.LocalLet,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case lexer.TConst:
		if !opts.allowLexicalDecl {
			p.forbidLexicalDecl(loc)
		}
		p.lexer.Next()

		if p.ts.Parse && p.lexer.Token == lexer.TEnum {
			return p.parseEnumStmt(loc, opts)
		}

		decls := p.parseAndDeclareDecls(ast.SymbolOther, opts)
		p.lexer.ExpectOrInsertSemicolon()
		if !opts.isTypeScriptDeclare {
			p.requireInitializers(decls)
		}
		return ast.Stmt{loc, &ast.SLocal{
			Kind:     ast.LocalConst,
			Decls:    decls,
			IsExport: opts.isExport,
		}}

	case lexer.TIf:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		yes := p.parseStmt(parseStmtOpts{})
		var no *ast.Stmt = nil
		if p.lexer.Token == lexer.TElse {
			p.lexer.Next()
			stmt := p.parseStmt(parseStmtOpts{})
			no = &stmt
		}
		return ast.Stmt{loc, &ast.SIf{test, yes, no}}

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
		return ast.Stmt{loc, &ast.SDoWhile{body, test}}

	case lexer.TWhile:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return ast.Stmt{loc, &ast.SWhile{test, body}}

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

		return ast.Stmt{loc, &ast.SWith{test, bodyLoc, body}}

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
					p.log.AddRangeError(p.source, p.lexer.Range(), "Multiple default clauses are not allowed")
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
					body = append(body, p.parseStmt(parseStmtOpts{allowLexicalDecl: true}))
				}
			}

			cases = append(cases, ast.Case{value, body})
		}

		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Stmt{loc, &ast.SSwitch{
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
				if p.target < ES2019 {
					// Generate a new symbol for the catch binding for older browsers
					ref := p.newSymbol(ast.SymbolOther, "e")
					p.currentScope.Generated = append(p.currentScope.Generated, ref)
					binding = &ast.Binding{p.lexer.Loc(), &ast.BIdentifier{ref}}
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
			catch = &ast.Catch{catchLoc, binding, stmts}
			p.popScope()
		}

		if p.lexer.Token == lexer.TFinally || catch == nil {
			finallyLoc := p.lexer.Loc()
			p.pushScopeForParsePass(ast.ScopeBlock, finallyLoc)
			p.lexer.Expect(lexer.TFinally)
			p.lexer.Expect(lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
			p.lexer.Next()
			finally = &ast.Finally{finallyLoc, stmts}
			p.popScope()
		}

		return ast.Stmt{loc, &ast.STry{body, catch, finally}}

	case lexer.TFor:
		p.pushScopeForParsePass(ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()

		// "for await (let x of y) {}"
		isAwait := p.lexer.IsContextualKeyword("await")
		if isAwait {
			if !p.currentFnOpts.allowAwait {
				p.addRangeError(p.lexer.Range(), "Cannot use \"await\" outside an async function")
				isAwait = false
			} else {
				p.markFutureSyntax(futureSyntaxForAwait, p.lexer.Range())
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
			init = &ast.Stmt{initLoc, &ast.SLocal{Kind: ast.LocalVar, Decls: decls}}

		case lexer.TLet:
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolOther, parseStmtOpts{})
			init = &ast.Stmt{initLoc, &ast.SLocal{Kind: ast.LocalLet, Decls: decls}}

		case lexer.TConst:
			p.lexer.Next()
			decls = p.parseAndDeclareDecls(ast.SymbolOther, parseStmtOpts{})
			init = &ast.Stmt{initLoc, &ast.SLocal{Kind: ast.LocalConst, Decls: decls}}

		case lexer.TSemicolon:

		default:
			init = &ast.Stmt{initLoc, &ast.SExpr{p.parseExpr(ast.LLowest)}}
		}

		// "in" expressions are allowed again
		p.allowIn = true

		// Detect for-of loops
		if p.lexer.IsContextualKeyword("of") || isAwait {
			if isAwait && !p.lexer.IsContextualKeyword("of") {
				if init != nil {
					p.lexer.ExpectedString("\"of\"")
				} else {
					p.lexer.Unexpected()
				}
			}
			p.forbidInitializers(decls, "of", false)
			p.lexer.Next()
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{loc, &ast.SForOf{isAwait, *init, value, body}}
		}

		// Detect for-in loops
		if p.lexer.Token == lexer.TIn {
			p.forbidInitializers(decls, "in", isVar)
			p.lexer.Next()
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{loc, &ast.SForIn{*init, value, body}}
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
		return ast.Stmt{loc, &ast.SFor{init, test, update, body}}

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
			expr := p.parseSuffix(p.parseImportExpr(loc), ast.LLowest, nil)
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{loc, &ast.SExpr{expr}}

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

			items := p.parseImportClause()
			stmt.Items = &items
			p.lexer.ExpectContextualKeyword("from")

		case lexer.TIdentifier:
			// "import defaultItem from 'path'"
			// "import foo = bar"
			if !opts.isModuleScope && !opts.isNamespaceScope {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			defaultName := p.lexer.Identifier
			stmt.DefaultName = &ast.LocRef{p.lexer.Loc(), p.storeNameInRef(defaultName)}
			p.lexer.Next()

			if p.ts.Parse {
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
							return ast.Stmt{loc, &ast.STypeScript{}}
						}

					case lexer.TAsterisk:
						// "import type * as foo from 'bar';"
						p.lexer.Next()
						p.lexer.ExpectContextualKeyword("as")
						p.lexer.Expect(lexer.TIdentifier)
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return ast.Stmt{loc, &ast.STypeScript{}}

					case lexer.TOpenBrace:
						// "import type {foo} from 'bar';"
						p.parseImportClause()
						p.lexer.ExpectContextualKeyword("from")
						p.parsePath()
						p.lexer.ExpectOrInsertSemicolon()
						return ast.Stmt{loc, &ast.STypeScript{}}
					}
				}

				// Parse TypeScript import assignment statements
				if p.lexer.Token == lexer.TEquals || opts.isExport || opts.isNamespaceScope {
					p.lexer.Expect(lexer.TEquals)
					value := p.parseExpr(ast.LComma)
					p.lexer.ExpectOrInsertSemicolon()
					ref := p.declareSymbol(ast.SymbolOther, stmt.DefaultName.Loc, defaultName)
					decls := []ast.Decl{ast.Decl{
						ast.Binding{stmt.DefaultName.Loc, &ast.BIdentifier{ref}},
						&value,
					}}
					if opts.isExport {
						p.recordExport(stmt.DefaultName.Loc, defaultName, ref)
					}

					// The kind of statement depends on the expression
					if _, ok := value.Data.(*ast.ECall); ok {
						// "import ns = require('x')"
						return ast.Stmt{loc, &ast.SLocal{
							Kind:                         ast.LocalConst,
							Decls:                        decls,
							IsExport:                     opts.isExport,
							WasTSImportEqualsInNamespace: opts.isNamespaceScope,
						}}
					} else {
						// "import Foo = Bar"
						return ast.Stmt{loc, &ast.SLocal{
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
					items := p.parseImportClause()
					stmt.Items = &items

				default:
					p.lexer.Unexpected()
				}
			}

			p.lexer.ExpectContextualKeyword("from")

		default:
			p.lexer.Unexpected()
			return ast.Stmt{}
		}

		stmt.Path = p.parsePath()
		p.lexer.ExpectOrInsertSemicolon()
		kind := ast.SymbolOther

		// In TypeScript, imports are allowed to silently collide with symbols within
		// the module. Presumably this is because the imports may be type-only.
		if p.ts.Parse {
			kind = ast.SymbolTSImport
		}

		if stmt.StarNameLoc != nil {
			name := p.loadNameFromRef(stmt.NamespaceRef)
			stmt.NamespaceRef = p.declareSymbol(kind, *stmt.StarNameLoc, name)
		} else {
			// Generate a symbol for the namespace
			name := ast.GenerateNonUniqueNameFromPath(stmt.Path.Text)
			stmt.NamespaceRef = p.newSymbol(ast.SymbolOther, name)
			p.currentScope.Generated = append(p.currentScope.Generated, stmt.NamespaceRef)
		}
		itemRefs := make(map[string]ast.LocRef)

		// Link the default item to the namespace
		if stmt.DefaultName != nil {
			name := p.loadNameFromRef(stmt.DefaultName.Ref)
			ref := p.declareSymbol(kind, stmt.DefaultName.Loc, name)
			p.isImportItem[ref] = true
			stmt.DefaultName.Ref = ref
			itemRefs["default"] = *stmt.DefaultName
		}

		// Link each import item to the namespace
		if stmt.Items != nil {
			for i, item := range *stmt.Items {
				name := p.loadNameFromRef(item.Name.Ref)
				ref := p.declareSymbol(kind, item.Name.Loc, name)
				p.isImportItem[ref] = true
				(*stmt.Items)[i].Name.Ref = ref
				itemRefs[item.Alias] = ast.LocRef{item.Name.Loc, ref}
			}
		}

		// Track the items for this namespace
		p.importItemsForNamespace[stmt.NamespaceRef] = itemRefs

		return ast.Stmt{loc, &stmt}

	case lexer.TBreak:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SBreak{name}}

	case lexer.TContinue:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SContinue{name}}

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
		if p.currentFnOpts.isOutsideFn {
			p.hasTopLevelReturn = true
		}
		return ast.Stmt{loc, &ast.SReturn{value}}

	case lexer.TThrow:
		p.lexer.Next()
		if p.lexer.HasNewlineBefore {
			p.addError(ast.Loc{loc.Start + 5}, "Unexpected newline after \"throw\"")
			panic(lexer.LexerPanic{})
		}
		expr := p.parseExpr(ast.LLowest)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SThrow{expr}}

	case lexer.TDebugger:
		p.lexer.Next()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SDebugger{}}

	case lexer.TOpenBrace:
		p.pushScopeForParsePass(ast.ScopeBlock, loc)
		defer p.popScope()

		p.lexer.Next()
		stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
		p.lexer.Next()
		return ast.Stmt{loc, &ast.SBlock{stmts}}

	default:
		isIdentifier := p.lexer.Token == lexer.TIdentifier
		name := p.lexer.Identifier

		// Parse either an async function, an async expression, or a normal expression
		var expr ast.Expr
		if isIdentifier && name == "async" {
			asyncRange := p.lexer.Range()
			p.lexer.Next()
			if p.lexer.Token == lexer.TFunction {
				p.lexer.Next()
				return p.parseFnStmt(asyncRange.Loc, opts, true /* isAsync */, asyncRange)
			}
			expr = p.parseSuffix(p.parseAsyncPrefixExpr(asyncRange), ast.LLowest, nil)
		} else {
			expr = p.parseExpr(ast.LLowest)
		}

		if isIdentifier {
			if ident, ok := expr.Data.(*ast.EIdentifier); ok {
				if p.lexer.Token == lexer.TColon {
					p.pushScopeForParsePass(ast.ScopeLabel, loc)
					defer p.popScope()

					// Parse a labeled statement
					p.lexer.Next()
					name := ast.LocRef{expr.Loc, ident.Ref}
					stmt := p.parseStmt(parseStmtOpts{})
					return ast.Stmt{loc, &ast.SLabel{name, stmt}}
				}

				if p.ts.Parse {
					switch name {
					case "type":
						if p.lexer.Token == lexer.TIdentifier {
							// "type Foo = any"
							p.skipTypeScriptTypeStmt(parseStmtOpts{})
							return ast.Stmt{loc, &ast.STypeScript{}}
						}

					case "namespace", "module":
						// "namespace Foo {}"
						// "module Foo {}"
						// "declare module 'fs' {}"
						// "declare module 'fs';"
						if (opts.isModuleScope || opts.isNamespaceScope) && (p.lexer.Token == lexer.TIdentifier ||
							(p.lexer.Token == lexer.TStringLiteral && opts.isTypeScriptDeclare)) {
							return p.parseNamespaceStmt(loc, opts)
						}

					case "abstract":
						if p.lexer.Token == lexer.TClass {
							return p.parseClassStmt(loc, opts)
						}

					case "declare":
						opts.allowLexicalDecl = true
						opts.isTypeScriptDeclare = true

						// "declare global { ... }"
						if p.lexer.IsContextualKeyword("global") {
							p.lexer.Next()
							p.lexer.Expect(lexer.TOpenBrace)
							p.parseStmtsUpTo(lexer.TCloseBrace, opts)
							p.lexer.Next()
							return ast.Stmt{loc, &ast.STypeScript{}}
						}

						// "declare const x: any"
						p.parseStmt(opts)
						return ast.Stmt{loc, &ast.STypeScript{}}
					}
				}
			}
		}

		p.lexer.ExpectOrInsertSemicolon()

		// Parse a "use strict" directive
		if str, ok := expr.Data.(*ast.EString); ok && len(str.Value) == len("use strict") {
			isEqual := true
			for i, c := range str.Value {
				if c != uint16("use strict"[i]) {
					isEqual = false
					break
				}
			}
			if isEqual {
				return ast.Stmt{loc, &ast.SDirective{Value: str.Value}}
			}
		}

		return ast.Stmt{loc, &ast.SExpr{expr}}
	}
}

func (p *parser) parseNamespaceStmt(loc ast.Loc, opts parseStmtOpts) ast.Stmt {
	// "namespace Foo {}"
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Next()

	name := ast.LocRef{nameLoc, ast.InvalidRef}
	argRef := ast.InvalidRef

	scopeIndex := p.pushScopeForParsePass(ast.ScopeEntry, loc)
	oldEnclosingNamespaceRef := p.enclosingNamespaceRef
	p.enclosingNamespaceRef = &name.Ref

	if !opts.isTypeScriptDeclare {
		argRef = p.declareSymbol(ast.SymbolHoistedFunction, nameLoc, nameText)
	}

	var stmts []ast.Stmt
	if p.lexer.Token == lexer.TDot {
		dotLoc := p.lexer.Loc()
		p.lexer.Next()
		stmts = []ast.Stmt{p.parseNamespaceStmt(dotLoc, parseStmtOpts{
			isExport:            true,
			isNamespaceScope:    true,
			isTypeScriptDeclare: opts.isTypeScriptDeclare,
		})}
	} else if opts.isTypeScriptDeclare && p.lexer.Token != lexer.TOpenBrace {
		p.lexer.ExpectOrInsertSemicolon()
	} else {
		p.lexer.Expect(lexer.TOpenBrace)
		stmts = p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{
			isNamespaceScope:    true,
			isTypeScriptDeclare: opts.isTypeScriptDeclare,
		})
		p.lexer.Next()
	}

	p.enclosingNamespaceRef = oldEnclosingNamespaceRef

	// Import assignments may be only used in type expressions, not value
	// expressions. If this is the case, the TypeScript compiler removes
	// them entirely from the output. That can cause the namespace itself
	// to be considered empty and thus be removed.
	importEqualsCount := 0
	for _, stmt := range stmts {
		if local, ok := stmt.Data.(*ast.SLocal); ok && local.WasTSImportEqualsInNamespace && !local.IsExport {
			importEqualsCount++
		}
	}

	// TypeScript omits namespaces without values. These namespaces
	// are only allowed to be used in type expressions. They are
	// allowed to be exported, but can also only be used in type
	// expressions when imported. So we shouldn't count them as a
	// real export either.
	if len(stmts) == importEqualsCount || opts.isTypeScriptDeclare {
		p.popAndDiscardScope(scopeIndex)
		return ast.Stmt{loc, &ast.STypeScript{}}
	}

	p.popScope()
	if !opts.isTypeScriptDeclare {
		_, alreadyExists := p.currentScope.Members[nameText]
		name.Ref = p.declareSymbol(ast.SymbolTSNamespace, nameLoc, nameText)

		// It's valid to have multiple exported namespace statements as long as
		// each one has the "export" keyword. Make sure we don't record the same
		// export more than once, because then we will incorrectly detect duplicate
		// exports.
		if opts.isExport && !alreadyExists {
			p.recordExport(nameLoc, nameText, name.Ref)
		}
	}
	return ast.Stmt{loc, &ast.SNamespace{name, argRef, stmts, opts.isExport}}
}

func (p *parser) parseEnumStmt(loc ast.Loc, opts parseStmtOpts) ast.Stmt {
	p.lexer.Expect(lexer.TEnum)
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Expect(lexer.TIdentifier)
	name := ast.LocRef{nameLoc, ast.InvalidRef}
	argRef := ast.InvalidRef
	if !opts.isTypeScriptDeclare {
		name.Ref = p.declareSymbol(ast.SymbolTSEnum, nameLoc, nameText)
		p.pushScopeForParsePass(ast.ScopeEntry, loc)
		argRef = p.declareSymbol(ast.SymbolHoisted, nameLoc, nameText)
	}
	p.lexer.Expect(lexer.TOpenBrace)

	values := []ast.EnumValue{}

	for p.lexer.Token != lexer.TCloseBrace {
		value := ast.EnumValue{
			Loc: p.lexer.Loc(),
			Ref: ast.InvalidRef,
		}

		// Parse the name
		if p.lexer.Token == lexer.TStringLiteral {
			value.Name = p.lexer.StringLiteral
		} else if p.lexer.IsIdentifierOrKeyword() {
			value.Name = lexer.StringToUTF16(p.lexer.Identifier)
		} else {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		// Identifiers can be referenced by other values
		if !opts.isTypeScriptDeclare && lexer.IsIdentifierUTF16(value.Name) {
			value.Ref = p.declareSymbol(ast.SymbolOther, value.Loc, lexer.UTF16ToString(value.Name))
		}

		// Parse the initializer
		if p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			initializer := p.parseExpr(ast.LComma)
			value.Value = &initializer
		}

		values = append(values, value)

		if p.lexer.Token != lexer.TComma && p.lexer.Token != lexer.TSemicolon {
			break
		}
		p.lexer.Next()
	}

	if !opts.isTypeScriptDeclare {
		p.popScope()
		if opts.isExport {
			p.recordExport(nameLoc, nameText, name.Ref)
		}
	}

	p.lexer.Expect(lexer.TCloseBrace)
	return ast.Stmt{loc, &ast.SEnum{name, argRef, values, opts.isExport}}
}

func (p *parser) parseFnBody(opts fnOpts) ast.FnBody {
	oldFnOpts := p.currentFnOpts
	oldAllowIn := p.allowIn
	p.currentFnOpts = opts
	p.allowIn = true

	loc := p.lexer.Loc()
	p.pushScopeForParsePass(ast.ScopeFunctionBody, loc)
	defer p.popScope()

	p.lexer.Expect(lexer.TOpenBrace)
	stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
	p.lexer.Next()

	p.allowIn = oldAllowIn
	p.currentFnOpts = oldFnOpts
	return ast.FnBody{loc, stmts}
}

func (p *parser) forbidLexicalDecl(loc ast.Loc) {
	p.addError(loc, "Cannot use a declaration in a single-statement context")
}

func (p *parser) parseStmtsUpTo(end lexer.T, opts parseStmtOpts) []ast.Stmt {
	stmts := []ast.Stmt{}
	returnWithoutSemicolonStart := int32(-1)
	opts.allowLexicalDecl = true

	for p.lexer.Token != end {
		stmt := p.parseStmt(opts)

		// Skip TypeScript types entirely
		if p.ts.Parse {
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
					p.log.AddWarning(p.source, ast.Loc{returnWithoutSemicolonStart + 6},
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

func (p *parser) generateTempRef(declare generateTempRefArg) ast.Ref {
	scope := p.currentScope
	for !scope.Kind.StopsHoisting() {
		scope = scope.Parent
	}
	ref := p.newSymbol(ast.SymbolOther, "_"+lexer.NumberToMinifiedName(p.tempRefCount))
	p.tempRefCount++
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
			ref = member
			break
		}

		s = s.Parent
		if s == nil {
			// Allocate an "unbound" symbol
			ref = p.newSymbol(ast.SymbolUnbound, name)
			p.moduleScope.Members[name] = ref
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

func (p *parser) findLabelSymbol(loc ast.Loc, name string) ast.Ref {
	for s := p.currentScope; s != nil && !s.Kind.StopsHoisting(); s = s.Parent {
		if s.Kind == ast.ScopeLabel && name == p.symbols[s.LabelRef.InnerIndex].Name {
			// Track how many times we've referenced this symbol
			p.recordUsage(s.LabelRef)
			return s.LabelRef
		}
	}

	r := lexer.RangeOfIdentifier(p.source, loc)
	p.log.AddRangeError(p.source, r, fmt.Sprintf("There is no containing label named %q", name))

	// Allocate an "unbound" symbol
	ref := p.newSymbol(ast.SymbolUnbound, name)

	// Track how many times we've referenced this symbol
	p.recordUsage(ref)
	return ref
}

func findIdentifiers(binding ast.Binding, identifiers []ast.Decl) []ast.Decl {
	switch b := binding.Data.(type) {
	case *ast.BIdentifier:
		identifiers = append(identifiers, ast.Decl{Binding: binding})

	case *ast.BArray:
		for _, i := range b.Items {
			identifiers = findIdentifiers(i.Binding, identifiers)
		}

	case *ast.BObject:
		for _, i := range b.Properties {
			identifiers = findIdentifiers(i.Value, identifiers)
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
			decls = append(decls, ast.Decl{ast.Binding{ast.Loc{}, &ast.BIdentifier{ref}}, nil})
		}
		stmts = append([]ast.Stmt{ast.Stmt{ast.Loc{}, &ast.SLocal{Kind: ast.LocalVar, Decls: decls}}}, stmts...)
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
	if !p.mangleSyntax {
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
	for _, stmt := range visited {
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
				result = append(result, stmt)
				continue
			}

		case *ast.SReturn:
			// Merge return statements with the previous expression statement
			if len(result) > 0 && s.Value != nil {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					value := ast.JoinWithComma(prevS.Value, *s.Value)
					result[len(result)-1] = ast.Stmt{prevStmt.Loc, &ast.SReturn{&value}}
					continue
				}
			}

		case *ast.SThrow:
			// Merge throw statements with the previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					result[len(result)-1] = ast.Stmt{prevStmt.Loc, &ast.SThrow{ast.JoinWithComma(prevS.Value, s.Value)}}
					continue
				}
			}

		case *ast.SFor:
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					// Insert the previous expression into the for loop initializer
					if s.Init == nil {
						result[len(result)-1] = stmt
						s.Init = &ast.Stmt{prevStmt.Loc, &ast.SExpr{prevS.Value}}
						continue
					} else if s2, ok := s.Init.Data.(*ast.SExpr); ok {
						result[len(result)-1] = stmt
						s.Init = &ast.Stmt{prevStmt.Loc, &ast.SExpr{ast.JoinWithComma(prevS.Value, s2.Value)}}
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
					lastReturn = &ast.SReturn{&lastValue}

					// Merge the last two statements
					lastStmt = ast.Stmt{prevStmt.Loc, lastReturn}
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
						left = &ast.Expr{prevS.Yes.Loc, &ast.EUndefined{}}
					}
					if right == nil {
						// "if (a) return a; return;" => "return a ? b : void 0;"
						right = &ast.Expr{lastStmt.Loc, &ast.EUndefined{}}
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
						lastReturn = &ast.SReturn{&lastValue}
					} else {
						// "if (a) return b; return c;" => "return a ? b : c;"
						lastReturn = &ast.SReturn{&ast.Expr{prevS.Test.Loc, &ast.EIf{prevS.Test, *left, *right}}}
					}

					// Merge the last two statements
					lastStmt = ast.Stmt{prevStmt.Loc, lastReturn}
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
					lastThrow = &ast.SThrow{ast.JoinWithComma(prevS.Value, lastThrow.Value)}

					// Merge the last two statements
					lastStmt = ast.Stmt{prevStmt.Loc, lastThrow}
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
					lastThrow = &ast.SThrow{ast.Expr{prevS.Test.Loc, &ast.EIf{prevS.Test, left, right}}}
					lastStmt = ast.Stmt{prevStmt.Loc, lastThrow}
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

func (p *parser) visitSingleStmt(stmt ast.Stmt) ast.Stmt {
	stmts := p.visitStmts([]ast.Stmt{stmt})

	// This statement could potentially expand to several statements
	switch len(stmts) {
	case 0:
		return ast.Stmt{stmt.Loc, &ast.SEmpty{}}
	case 1:
		return stmts[0]
	default:
		return ast.Stmt{stmt.Loc, &ast.SBlock{stmts}}
	}
}

func (p *parser) lowerExponentiationAssignmentOperator(loc ast.Loc, e *ast.EBinary) ast.Expr {
	switch left := e.Left.Data.(type) {
	case *ast.EDot:
		if left.OptionalChain == ast.OptionalChainNone {
			referenceFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, left.Target)
			return wrapFunc(ast.Expr{loc, &ast.EBinary{
				Op: ast.BinOpAssign,
				Left: ast.Expr{e.Left.Loc, &ast.EDot{
					Target:  referenceFunc(),
					Name:    left.Name,
					NameLoc: left.NameLoc,
				}},
				Right: p.callRuntime(loc, "__pow", []ast.Expr{
					ast.Expr{e.Left.Loc, &ast.EDot{
						Target:  referenceFunc(),
						Name:    left.Name,
						NameLoc: left.NameLoc,
					}},
					e.Right,
				}),
			}})
		}

	case *ast.EIndex:
		if left.OptionalChain == ast.OptionalChainNone {
			targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, left.Target)
			indexFunc, indexWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, left.Index)
			return targetWrapFunc(indexWrapFunc(ast.Expr{loc, &ast.EBinary{
				Op: ast.BinOpAssign,
				Left: ast.Expr{e.Left.Loc, &ast.EIndex{
					Target: targetFunc(),
					Index:  indexFunc(),
				}},
				Right: p.callRuntime(loc, "__pow", []ast.Expr{
					ast.Expr{e.Left.Loc, &ast.EIndex{
						Target: targetFunc(),
						Index:  indexFunc(),
					}},
					e.Right,
				}),
			}}))
		}

	case *ast.EIdentifier:
		return ast.Expr{loc, &ast.EBinary{
			Op:    ast.BinOpAssign,
			Left:  ast.Expr{e.Left.Loc, &ast.EIdentifier{left.Ref}},
			Right: p.callRuntime(loc, "__pow", []ast.Expr{e.Left, e.Right}),
		}}
	}

	// We shouldn't get here with valid syntax? Just let this through for now
	// since there's currently no assignment target validation. Garbage in,
	// garbage out.
	return ast.Expr{loc, e}
}

// Lower object spread for environments that don't support them. Non-spread
// properties are grouped into object literals and then passed to __assign()
// like this (__assign() is an alias for Object.assign()):
//
//   "{a, b, ...c, d, e}" => "__assign(__assign(__assign({a, b}, c), {d, e})"
//
// If the object literal starts with a spread, then we pass an empty object
// literal to __assign() to make sure we clone the object:
//
//   "{...a, b}" => "__assign(__assign({}, a), {b})"
//
// It's not immediately obvious why we don't compile everything to a single
// call to __assign(). After all, Object.assign() can take any number of
// arguments. The reason is to preserve the order of side effects. Consider
// this code:
//
//   let a = {get x() { b = {y: 2}; return 1 }}
//   let b = {}
//   let c = {...a, ...b}
//
// Converting the above code to "let c = __assign({}, a, b)" means "c" becomes
// "{x: 1}" which is incorrect. Converting the above code instead to
// "let c = __assign(__assign({}, a), b)" means "c" becomes "{x: 1, y: 2}"
// which is correct.
func (p *parser) lowerObjectSpread(loc ast.Loc, e *ast.EObject) ast.Expr {
	needsLowering := false

	if p.target < ES2018 {
		for _, property := range e.Properties {
			if property.Kind == ast.PropertySpread {
				needsLowering = true
				break
			}
		}
	}

	if !needsLowering {
		return ast.Expr{loc, e}
	}

	var result ast.Expr
	properties := []ast.Property{}

	for _, property := range e.Properties {
		if property.Kind != ast.PropertySpread {
			properties = append(properties, property)
			continue
		}

		if len(properties) > 0 || result.Data == nil {
			if result.Data == nil {
				// "{a, ...b}" => "__assign({a}, b)"
				result = ast.Expr{loc, &ast.EObject{
					Properties:   properties,
					IsSingleLine: e.IsSingleLine,
				}}
			} else {
				// "{...a, b, ...c}" => "__assign(__assign(__assign({}, a), {b}), c)"
				result = p.callRuntime(loc, "__assign",
					[]ast.Expr{result, ast.Expr{loc, &ast.EObject{
						Properties:   properties,
						IsSingleLine: e.IsSingleLine,
					}}})
			}
			properties = []ast.Property{}
		}

		// "{a, ...b}" => "__assign({a}, b)"
		result = p.callRuntime(loc, "__assign", []ast.Expr{result, *property.Value})
	}

	if len(properties) > 0 {
		// "{...a, b}" => "__assign(__assign({}, a), {b})"
		result = p.callRuntime(loc, "__assign", []ast.Expr{result, ast.Expr{loc, &ast.EObject{
			Properties:   properties,
			IsSingleLine: e.IsSingleLine,
		}}})
	}

	return result
}

// Lower class fields for environments that don't support them
func (p *parser) lowerClass(classLoc ast.Loc, class *ast.Class, isStmt bool) (
	staticFields []ast.Expr, // Generated static field initializers
	newExpr ast.Expr, // Only for !isStmt, should replace the expression if not nil
) {
	if !p.ts.Parse && p.target >= ESNext {
		return
	}

	var ctor *ast.EFunction
	props := class.Properties
	parameterFields := []ast.Stmt{}
	instanceFields := []ast.Stmt{}
	end := 0

	// These are only for class expressions that need to be captured
	var nameFunc func() ast.Expr
	var wrapFunc func(ast.Expr) ast.Expr
	var classExpr *ast.EClass

	for _, prop := range props {
		// Instance and static fields are a JavaScript feature
		if (p.ts.Parse || p.target < ESNext) && !prop.IsMethod && (prop.IsStatic || prop.Value == nil) {
			// The TypeScript compiler doesn't follow the JavaScript spec for
			// uninitialized fields. They are supposed to be set to undefined but the
			// TypeScript compiler just omits them entirely.
			if p.ts.Parse && prop.Initializer == nil && prop.Value == nil {
				continue
			}

			// Determine where to store the field
			var target ast.Expr
			if prop.IsStatic {
				if nameFunc == nil {
					if !isStmt {
						// If this is a class expression, capture and store it. We have to
						// do this even if it has a name since the name isn't exposed
						// outside the class body.
						classExpr = &ast.EClass{}
						nameFunc, wrapFunc = p.captureValueWithPossibleSideEffects(classLoc, 2, ast.Expr{classLoc, classExpr})
						newExpr = nameFunc()
					} else {
						// Otherwise, this is a class statement. Just use the class name.
						nameFunc = func() ast.Expr {
							p.recordUsage(class.Name.Ref)
							return ast.Expr{classLoc, &ast.EIdentifier{class.Name.Ref}}
						}

						// Class statements can be missing a name if they are in an
						// "export default" statement:
						//
						//   export default class {
						//     static foo = 123
						//   }
						//
						if class.Name == nil {
							class.Name = &ast.LocRef{classLoc, p.generateTempRef(tempRefNoDeclare)}
						}
					}
				}
				target = nameFunc()
			} else {
				target = ast.Expr{prop.Key.Loc, &ast.EThis{}}
			}

			// Generate the assignment target
			if key, ok := prop.Key.Data.(*ast.EString); ok && !prop.IsComputed {
				target = ast.Expr{prop.Key.Loc, &ast.EDot{
					Target:  target,
					Name:    lexer.UTF16ToString(key.Value),
					NameLoc: prop.Key.Loc,
				}}
			} else {
				target = ast.Expr{prop.Key.Loc, &ast.EIndex{
					Target: target,
					Index:  prop.Key,
				}}
			}

			// Generate the assignment initializer
			var init ast.Expr
			if prop.Initializer != nil {
				init = *prop.Initializer
			} else if prop.Value != nil {
				init = *prop.Value
			} else {
				init = ast.Expr{prop.Key.Loc, &ast.EUndefined{}}
			}

			expr := ast.Expr{prop.Key.Loc, &ast.EBinary{ast.BinOpAssign, target, init}}
			if prop.IsStatic {
				// Move this property to an assignment after the class ends
				staticFields = append(staticFields, expr)
			} else {
				// Move this property to an assignment inside the class constructor
				instanceFields = append(instanceFields, ast.Stmt{prop.Key.Loc, &ast.SExpr{expr}})
			}
			continue
		}

		// Remember where the constructor is for later
		if prop.IsMethod && prop.Value != nil {
			if str, ok := prop.Key.Data.(*ast.EString); ok && lexer.UTF16EqualsString(str.Value, "constructor") {
				if fn, ok := prop.Value.Data.(*ast.EFunction); ok {
					ctor = fn

					// Initialize TypeScript constructor parameter fields
					if p.ts.Parse {
						for _, arg := range ctor.Fn.Args {
							if arg.IsTypeScriptCtorField {
								if id, ok := arg.Binding.Data.(*ast.BIdentifier); ok {
									parameterFields = append(parameterFields, ast.Stmt{arg.Binding.Loc, &ast.SExpr{ast.Expr{arg.Binding.Loc, &ast.EBinary{
										ast.BinOpAssign,
										ast.Expr{arg.Binding.Loc, &ast.EDot{
											Target:  ast.Expr{arg.Binding.Loc, &ast.EThis{}},
											Name:    p.symbols[id.Ref.InnerIndex].Name,
											NameLoc: arg.Binding.Loc,
										}},
										ast.Expr{arg.Binding.Loc, &ast.EIdentifier{id.Ref}},
									}}}})
								}
							}
						}
					}
				}
			}
		}

		// Keep this property
		props[end] = prop
		end++
	}

	// Finish the filtering operation
	props = props[:end]

	// Insert instance field initializers into the constructor
	if len(instanceFields) > 0 || len(parameterFields) > 0 {
		// Create a constructor if one doesn't already exist
		if ctor == nil {
			ctor = &ast.EFunction{}

			// Append it to the list to reuse existing allocation space
			props = append(props, ast.Property{
				IsMethod: true,
				Key:      ast.Expr{classLoc, &ast.EString{lexer.StringToUTF16("constructor")}},
				Value:    &ast.Expr{classLoc, ctor},
			})

			// Make sure the constructor has a super() call if needed
			if class.Extends != nil {
				argumentsRef := p.newSymbol(ast.SymbolUnbound, "arguments")
				p.currentScope.Generated = append(p.currentScope.Generated, argumentsRef)
				ctor.Fn.Body.Stmts = append(ctor.Fn.Body.Stmts, ast.Stmt{classLoc, &ast.SExpr{ast.Expr{classLoc, &ast.ECall{
					Target: ast.Expr{classLoc, &ast.ESuper{}},
					Args: []ast.Expr{
						ast.Expr{classLoc, &ast.ESpread{ast.Expr{classLoc, &ast.EIdentifier{argumentsRef}}}},
					},
				}}}})
			}
		}

		// Insert the instance field initializers after the super call if there is one
		stmtsFrom := ctor.Fn.Body.Stmts
		stmtsTo := []ast.Stmt{}
		if len(stmtsFrom) > 0 && ast.IsSuperCall(stmtsFrom[0]) {
			stmtsTo = append(stmtsTo, stmtsFrom[0])
			stmtsFrom = stmtsFrom[1:]
		}
		stmtsTo = append(stmtsTo, parameterFields...)
		stmtsTo = append(stmtsTo, instanceFields...)
		ctor.Fn.Body.Stmts = append(stmtsTo, stmtsFrom...)

		// Sort the constructor first to match the TypeScript compiler's output
		for i := 0; i < len(props); i++ {
			if props[i].Value != nil && props[i].Value.Data == ctor {
				ctorProp := props[i]
				for j := i; j > 0; j-- {
					props[j] = props[j-1]
				}
				props[0] = ctorProp
				break
			}
		}
	}

	class.Properties = props

	// Join the static field initializers if this is a class expression
	if !isStmt && len(staticFields) > 0 {
		classExpr.Class = *class
		for _, initializer := range staticFields {
			newExpr = ast.JoinWithComma(newExpr, initializer)
		}
		newExpr = ast.JoinWithComma(newExpr, nameFunc())
		if wrapFunc != nil {
			newExpr = wrapFunc(newExpr)
		}
	}

	return
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
		for _, i := range b.Items {
			p.visitBinding(i.Binding)
			if i.DefaultValue != nil {
				*i.DefaultValue = p.visitExpr(*i.DefaultValue)
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
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
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

func mangleIf(loc ast.Loc, s *ast.SIf, isTestBooleanConstant bool, testBooleanValue bool) ast.Stmt {
	// Constant folding using the test expression
	if isTestBooleanConstant {
		if testBooleanValue {
			// The test is true
			if s.No == nil || !shouldKeepStmtInDeadControlFlow(*s.No) {
				// We can drop the "no" branch
				if statementCaresAboutScope(s.Yes) {
					return ast.Stmt{s.Yes.Loc, &ast.SBlock{[]ast.Stmt{s.Yes}}}
				} else {
					return s.Yes
				}
			} else {
				// We have to keep the "no" branch
			}
		} else {
			// The test is false
			if !shouldKeepStmtInDeadControlFlow(s.Yes) {
				// We can drop the "yes" branch
				if s.No == nil {
					return ast.Stmt{loc, &ast.SEmpty{}}
				} else if statementCaresAboutScope(*s.No) {
					return ast.Stmt{s.No.Loc, &ast.SBlock{[]ast.Stmt{*s.No}}}
				} else {
					return *s.No
				}
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
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalOr,
					not.Value,
					yes.Value,
				}}}}
			} else {
				// "if (a) b();" => "a && b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalAnd,
					s.Test,
					yes.Value,
				}}}}
			}
		} else if no, ok := s.No.Data.(*ast.SExpr); ok {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) b(); else c();" => "a ? c() : b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EIf{
					not.Value,
					no.Value,
					yes.Value,
				}}}}
			} else {
				// "if (a) b(); else c();" => "a ? b() : c();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EIf{
					s.Test,
					yes.Value,
					no.Value,
				}}}}
			}
		}
	} else if _, ok := s.Yes.Data.(*ast.SEmpty); ok {
		// "yes" is missing
		if s.No == nil {
			// "yes" and "no" are both missing
			if hasNoSideEffects(s.Test.Data) {
				// "if (1) {}" => ";"
				return ast.Stmt{loc, &ast.SEmpty{}}
			} else {
				// "if (a) {}" => "a;"
				return ast.Stmt{loc, &ast.SExpr{s.Test}}
			}
		} else if no, ok := s.No.Data.(*ast.SExpr); ok {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) {} else b();" => "a && b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalAnd,
					not.Value,
					no.Value,
				}}}}
			} else {
				// "if (a) {} else b();" => "a || b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalOr,
					s.Test,
					no.Value,
				}}}}
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
				s.Test = ast.Expr{s.Test.Loc, &ast.EUnary{ast.UnOpNot, s.Test}}
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
		}
	}

	return ast.Stmt{loc, s}
}

func (p *parser) generateClosureForNamespaceOrEnum(
	stmts []ast.Stmt, stmtLoc ast.Loc, isExport bool, nameLoc ast.Loc,
	nameRef ast.Ref, argRef ast.Ref, stmtsInsideClosure []ast.Stmt,
) []ast.Stmt {
	// Follow the link chain in case symbols were merged
	symbol := p.symbols[nameRef.InnerIndex]
	for symbol.Link != ast.InvalidRef {
		nameRef = symbol.Link
		symbol = p.symbols[nameRef.InnerIndex]
	}

	// Make sure to only emit a variable once for a given namespace, since there
	// can be multiple namespace blocks for the same namespace
	if (symbol.Kind == ast.SymbolTSNamespace || symbol.Kind == ast.SymbolTSEnum) && !p.emittedNamespaceVars[nameRef] {
		p.emittedNamespaceVars[nameRef] = true
		if p.enclosingNamespaceRef == nil {
			// Top-level namespace
			stmts = append(stmts, ast.Stmt{stmtLoc, &ast.SLocal{
				Kind:     ast.LocalVar,
				Decls:    []ast.Decl{ast.Decl{ast.Binding{nameLoc, &ast.BIdentifier{nameRef}}, nil}},
				IsExport: isExport,
			}})
		} else {
			// Nested namespace
			stmts = append(stmts, ast.Stmt{stmtLoc, &ast.SLocal{
				Kind:  ast.LocalLet,
				Decls: []ast.Decl{ast.Decl{ast.Binding{nameLoc, &ast.BIdentifier{nameRef}}, nil}},
			}})
		}
	}

	var argExpr ast.Expr
	if isExport && p.enclosingNamespaceRef != nil {
		// "name = enclosing.name || (enclosing.name = {})"
		name := p.symbols[nameRef.InnerIndex].Name
		argExpr = ast.Expr{nameLoc, &ast.EBinary{
			ast.BinOpAssign,
			ast.Expr{nameLoc, &ast.EIdentifier{nameRef}},
			ast.Expr{nameLoc, &ast.EBinary{
				ast.BinOpLogicalOr,
				ast.Expr{nameLoc, &ast.EDot{
					Target:  ast.Expr{nameLoc, &ast.EIdentifier{*p.enclosingNamespaceRef}},
					Name:    name,
					NameLoc: nameLoc,
				}},
				ast.Expr{nameLoc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{nameLoc, &ast.EDot{
						Target:  ast.Expr{nameLoc, &ast.EIdentifier{*p.enclosingNamespaceRef}},
						Name:    name,
						NameLoc: nameLoc,
					}},
					ast.Expr{nameLoc, &ast.EObject{}},
				}},
			}},
		}}
		p.recordUsage(*p.enclosingNamespaceRef)
		p.recordUsage(*p.enclosingNamespaceRef)
		p.recordUsage(nameRef)
	} else {
		// "name || (name = {})"
		argExpr = ast.Expr{nameLoc, &ast.EBinary{
			ast.BinOpLogicalOr,
			ast.Expr{nameLoc, &ast.EIdentifier{nameRef}},
			ast.Expr{nameLoc, &ast.EBinary{
				ast.BinOpAssign,
				ast.Expr{nameLoc, &ast.EIdentifier{nameRef}},
				ast.Expr{nameLoc, &ast.EObject{}},
			}},
		}}
		p.recordUsage(nameRef)
		p.recordUsage(nameRef)
	}

	// Call the closure with the name object
	stmts = append(stmts, ast.Stmt{stmtLoc, &ast.SExpr{ast.Expr{stmtLoc, &ast.ECall{
		Target: ast.Expr{stmtLoc, &ast.EFunction{Fn: ast.Fn{
			Args: []ast.Arg{ast.Arg{Binding: ast.Binding{nameLoc, &ast.BIdentifier{argRef}}}},
			Body: ast.FnBody{stmtLoc, stmtsInsideClosure},
		}}},
		Args: []ast.Expr{argExpr},
	}}}})

	return stmts
}

func (p *parser) visitAndAppendStmt(stmts []ast.Stmt, stmt ast.Stmt) []ast.Stmt {
	switch s := stmt.Data.(type) {
	case *ast.SDebugger, *ast.SEmpty, *ast.SDirective:
		// These don't contain anything to traverse

	case *ast.STypeScript:
		// Erase TypeScript constructs from the output completely
		return stmts

	case *ast.SImport:
		if s.DefaultName != nil {
			p.recordDeclaredSymbol(s.DefaultName.Ref)
		}

		if s.StarNameLoc != nil {
			p.recordDeclaredSymbol(s.NamespaceRef)
		}

		if s.Items != nil {
			for _, item := range *s.Items {
				p.recordDeclaredSymbol(item.Name.Ref)
			}
		}

	case *ast.SExportClause:
		for i, item := range s.Items {
			name := p.loadNameFromRef(item.Name.Ref)
			ref := p.findSymbol(name).ref
			s.Items[i].Name.Ref = ref
			p.recordExport(item.AliasLoc, item.Alias, ref)
		}

	case *ast.SExportFrom:
		// Generate a symbol for the namespace
		name := ast.GenerateNonUniqueNameFromPath(s.Path.Text)
		namespaceRef := p.newSymbol(ast.SymbolOther, name)
		p.currentScope.Generated = append(p.currentScope.Generated, namespaceRef)
		s.NamespaceRef = namespaceRef

		// Path: this is a re-export and the names are symbols in another file
		for i, item := range s.Items {
			name := p.loadNameFromRef(item.Name.Ref)
			ref := p.newSymbol(ast.SymbolUnbound, name)
			s.Items[i].Name.Ref = ref
			p.recordExport(item.AliasLoc, item.Alias, ref)
		}

	case *ast.SExportStar:
		if s.Item != nil {
			// "export * as ns from 'path'"
			name := p.loadNameFromRef(s.Item.Name.Ref)
			ref := p.newSymbol(ast.SymbolOther, name)

			// This name isn't ever declared in this scope, so code in this module
			// must not be able to get to it. Still, we need to associate it with
			// the scope somehow so it will be minified.
			p.currentScope.Generated = append(p.currentScope.Generated, ref)
			s.Item.Name.Ref = ref
			p.recordExport(s.Item.AliasLoc, s.Item.Alias, ref)
		}

	case *ast.SExportDefault:
		p.recordDeclaredSymbol(s.DefaultName.Ref)

		switch {
		case s.Value.Expr != nil:
			*s.Value.Expr = p.visitExpr(*s.Value.Expr)

		case s.Value.Stmt != nil:
			switch s2 := s.Value.Stmt.Data.(type) {
			case *ast.SFunction:
				p.visitFn(&s2.Fn, s.Value.Stmt.Loc)

			case *ast.SClass:
				p.visitClass(&s2.Class)
				stmts = append(stmts, stmt)

				// Lower class field syntax for browsers that don't support it
				extraExprs, _ := p.lowerClass(s.Value.Stmt.Loc, &s2.Class, true /* isStmt */)
				for _, expr := range extraExprs {
					stmts = append(stmts, ast.Stmt{expr.Loc, &ast.SExpr{expr}})
				}
				return stmts

			default:
				panic("Internal error")
			}
		}

	case *ast.SExportEquals:
		// "module.exports = value"
		stmts = append(stmts, ast.Stmt{stmt.Loc, &ast.SExpr{ast.Expr{stmt.Loc, &ast.EBinary{
			ast.BinOpAssign,
			ast.Expr{stmt.Loc, &ast.EDot{
				Target:  ast.Expr{stmt.Loc, &ast.EIdentifier{p.moduleRef}},
				Name:    "exports",
				NameLoc: stmt.Loc,
			}},
			p.visitExpr(s.Value),
		}}}})
		p.recordUsage(p.moduleRef)
		return stmts

	case *ast.SBreak:
		if s.Name != nil {
			name := p.loadNameFromRef(s.Name.Ref)
			s.Name.Ref = p.findLabelSymbol(s.Name.Loc, name)
		}

	case *ast.SContinue:
		if s.Name != nil {
			name := p.loadNameFromRef(s.Name.Ref)
			s.Name.Ref = p.findLabelSymbol(s.Name.Loc, name)
		}

	case *ast.SLabel:
		p.pushScopeForVisitPass(ast.ScopeLabel, stmt.Loc)
		name := p.loadNameFromRef(s.Name.Ref)
		ref := p.newSymbol(ast.SymbolOther, name)
		s.Name.Ref = ref
		p.currentScope.LabelRef = ref
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
				if p.mangleSyntax && s.Kind == ast.LocalLet {
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
			if exprOrNil := p.exprForExportedDeclsInNamespace(s.Decls); exprOrNil.Data != nil {
				stmts = append(stmts, ast.Stmt{stmt.Loc, &ast.SExpr{exprOrNil}})
			}
			return stmts
		}

	case *ast.SExpr:
		s.Value = p.visitExpr(s.Value)

		// Trim expressions without side effects
		if p.mangleSyntax && hasNoSideEffects(s.Value.Data) {
			stmt = ast.Stmt{stmt.Loc, &ast.SEmpty{}}
		}

	case *ast.SThrow:
		s.Value = p.visitExpr(s.Value)

	case *ast.SReturn:
		if s.Value != nil {
			*s.Value = p.visitExpr(*s.Value)

			// Returning undefined is implicit
			if p.mangleSyntax {
				if _, ok := s.Value.Data.(*ast.EUndefined); ok {
					s.Value = nil
				}
			}
		}

	case *ast.SBlock:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		s.Stmts = p.visitStmts(s.Stmts)
		p.popScope()

		if p.mangleSyntax {
			if len(s.Stmts) == 1 && !statementCaresAboutScope(s.Stmts[0]) {
				// Unwrap blocks containing a single statement
				stmt = s.Stmts[0]
			} else if len(s.Stmts) == 0 {
				// Trim empty blocks
				stmt = ast.Stmt{stmt.Loc, &ast.SEmpty{}}
			}
		}

	case *ast.SWith:
		s.Value = p.visitExpr(s.Value)
		p.pushScopeForVisitPass(ast.ScopeWith, s.BodyLoc)
		s.Body = p.visitSingleStmt(s.Body)
		p.popScope()

	case *ast.SWhile:
		s.Test = p.visitBooleanExpr(s.Test)
		s.Body = p.visitSingleStmt(s.Body)

		if p.mangleSyntax {
			// "while (a) {}" => "for (;a;) {}"
			test := &s.Test
			if boolean, ok := toBooleanWithoutSideEffects(s.Test.Data); ok && boolean {
				test = nil
			}
			stmt = ast.Stmt{stmt.Loc, &ast.SFor{Test: test, Body: s.Body}}
		}

	case *ast.SDoWhile:
		s.Body = p.visitSingleStmt(s.Body)
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
			if p.mangleSyntax {
				if _, ok := s.No.Data.(*ast.SEmpty); ok {
					s.No = nil
				}
			}
		}

		if p.mangleSyntax {
			stmt = mangleIf(stmt.Loc, s, ok, boolean)
		}

	case *ast.SFor:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		if s.Init != nil {
			p.visitSingleStmt(*s.Init)
		}

		if s.Test != nil {
			*s.Test = p.visitBooleanExpr(*s.Test)

			// A true value is implied
			if p.mangleSyntax {
				if boolean, ok := toBooleanWithoutSideEffects(s.Test.Data); ok && boolean {
					s.Test = nil
				}
			}
		}

		if s.Update != nil {
			*s.Update = p.visitExpr(*s.Update)
		}
		s.Body = p.visitSingleStmt(s.Body)
		p.popScope()

	case *ast.SForIn:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		p.visitSingleStmt(s.Init)
		s.Value = p.visitExpr(s.Value)
		s.Body = p.visitSingleStmt(s.Body)
		p.popScope()

	case *ast.SForOf:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		p.visitSingleStmt(s.Init)
		s.Value = p.visitExpr(s.Value)
		s.Body = p.visitSingleStmt(s.Body)
		p.popScope()

	case *ast.STry:
		p.pushScopeForVisitPass(ast.ScopeBlock, stmt.Loc)
		p.tryBodyCount++
		s.Body = p.visitStmts(s.Body)
		p.tryBodyCount--
		p.popScope()

		if s.Catch != nil {
			p.pushScopeForVisitPass(ast.ScopeBlock, s.Catch.Loc)
			if s.Catch.Binding != nil {
				p.visitBinding(*s.Catch.Binding)
			}
			s.Catch.Body = p.visitStmts(s.Catch.Body)
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
		for i, c := range s.Cases {
			if c.Value != nil {
				*c.Value = p.visitExpr(*c.Value)
			}
			c.Body = p.visitStmts(c.Body)

			// Make sure the assignment to the body above is preserved
			s.Cases[i] = c
		}
		p.popScope()

	case *ast.SFunction:
		p.recordDeclaredSymbol(s.Fn.Name.Ref)
		p.visitFn(&s.Fn, stmt.Loc)

		// Handle exporting this function from a namespace
		if s.IsExport && p.enclosingNamespaceRef != nil {
			s.IsExport = false
			stmts = append(stmts,
				stmt,
				ast.Stmt{stmt.Loc, &ast.SExpr{ast.Expr{stmt.Loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{stmt.Loc, &ast.EDot{
						Target:  ast.Expr{stmt.Loc, &ast.EIdentifier{*p.enclosingNamespaceRef}},
						Name:    p.symbols[s.Fn.Name.Ref.InnerIndex].Name,
						NameLoc: s.Fn.Name.Loc,
					}},
					ast.Expr{s.Fn.Name.Loc, &ast.EIdentifier{s.Fn.Name.Ref}},
				}}}},
			)
			return stmts
		}

	case *ast.SClass:
		p.recordDeclaredSymbol(s.Class.Name.Ref)
		p.visitClass(&s.Class)
		stmts = append(stmts, stmt)

		// Lower class field syntax for browsers that don't support it
		extraExprs, _ := p.lowerClass(stmt.Loc, &s.Class, true /* isStmt */)
		for _, expr := range extraExprs {
			stmts = append(stmts, ast.Stmt{expr.Loc, &ast.SExpr{expr}})
		}

		// Handle exporting this class from a namespace
		if s.IsExport && p.enclosingNamespaceRef != nil {
			s.IsExport = false
			stmts = append(stmts,
				ast.Stmt{stmt.Loc, &ast.SExpr{ast.Expr{stmt.Loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{stmt.Loc, &ast.EDot{
						Target:  ast.Expr{stmt.Loc, &ast.EIdentifier{*p.enclosingNamespaceRef}},
						Name:    p.symbols[s.Class.Name.Ref.InnerIndex].Name,
						NameLoc: s.Class.Name.Loc,
					}},
					ast.Expr{s.Class.Name.Loc, &ast.EIdentifier{s.Class.Name.Ref}},
				}}}},
			)
			return stmts
		}

		return stmts

	case *ast.SEnum:
		p.recordDeclaredSymbol(s.Name.Ref)
		p.pushScopeForVisitPass(ast.ScopeEntry, stmt.Loc)
		defer p.popScope()

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
				case *ast.ETemplate:
					hasStringValue = e.Tag == nil && len(e.Parts) == 0
				}
			} else if hasNumericValue {
				valuesSoFar[name] = nextNumericValue
				value.Value = &ast.Expr{value.Loc, &ast.ENumber{nextNumericValue}}
				nextNumericValue++
			} else {
				value.Value = &ast.Expr{value.Loc, &ast.EUndefined{}}
			}

			if p.mangleSyntax && lexer.IsIdentifier(name) {
				// "Enum.Name = value"
				assignTarget = ast.Expr{value.Loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{value.Loc, &ast.EDot{
						Target:  ast.Expr{value.Loc, &ast.EIdentifier{s.Arg}},
						Name:    name,
						NameLoc: value.Loc,
					}},
					*value.Value,
				}}
			} else {
				// "Enum['Name'] = value"
				assignTarget = ast.Expr{value.Loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{value.Loc, &ast.EIndex{
						Target: ast.Expr{value.Loc, &ast.EIdentifier{s.Arg}},
						Index:  ast.Expr{value.Loc, &ast.EString{value.Name}},
					}},
					*value.Value,
				}}
			}
			p.recordUsage(s.Arg)

			// String-valued enums do not form a two-way map
			if hasStringValue {
				valueExprs = append(valueExprs, assignTarget)
			} else {
				// "Enum[assignTarget] = 'Name'"
				valueExprs = append(valueExprs, ast.Expr{value.Loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{value.Loc, &ast.EIndex{
						Target: ast.Expr{value.Loc, &ast.EIdentifier{s.Arg}},
						Index:  assignTarget,
					}},
					ast.Expr{value.Loc, &ast.EString{value.Name}},
				}})
			}
			p.recordUsage(s.Arg)
		}

		p.shouldFoldNumericConstants = oldShouldFoldNumericConstants

		// Generate statements from expressions
		valueStmts := []ast.Stmt{}
		if len(valueExprs) > 0 {
			if p.mangleSyntax {
				// "a; b; c;" => "a, b, c;"
				joined := ast.JoinAllWithComma(valueExprs)
				valueStmts = append(valueStmts, ast.Stmt{joined.Loc, &ast.SExpr{joined}})
			} else {
				for _, expr := range valueExprs {
					valueStmts = append(valueStmts, ast.Stmt{expr.Loc, &ast.SExpr{expr}})
				}
			}
		}

		// Wrap this enum definition in a closure
		stmts = p.generateClosureForNamespaceOrEnum(
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
		stmtsInsideNamespace := p.visitStmtsAndPrependTempRefs(s.Stmts)
		p.popScope()
		p.enclosingNamespaceRef = oldEnclosingNamespaceRef

		// Generate a closure for this namespace
		stmts = p.generateClosureForNamespaceOrEnum(
			stmts, stmt.Loc, s.IsExport, s.Name.Loc, s.Name.Ref, s.Arg, stmtsInsideNamespace)
		return stmts

	default:
		panic(fmt.Sprintf("Unexpected statement of type %T", stmt.Data))
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
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
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

func (p *parser) exprForExportedDeclsInNamespace(decls []ast.Decl) ast.Expr {
	var expr ast.Expr
	for _, decl := range decls {
		if decl.Value != nil {
			expr = maybeJoinWithComma(expr, p.exprForExportedBindingInNamespace(decl.Binding, *decl.Value))
		}
	}
	return expr
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
//   return wrapFunc(ast.Expr{loc, &ast.EBinary{
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
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.ENull{}} }
	case *ast.EUndefined:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.EUndefined{}} }
	case *ast.EThis:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.EThis{}} }
	case *ast.EBoolean:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.EBoolean{e.Value}} }
	case *ast.ENumber:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.ENumber{e.Value}} }
	case *ast.EBigInt:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.EBigInt{e.Value}} }
	case *ast.EString:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.EString{e.Value}} }
	case *ast.EIdentifier:
		valueFunc = func() ast.Expr { return ast.Expr{loc, &ast.EIdentifier{e.Ref}} }
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
					tempRef = p.generateTempRef(tempRefNoDeclare)

					// Assign inline so the order of side effects remains the same
					p.recordUsage(tempRef)
					return ast.Expr{loc, &ast.EBinary{
						ast.BinOpAssign,
						ast.Expr{loc, &ast.EIdentifier{tempRef}},
						value,
					}}
				}
				p.recordUsage(tempRef)
				return ast.Expr{loc, &ast.EIdentifier{tempRef}}
			}, func(expr ast.Expr) ast.Expr {
				// Make sure side effects still happen if no expression was generated
				if expr.Data == nil {
					return value
				}

				// Generate a new variable using an arrow function to avoid messing with "this"
				return ast.Expr{loc, &ast.ECall{
					Target: ast.Expr{loc, &ast.EArrow{
						Args:       []ast.Arg{ast.Arg{Binding: ast.Binding{loc, &ast.BIdentifier{tempRef}}}},
						PreferExpr: true,
						Body:       ast.FnBody{loc, []ast.Stmt{ast.Stmt{loc, &ast.SReturn{&expr}}}},
					}},
					Args: []ast.Expr{},
				}}
			}
	}

	return func() ast.Expr {
		if tempRef == ast.InvalidRef {
			tempRef = p.generateTempRef(tempRefNeedsDeclare)
			p.recordUsage(tempRef)
			return ast.Expr{loc, &ast.EBinary{
				ast.BinOpAssign,
				ast.Expr{loc, &ast.EIdentifier{tempRef}},
				value,
			}}
		}
		p.recordUsage(tempRef)
		return ast.Expr{loc, &ast.EIdentifier{tempRef}}
	}, wrapFunc
}

// Returns "typeof ref === 'symbol' ? ref : ref + ''"
func symbolOrString(loc ast.Loc, ref ast.Ref) ast.Expr {
	return ast.Expr{loc, &ast.EIf{
		Test: ast.Expr{loc, &ast.EBinary{
			Op: ast.BinOpStrictEq,
			Left: ast.Expr{loc, &ast.EUnary{
				Op:    ast.UnOpTypeof,
				Value: ast.Expr{loc, &ast.EIdentifier{ref}},
			}},
			Right: ast.Expr{loc, &ast.EString{lexer.StringToUTF16("symbol")}},
		}},
		Yes: ast.Expr{loc, &ast.EIdentifier{ref}},
		No: ast.Expr{loc, &ast.EBinary{
			Op:    ast.BinOpAdd,
			Left:  ast.Expr{loc, &ast.EIdentifier{ref}},
			Right: ast.Expr{loc, &ast.EString{}},
		}},
	}}
}

func (p *parser) exprForExportedBindingInNamespace(binding ast.Binding, value ast.Expr) ast.Expr {
	loc := binding.Loc

	switch d := binding.Data.(type) {
	case *ast.BMissing:
		return ast.Expr{}

	case *ast.BIdentifier:
		return ast.Expr{loc, &ast.EBinary{
			ast.BinOpAssign,
			ast.Expr{loc, &ast.EDot{
				Target:  ast.Expr{loc, &ast.EIdentifier{*p.enclosingNamespaceRef}},
				Name:    p.symbols[d.Ref.InnerIndex].Name,
				NameLoc: loc,
			}},
			value,
		}}

	case *ast.BArray:
		var expr ast.Expr
		valueFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, len(d.Items), value)
		for i, item := range d.Items {
			var itemValue ast.Expr
			if d.HasSpread && i+1 == len(d.Items) {
				// "array.slice(i)"
				itemValue = ast.Expr{loc, &ast.ECall{
					Target: ast.Expr{loc, &ast.EDot{
						Target:  valueFunc(),
						Name:    "slice",
						NameLoc: loc,
					}},
					Args: []ast.Expr{ast.Expr{loc, &ast.ENumber{float64(i)}}},
				}}
			} else {
				// "array[i]"
				itemValue = ast.Expr{loc, &ast.EIndex{
					Target: valueFunc(),
					Index:  ast.Expr{loc, &ast.ENumber{float64(i)}},
				}}
			}

			// Handle default values
			if item.DefaultValue != nil {
				tempRef := p.generateTempRef(tempRefNeedsDeclare)
				expr = maybeJoinWithComma(expr, ast.Expr{loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{loc, &ast.EIdentifier{tempRef}},
					itemValue,
				}})
				itemValue = ast.Expr{loc, &ast.EIf{
					ast.Expr{loc, &ast.EBinary{
						ast.BinOpStrictEq,
						ast.Expr{loc, &ast.EIdentifier{tempRef}},
						ast.Expr{loc, &ast.EUndefined{}},
					}},
					*item.DefaultValue,
					ast.Expr{loc, &ast.EIdentifier{tempRef}},
				}}
			}

			expr = maybeJoinWithComma(expr, p.exprForExportedBindingInNamespace(item.Binding, itemValue))
		}
		return wrapFunc(expr)

	case *ast.BObject:
		var expr ast.Expr
		valueFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, len(d.Properties), value)

		// We will need to track the keys we use if this pattern contains a spread
		keysForSpread := []ast.Expr{}
		isSpread := false
		for _, property := range d.Properties {
			if property.IsSpread {
				isSpread = true
				break
			}
		}

		for _, property := range d.Properties {
			var propertyValue ast.Expr
			if property.IsSpread {
				// Call out to the __rest() helper function to implement spread
				propertyValue = p.callRuntime(loc, "__rest", []ast.Expr{
					valueFunc(),
					ast.Expr{loc, &ast.EArray{
						Items:        keysForSpread,
						IsSingleLine: true,
					}},
				})
			} else {
				key := property.Key

				// We need to save a copy of the key if there's a spread later on
				if isSpread {
					// Do different things based on the key expression. Certain
					// expressions can be converted to keys more efficiently than others.
					switch k := key.Data.(type) {
					case *ast.EString:
						keysForSpread = append(keysForSpread, ast.Expr{key.Loc, &ast.EString{k.Value}})

					case *ast.ENumber:
						asUint32 := uint32(k.Value)
						if k.Value == float64(asUint32) {
							// If this is an integer, emit it as a string
							text := lexer.StringToUTF16(strconv.FormatInt(int64(asUint32), 10))
							keysForSpread = append(keysForSpread, ast.Expr{key.Loc, &ast.EString{text}})
						} else {
							// Otherwise, emit it as the number plus a string (i.e. call
							// toString() on it). It's important to do it this way instead of
							// trying to print the float as a string because Go's floating-
							// point printer doesn't behave exactly the same as JavaScript
							// and if they are different, the generated code will be wrong.
							keysForSpread = append(keysForSpread, ast.Expr{key.Loc, &ast.EBinary{
								Op:    ast.BinOpAdd,
								Left:  ast.Expr{key.Loc, &ast.ENumber{k.Value}},
								Right: ast.Expr{key.Loc, &ast.EString{}},
							}})
						}

					case *ast.EIdentifier:
						keysForSpread = append(keysForSpread, symbolOrString(key.Loc, k.Ref))

					default:
						tempRef := p.generateTempRef(tempRefNeedsDeclare)
						key = ast.Expr{loc, &ast.EBinary{
							ast.BinOpAssign,
							ast.Expr{loc, &ast.EIdentifier{tempRef}},
							key,
						}}
						keysForSpread = append(keysForSpread, symbolOrString(key.Loc, tempRef))
					}
				}

				// Try to use a dot expression but fall back to an index expression
				name := ""
				if id, ok := key.Data.(*ast.EString); ok && lexer.IsIdentifierUTF16(id.Value) {
					name = lexer.UTF16ToString(id.Value)
				}
				if name != "" {
					propertyValue = ast.Expr{loc, &ast.EDot{
						Target:  valueFunc(),
						Name:    name,
						NameLoc: loc,
					}}
				} else {
					propertyValue = ast.Expr{loc, &ast.EIndex{
						Target: valueFunc(),
						Index:  key,
					}}
				}
			}

			// Handle default values
			if property.DefaultValue != nil {
				tempRef := p.generateTempRef(tempRefNeedsDeclare)
				expr = maybeJoinWithComma(expr, ast.Expr{loc, &ast.EBinary{
					ast.BinOpAssign,
					ast.Expr{loc, &ast.EIdentifier{tempRef}},
					propertyValue,
				}})
				propertyValue = ast.Expr{loc, &ast.EIf{
					ast.Expr{loc, &ast.EBinary{
						ast.BinOpStrictEq,
						ast.Expr{loc, &ast.EIdentifier{tempRef}},
						ast.Expr{loc, &ast.EUndefined{}},
					}},
					*property.DefaultValue,
					ast.Expr{loc, &ast.EIdentifier{tempRef}},
				}}
			}

			expr = maybeJoinWithComma(expr, p.exprForExportedBindingInNamespace(property.Value, propertyValue))
		}
		return wrapFunc(expr)

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
		return ast.Expr{}
	}
}

func (p *parser) visitClass(class *ast.Class) {
	if class.Extends != nil {
		*class.Extends = p.visitExpr(*class.Extends)
	}

	oldIsThisCaptured := p.isThisCaptured
	p.isThisCaptured = true

	// A scope is needed for private identifiers
	p.pushScopeForVisitPass(ast.ScopeClassBody, class.BodyLoc)
	defer p.popScope()

	for i, property := range class.Properties {
		// Special-case EPrivateIdentifier to allow it here
		if _, ok := property.Key.Data.(*ast.EPrivateIdentifier); !ok {
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

func (p *parser) stringsToMemberExpression(loc ast.Loc, parts []string) ast.Expr {
	ref := p.findSymbol(parts[0]).ref
	value := ast.Expr{loc, &ast.EIdentifier{ref}}

	// Substitute an EImportIdentifier now if this is an import item
	if p.isImportItem[ref] {
		value.Data = &ast.EImportIdentifier{ref}
	}

	for i := 1; i < len(parts); i++ {
		value = p.maybeRewriteDot(loc, &ast.EDot{
			Target:  value,
			Name:    parts[i],
			NameLoc: loc,
		})
	}
	return value
}

func (p *parser) warnAboutEqualityCheck(op string, value ast.Expr, afterOpLoc ast.Loc) bool {
	switch e := value.Data.(type) {
	case *ast.ENumber:
		if e.Value == 0 && math.Signbit(e.Value) {
			p.log.AddWarning(p.source, value.Loc,
				fmt.Sprintf("Comparison with -0 using the %s operator will also match 0", op))
			return true
		}

	case *ast.EArray, *ast.EArrow, *ast.EClass,
		*ast.EFunction, *ast.EObject, *ast.ERegExp:
		index := strings.LastIndex(p.source.Contents[:afterOpLoc.Start], op)
		p.log.AddRangeWarning(p.source, ast.Range{ast.Loc{int32(index)}, int32(len(op))},
			fmt.Sprintf("Comparison using the %s operator here is always %v", op, op[0] == '!'))
		return true
	}

	return false
}

// EDot nodes represent a property access. This function may return an
// expression to replace the property access with. It assumes that the
// target of the EDot expression has already been visited.
func (p *parser) maybeRewriteDot(loc ast.Loc, data *ast.EDot) ast.Expr {
	if id, ok := data.Target.Data.(*ast.EIdentifier); ok {
		// Rewrite property accesses on explicit namespace imports as an identifier.
		// This lets us replace them easily in the printer to rebind them to
		// something else without paying the cost of a whole-tree traversal during
		// module linking just to rewrite these EDot expressions.
		if importItems, ok := p.importItemsForNamespace[id.Ref]; ok {
			// Cache translation so each property access resolves to the same import
			item, ok := importItems[data.Name]
			if !ok {
				// Generate a new import item symbol in the module scope
				item = ast.LocRef{data.NameLoc, p.newSymbol(ast.SymbolOther, data.Name)}
				p.moduleScope.Generated = append(p.moduleScope.Generated, item.Ref)

				// Link the namespace import and the import item together
				importItems[data.Name] = item
				p.isImportItem[item.Ref] = true

				symbol := &p.symbols[item.Ref.InnerIndex]
				if !p.isBundling {
					// Make sure the printer prints this as a property access
					symbol.NamespaceAlias = &ast.NamespaceAlias{
						NamespaceRef: id.Ref,
						Alias:        data.Name,
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
			if p.isBundling {
				p.ignoreUsage(id.Ref)
			}

			// Track how many times we've referenced this symbol
			p.recordUsage(item.Ref)
			return ast.Expr{loc, &ast.EImportIdentifier{item.Ref}}
		}

		// If this is a known enum value, inline the value of the enum
		if p.ts.Parse {
			if enumValueMap, ok := p.knownEnumValues[id.Ref]; ok {
				if number, ok := enumValueMap[data.Name]; ok {
					return ast.Expr{loc, &ast.ENumber{number}}
				}
			}
		}
	}

	return ast.Expr{loc, data}
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
			return &ast.Expr{left.Loc, &ast.EString{joinStrings(l.Value, r.Value)}}

		case *ast.ETemplate:
			if r.Tag == nil {
				return &ast.Expr{left.Loc, &ast.ETemplate{nil, joinStrings(l.Value, r.Head), "", r.Parts}}
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
				return &ast.Expr{left.Loc, &ast.ETemplate{nil, head, "", parts}}

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
					return &ast.Expr{left.Loc, &ast.ETemplate{nil, head, "", parts}}
				}
			}
		}
	}

	return nil
}

func (p *parser) visitBooleanExpr(expr ast.Expr) ast.Expr {
	expr = p.visitExpr(expr)

	// Simplify syntax when we know it's used inside a boolean context
	if p.mangleSyntax {
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

// Lower optional chaining for environments that don't support it
func (p *parser) lowerOptionalChain(expr ast.Expr, in exprIn, out exprOut, thisArgFunc func() ast.Expr) (ast.Expr, exprOut) {
	valueWhenUndefined := ast.Expr{expr.Loc, &ast.EUndefined{}}
	endsWithPropertyAccess := false
	startsWithCall := false
	originalExpr := expr
	chain := []ast.Expr{}
	loc := expr.Loc

	// Step 1: Get an array of all expressions in the chain. We're traversing the
	// chain from the outside in, so the array will be filled in "backwards".
flatten:
	for {
		chain = append(chain, expr)

		switch e := expr.Data.(type) {
		case *ast.EDot:
			expr = e.Target
			if len(chain) == 1 {
				endsWithPropertyAccess = true
			}
			if e.OptionalChain == ast.OptionalChainStart {
				break flatten
			}

		case *ast.EIndex:
			expr = e.Target
			if len(chain) == 1 {
				endsWithPropertyAccess = true
			}
			if e.OptionalChain == ast.OptionalChainStart {
				break flatten
			}

		case *ast.ECall:
			expr = e.Target
			if e.OptionalChain == ast.OptionalChainStart {
				startsWithCall = true
				break flatten
			}

		case *ast.EUnary: // UnOpDelete
			valueWhenUndefined = ast.Expr{loc, &ast.EBoolean{Value: true}}
			expr = e.Value

		default:
			panic("Internal error")
		}
	}

	// Stop now if we can strip the whole chain as dead code. Since the chain is
	// lazily evaluated, it's safe to just drop the code entirely.
	switch expr.Data.(type) {
	case *ast.ENull, *ast.EUndefined:
		return valueWhenUndefined, exprOut{}
	}

	// Don't lower this if we don't need to. This check must be done here instead
	// of earlier so we can do the dead code elimination above when the target is
	// null or undefined.
	if p.target >= ES2020 {
		return originalExpr, exprOut{}
	}

	// Step 2: Figure out if we need to capture the value for "this" for the
	// initial ECall. This will be passed to ".call(this, ...args)" later.
	var thisArg ast.Expr
	var targetWrapFunc func(ast.Expr) ast.Expr
	if startsWithCall {
		if thisArgFunc != nil {
			// The initial value is a nested optional chain that ended in a property
			// access. The nested chain was processed first and has saved the
			// appropriate value for "this". The callback here will return a
			// reference to that saved location.
			thisArg = thisArgFunc()
		} else {
			// The initial value is a normal expression. If it's a property access,
			// strip the property off and save the target of the property access to
			// be used as the value for "this".
			switch e := expr.Data.(type) {
			case *ast.EDot:
				targetFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, e.Target)
				expr = ast.Expr{loc, &ast.EDot{
					Target:  targetFunc(),
					Name:    e.Name,
					NameLoc: e.NameLoc,
				}}
				thisArg = targetFunc()
				targetWrapFunc = wrapFunc

			case *ast.EIndex:
				targetFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, e.Target)
				expr = ast.Expr{loc, &ast.EIndex{
					Target: targetFunc(),
					Index:  e.Index,
				}}
				thisArg = targetFunc()
				targetWrapFunc = wrapFunc
			}
		}
	}

	// Step 3: Figure out if we need to capture the starting value. We don't need
	// to capture it if it doesn't have any side effects (e.g. it's just a bare
	// identifier). Skipping the capture reduces code size and matches the output
	// of the TypeScript compiler.
	exprFunc, exprWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, expr)
	expr = exprFunc()
	result := exprFunc()

	// Step 4: Wrap the starting value by each expression in the chain. We
	// traverse the chain in reverse because we want to go from the inside out
	// and the chain was built from the outside in.
	for i := len(chain) - 1; i >= 0; i-- {
		// Save a reference to the value of "this" for our parent ECall
		if i == 0 && in.storeThisArgForParentOptionalChain != nil && endsWithPropertyAccess {
			result = in.storeThisArgForParentOptionalChain(result)
		}

		switch e := chain[i].Data.(type) {
		case *ast.EDot:
			result = ast.Expr{loc, &ast.EDot{
				Target:  result,
				Name:    e.Name,
				NameLoc: e.NameLoc,
			}}

		case *ast.EIndex:
			result = ast.Expr{loc, &ast.EIndex{
				Target: result,
				Index:  e.Index,
			}}

		case *ast.ECall:
			// If this is the initial ECall in the chain and it's being called off of
			// a property access, invoke the function using ".call(this, ...args)" to
			// explicitly provide the value for "this".
			if i == len(chain)-1 && thisArg.Data != nil {
				result = ast.Expr{loc, &ast.ECall{
					Target: ast.Expr{loc, &ast.EDot{
						Target:  result,
						Name:    "call",
						NameLoc: loc,
					}},
					Args: append([]ast.Expr{thisArg}, e.Args...),
				}}
				break
			}

			result = ast.Expr{loc, &ast.ECall{
				Target:       result,
				Args:         e.Args,
				IsDirectEval: e.IsDirectEval,
			}}

		case *ast.EUnary:
			result = ast.Expr{loc, &ast.EUnary{
				Op:    ast.UnOpDelete,
				Value: result,
			}}

		default:
			panic("Internal error")
		}
	}

	// Step 5: Wrap it all in a conditional that returns the chain or the default
	// value if the initial value is null/undefined. The default value is usually
	// "undefined" but is "true" if the chain ends in a "delete" operator.
	result = ast.Expr{loc, &ast.EIf{
		Test: ast.Expr{loc, &ast.EBinary{
			Op:    ast.BinOpLooseEq,
			Left:  expr,
			Right: ast.Expr{loc, &ast.ENull{}},
		}},
		Yes: valueWhenUndefined,
		No:  result,
	}}
	if exprWrapFunc != nil {
		result = exprWrapFunc(result)
	}
	if targetWrapFunc != nil {
		result = targetWrapFunc(result)
	}
	return result, exprOut{}
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
	isAssignTarget bool
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

func (p *parser) visitExprInOut(expr ast.Expr, in exprIn) (ast.Expr, exprOut) {
	switch e := expr.Data.(type) {
	case *ast.EMissing, *ast.ENull, *ast.ESuper, *ast.EString,
		*ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.ERegExp, *ast.ENewTarget, *ast.EUndefined:

	case *ast.EThis:
		if p.isBundling && !p.isThisCaptured {
			if p.hasES6ImportSyntax || p.hasES6ExportSyntax {
				// In an ES6 module, "this" is supposed to be undefined. Instead of
				// doing this at runtime using "fn.call(undefined)", we do it at
				// compile time using expression substitution here.
				return ast.Expr{expr.Loc, &ast.EUndefined{}}, exprOut{}
			} else {
				// In a CommonJS module, "this" is supposed to be the same as "exports".
				// Instead of doing this at runtime using "fn.call(module.exports)", we
				// do it at compile time using expression substitution here.
				p.recordUsage(p.exportsRef)
				return ast.Expr{expr.Loc, &ast.EIdentifier{p.exportsRef}}, exprOut{}
			}
		}

	case *ast.EImportMeta:
		if p.isBundling {
			// Replace "import.meta" with a dummy object when bundling
			return ast.Expr{expr.Loc, &ast.EObject{}}, exprOut{}
		}

	case *ast.ESpread:
		e.Value = p.visitExpr(e.Value)

	case *ast.EIdentifier:
		name := p.loadNameFromRef(e.Ref)
		result := p.findSymbol(name)
		e.Ref = result.ref

		// Substitute user-specified defines for unbound symbols
		if p.symbols[e.Ref.InnerIndex].Kind == ast.SymbolUnbound && !result.isInsideWithScope {
			if defineFunc, ok := p.processedDefines.IdentifierDefines[name]; ok {
				new := p.valueForDefine(expr.Loc, defineFunc)

				// Don't substitute an identifier for a non-identifier if this is an
				// assignment target, since it'll cause a syntax error
				if _, ok := new.Data.(*ast.EIdentifier); !in.isAssignTarget || ok {
					return new, exprOut{}
				}
			}
		}

		return p.handleIdentifier(expr.Loc, e), exprOut{}

	case *ast.EPrivateIdentifier:
		// We should never get here
		panic("Internal error")

	case *ast.EJSXElement:
		// A missing tag is a fragment
		tag := e.Tag
		if tag == nil {
			value := p.stringsToMemberExpression(expr.Loc, p.jsx.Fragment)
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
			args = append(args, ast.Expr{expr.Loc, &ast.ENull{}})
		}
		if len(e.Children) > 0 {
			for _, child := range e.Children {
				args = append(args, p.visitExpr(child))
			}
		}

		// Call createElement()
		return ast.Expr{expr.Loc, &ast.ECall{
			Target: p.stringsToMemberExpression(expr.Loc, p.jsx.Factory),
			Args:   args,
		}}, exprOut{}

	case *ast.ETemplate:
		if e.Tag != nil {
			*e.Tag = p.visitExpr(*e.Tag)
		}
		for i, part := range e.Parts {
			e.Parts[i].Value = p.visitExpr(part.Value)
		}

	case *ast.EBinary:
		e.Left, _ = p.visitExprInOut(e.Left, exprIn{isAssignTarget: e.Op.IsBinaryAssign()})
		e.Right = p.visitExpr(e.Right)

		// Fold constants
		switch e.Op {
		case ast.BinOpLooseEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{result}}, exprOut{}
			} else if !p.warnAboutEqualityCheck("==", e.Left, e.Right.Loc) {
				p.warnAboutEqualityCheck("==", e.Right, e.Right.Loc)
			}

		case ast.BinOpStrictEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{result}}, exprOut{}
			} else if !p.warnAboutEqualityCheck("===", e.Left, e.Right.Loc) {
				p.warnAboutEqualityCheck("===", e.Right, e.Right.Loc)
			}

		case ast.BinOpLooseNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{!result}}, exprOut{}
			} else if !p.warnAboutEqualityCheck("!=", e.Left, e.Right.Loc) {
				p.warnAboutEqualityCheck("!=", e.Right, e.Right.Loc)
			}

		case ast.BinOpStrictNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{!result}}, exprOut{}
			} else if !p.warnAboutEqualityCheck("!==", e.Left, e.Right.Loc) {
				p.warnAboutEqualityCheck("!==", e.Right, e.Right.Loc)
			}

		case ast.BinOpNullishCoalescing:
			switch e.Left.Data.(type) {
			case *ast.EBoolean, *ast.ENumber, *ast.EString, *ast.ERegExp,
				*ast.EObject, *ast.EArray, *ast.EFunction, *ast.EArrow, *ast.EClass:
				return e.Left, exprOut{}

			case *ast.ENull, *ast.EUndefined:
				return e.Right, exprOut{}

			default:
				if p.target < ES2020 {
					// "a ?? b" => "a != null ? a : b"
					// "a() ?? b()" => "_ = a(), _ != null ? _ : b"
					leftFunc, wrapFunc := p.captureValueWithPossibleSideEffects(expr.Loc, 2, e.Left)
					return wrapFunc(ast.Expr{e.Right.Loc, &ast.EIf{
						ast.Expr{expr.Loc, &ast.EBinary{
							ast.BinOpLooseNe,
							leftFunc(),
							ast.Expr{expr.Loc, &ast.ENull{}},
						}},
						leftFunc(),
						e.Right,
					}}), exprOut{}
				}
			}

		case ast.BinOpLogicalOr:
			if boolean, ok := toBooleanWithoutSideEffects(e.Left.Data); ok {
				if boolean {
					return e.Left, exprOut{}
				} else {
					return e.Right, exprOut{}
				}
			}

		case ast.BinOpLogicalAnd:
			if boolean, ok := toBooleanWithoutSideEffects(e.Left.Data); ok {
				if boolean {
					return e.Right, exprOut{}
				} else {
					return e.Left, exprOut{}
				}
			}

		case ast.BinOpAdd:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{left + right}}, exprOut{}
				}
			}

			// "'abc' + 'xyz'" => "'abcxyz'"
			if result := foldStringAddition(e.Left, e.Right); result != nil {
				return *result, exprOut{}
			}

			if left, ok := e.Left.Data.(*ast.EBinary); ok && left.Op == ast.BinOpAdd {
				// "x + 'abc' + 'xyz'" => "x + 'abcxyz'"
				if result := foldStringAddition(left.Right, e.Right); result != nil {
					return ast.Expr{expr.Loc, &ast.EBinary{left.Op, left.Left, *result}}, exprOut{}
				}
			}

		case ast.BinOpSub:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{left - right}}, exprOut{}
				}
			}

		case ast.BinOpMul:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{left * right}}, exprOut{}
				}
			}

		case ast.BinOpDiv:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{left / right}}, exprOut{}
				}
			}

		case ast.BinOpRem:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{math.Mod(left, right)}}, exprOut{}
				}
			}

		case ast.BinOpPow:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{math.Pow(left, right)}}, exprOut{}
				}
			}

			// Lower the exponentiation operator for browsers that don't support it
			if p.target < ES2016 {
				return p.callRuntime(expr.Loc, "__pow", []ast.Expr{e.Left, e.Right}), exprOut{}
			}

		case ast.BinOpPowAssign:
			// Lower the exponentiation operator for browsers that don't support it
			if p.target < ES2016 {
				return p.lowerExponentiationAssignmentOperator(expr.Loc, e), exprOut{}
			}

		case ast.BinOpShl:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{float64(toInt32(left) << (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case ast.BinOpShr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{float64(toInt32(left) >> (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case ast.BinOpUShr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{float64(toUint32(left) >> (toUint32(right) & 31))}}, exprOut{}
				}
			}

		case ast.BinOpBitwiseAnd:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{float64(toInt32(left) & toInt32(right))}}, exprOut{}
				}
			}

		case ast.BinOpBitwiseOr:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{float64(toInt32(left) | toInt32(right))}}, exprOut{}
				}
			}

		case ast.BinOpBitwiseXor:
			if p.shouldFoldNumericConstants {
				if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
					return ast.Expr{expr.Loc, &ast.ENumber{float64(toInt32(left) ^ toInt32(right))}}, exprOut{}
				}
			}
		}

	case *ast.EIndex:
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
			if !p.symbols[result.ref.InnerIndex].Kind.IsPrivate() {
				r := ast.Range{e.Index.Loc, int32(len(name))}
				p.addRangeError(r, fmt.Sprintf("Private name %q is not available here", name))
			}
		} else {
			e.Index = p.visitExpr(e.Index)
		}

		// Lower optional chaining if we're the top of the chain
		containsOptionalChain := e.OptionalChain != ast.OptionalChainNone
		if containsOptionalChain && !in.hasChainParent {
			return p.lowerOptionalChain(expr, in, out, nil)
		}

		if p.mangleSyntax || p.ts.Parse {
			if str, ok := e.Index.Data.(*ast.EString); ok {
				// If this is a known enum value, inline the value of the enum
				if p.ts.Parse {
					if id, ok := e.Target.Data.(*ast.EIdentifier); ok {
						if enumValueMap, ok := p.knownEnumValues[id.Ref]; ok {
							if number, ok := enumValueMap[lexer.UTF16ToString(str.Value)]; ok {
								return ast.Expr{expr.Loc, &ast.ENumber{number}}, exprOut{}
							}
						}
					}
				}

				// "a['b']" => "a.b"
				if p.mangleSyntax {
					if lexer.IsIdentifierUTF16(str.Value) {
						return p.maybeRewriteDot(expr.Loc, &ast.EDot{
							Target:  e.Target,
							Name:    lexer.UTF16ToString(str.Value),
							NameLoc: e.Index.Loc,
						}), exprOut{}
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

		case ast.UnOpDelete:
			value, out := p.visitExprInOut(e.Value, exprIn{hasChainParent: true, isAssignTarget: true})
			e.Value = value

			// Lower optional chaining if present since we're guaranteed to be the
			// end of the chain
			if out.childContainsOptionalChain {
				return p.lowerOptionalChain(expr, in, out, nil)
			}

			return expr, exprOut{}
		}

		e.Value, _ = p.visitExprInOut(e.Value, exprIn{isAssignTarget: e.Op.IsUnaryUpdate()})

		// Fold constants
		switch e.Op {
		case ast.UnOpNot:
			if boolean, ok := toBooleanWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{!boolean}}, exprOut{}
			}

		case ast.UnOpVoid:
			if hasNoSideEffects(e.Value.Data) {
				return ast.Expr{expr.Loc, &ast.EUndefined{}}, exprOut{}
			}

		case ast.UnOpTypeof:
			// "typeof require" => "'function'"
			if id, ok := e.Value.Data.(*ast.EIdentifier); ok && id.Ref == p.requireRef {
				p.ignoreUsage(p.requireRef)
				return ast.Expr{expr.Loc, &ast.EString{lexer.StringToUTF16("function")}}, exprOut{}
			}

			if typeof, ok := typeofWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.EString{lexer.StringToUTF16(typeof)}}, exprOut{}
			}

		case ast.UnOpPos:
			if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.ENumber{number}}, exprOut{}
			}

		case ast.UnOpNeg:
			if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.ENumber{-number}}, exprOut{}
			}
		}

	case *ast.EDot:
		// Check both user-specified defines and known globals
		if defines, ok := p.processedDefines.DotDefines[e.Name]; ok {
			for _, define := range defines {
				if p.isDotDefineMatch(expr, define.Parts) {
					if define.CanBeRemovedIfUnused {
						// This expression matches our whitelist of side-effect-free properties
						e.CanBeRemovedIfUnused = true
						break
					}

					// Substitute user-specified defines
					return p.valueForDefine(expr.Loc, define.DefineFunc), exprOut{}
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

		return p.maybeRewriteDot(expr.Loc, e), exprOut{
			childContainsOptionalChain: containsOptionalChain,
		}

	case *ast.EIf:
		e.Test = p.visitBooleanExpr(e.Test)
		e.Yes = p.visitExpr(e.Yes)
		e.No = p.visitExpr(e.No)

		// Fold constants
		if boolean, ok := toBooleanWithoutSideEffects(e.Test.Data); ok {
			if boolean {
				return e.Yes, exprOut{}
			} else {
				return e.No, exprOut{}
			}
		}

		if p.mangleSyntax {
			// "!a ? b : c" => "a ? c : b"
			if not, ok := e.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				e.Test = not.Value
				e.Yes, e.No = e.No, e.Yes
			}
		}

	case *ast.EAwait:
		e.Value = p.visitExpr(e.Value)

	case *ast.EYield:
		if e.Value != nil {
			*e.Value = p.visitExpr(*e.Value)
		}

	case *ast.EArray:
		for i, item := range e.Items {
			e.Items[i], _ = p.visitExprInOut(item, exprIn{isAssignTarget: in.isAssignTarget})
		}

	case *ast.EObject:
		for i, property := range e.Properties {
			if property.Kind != ast.PropertySpread {
				e.Properties[i].Key = p.visitExpr(property.Key)
			}
			if property.Value != nil {
				*property.Value, _ = p.visitExprInOut(*property.Value, exprIn{isAssignTarget: in.isAssignTarget})
			}
			if property.Initializer != nil {
				*property.Initializer = p.visitExpr(*property.Initializer)
			}
		}
		return p.lowerObjectSpread(expr.Loc, e), exprOut{}

	case *ast.EImport:
		e.Expr = p.visitExpr(e.Expr)

		// Convert no-substitution template literals into strings
		if template, ok := e.Expr.Data.(*ast.ETemplate); ok && template.Tag == nil && len(template.Parts) == 0 {
			e.Expr.Data = &ast.EString{template.Head}
		}

		// The argument must be a string
		str, ok := e.Expr.Data.(*ast.EString)
		if !ok {
			if p.isBundling {
				p.log.AddError(p.source, e.Expr.Loc, "The argument to import() must be a string literal")
			}
			return expr, exprOut{}
		}

		// Ignore calls to import() if the control flow is provably dead here.
		// We don't want to spend time scanning the required files if they will
		// never be used.
		if p.isControlFlowDead {
			return ast.Expr{expr.Loc, &ast.ENull{}}, exprOut{}
		}

		path := ast.Path{
			Loc:  e.Expr.Loc,
			Text: lexer.UTF16ToString(str.Value),
		}

		p.importPaths = append(p.importPaths, ast.ImportPath{Path: path, Kind: ast.ImportDynamic})

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
		for i, arg := range e.Args {
			e.Args[i] = p.visitExpr(arg)
		}

		// Detect if this is a direct eval. Note that "(1 ? eval : 0)(x)" will
		// become "eval(x)" after we visit the target due to dead code elimination,
		// but that doesn't mean it should become a direct eval.
		if wasIdentifierBeforeVisit {
			if id, ok := e.Target.Data.(*ast.EIdentifier); ok {
				if symbol := p.symbols[id.Ref.InnerIndex]; symbol.Name == "eval" {
					e.IsDirectEval = true

					// Mark this scope and all parent scopes as containing a direct eval.
					// This will prevent us from renaming any symbols.
					for s := p.currentScope; s != nil; s = s.Parent {
						s.ContainsDirectEval = true
					}
				}
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

		// Track calls to require() so we can use them while bundling
		if id, ok := e.Target.Data.(*ast.EIdentifier); ok && id.Ref == p.requireRef && p.isBundling {
			// There must be one argument
			if len(e.Args) != 1 {
				p.log.AddError(p.source, expr.Loc,
					fmt.Sprintf("Calls to %s() must take a single argument", p.symbols[id.Ref.InnerIndex].Name))
				return expr, exprOut{}
			}
			arg := e.Args[0]

			// Convert no-substitution template literals into strings when bundling
			if template, ok := arg.Data.(*ast.ETemplate); ok && template.Tag == nil && len(template.Parts) == 0 {
				arg.Data = &ast.EString{template.Head}
			}

			// The argument must be a string
			str, ok := arg.Data.(*ast.EString)
			if !ok {
				p.log.AddError(p.source, arg.Loc,
					fmt.Sprintf("The argument to %s() must be a string literal", p.symbols[id.Ref.InnerIndex].Name))
				return expr, exprOut{}
			}

			// Ignore calls to require() if the control flow is provably dead here.
			// We don't want to spend time scanning the required files if they will
			// never be used.
			if p.isControlFlowDead {
				return ast.Expr{expr.Loc, &ast.ENull{}}, exprOut{}
			}

			path := ast.Path{
				Loc:  arg.Loc,
				Text: lexer.UTF16ToString(str.Value),
			}

			p.importPaths = append(p.importPaths, ast.ImportPath{Path: path, Kind: ast.ImportRequire})

			// Create a new expression to represent the operation
			p.ignoreUsage(p.requireRef)
			return ast.Expr{expr.Loc, &ast.ERequire{Path: path}}, exprOut{}
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
		oldTryBodyCount := p.tryBodyCount
		p.tryBodyCount = 0

		p.pushScopeForVisitPass(ast.ScopeFunctionArgs, expr.Loc)
		p.visitArgs(e.Args)
		p.pushScopeForVisitPass(ast.ScopeFunctionBody, e.Body.Loc)
		e.Body.Stmts = p.visitStmtsAndPrependTempRefs(e.Body.Stmts)
		p.popScope()
		p.popScope()

		if p.mangleSyntax && len(e.Body.Stmts) == 1 {
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

		p.tryBodyCount = oldTryBodyCount

	case *ast.EFunction:
		p.visitFn(&e.Fn, expr.Loc)

	case *ast.EClass:
		if e.Class.Name != nil {
			p.pushScopeForVisitPass(ast.ScopeClassName, expr.Loc)
		}
		p.visitClass(&e.Class)
		if e.Class.Name != nil {
			p.popScope()
		}

		// Lower class field syntax for browsers that don't support it
		_, newExpr := p.lowerClass(expr.Loc, &e.Class, false /* isStmt */)
		if newExpr.Data != nil {
			expr = newExpr
		}

	default:
		panic(fmt.Sprintf("Unexpected expression of type %T", expr.Data))
	}

	return expr, exprOut{}
}

func (p *parser) valueForDefine(loc ast.Loc, defineFunc DefineFunc) ast.Expr {
	expr := ast.Expr{loc, defineFunc(p.findSymbolHelper)}
	if id, ok := expr.Data.(*ast.EIdentifier); ok {
		return p.handleIdentifier(loc, id)
	}
	return expr
}

func (p *parser) handleIdentifier(loc ast.Loc, e *ast.EIdentifier) ast.Expr {
	// Substitute an EImportIdentifier now if this is an import item
	if p.isImportItem[e.Ref] {
		return ast.Expr{loc, &ast.EImportIdentifier{e.Ref}}
	}

	// Substitute a namespace export reference now if appropriate
	if p.ts.Parse {
		if nsRef, ok := p.isExportedInsideNamespace[e.Ref]; ok {
			name := p.symbols[e.Ref.InnerIndex].Name

			// If this is a known enum value, inline the value of the enum
			if enumValueMap, ok := p.knownEnumValues[nsRef]; ok {
				if number, ok := enumValueMap[name]; ok {
					return ast.Expr{loc, &ast.ENumber{number}}
				}
			}

			// Otherwise, create a property access on the namespace
			return ast.Expr{loc, &ast.EDot{
				Target:  ast.Expr{loc, &ast.EIdentifier{nsRef}},
				Name:    name,
				NameLoc: loc,
			}}
		}
	}

	// Disallow capturing the "require" variable without calling it
	if e.Ref == p.requireRef && (e != p.callTarget && e != p.typeofTarget) {
		if p.tryBodyCount == 0 {
			r := lexer.RangeOfIdentifier(p.source, loc)
			p.log.AddRangeError(p.source, r, "\"require\" must not be called indirectly")
		} else {
			// The "moment" library contains code that looks like this:
			//
			//   try {
			//     oldLocale = globalLocale._abbr;
			//     var aliasedRequire = require;
			//     aliasedRequire('./locale/' + name);
			//     getSetGlobalLocale(oldLocale);
			//   } catch (e) {}
			//
			// This is unfortunate because it prevents the module graph from being
			// statically determined. However, the dependencies are optional and
			// the library will work fine without them.
			//
			// Handle this case by deliberately ignoring code that uses require
			// incorrectly inside a try statement like this. We replace it with
			// null so it's guaranteed to crash at runtime.
			return ast.Expr{loc, &ast.ENull{}}
		}
	}

	return ast.Expr{loc, e}
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
	oldTryBodyCount := p.tryBodyCount
	oldIsThisCaptured := p.isThisCaptured
	p.tryBodyCount = 0
	p.isThisCaptured = true

	p.pushScopeForVisitPass(ast.ScopeFunctionArgs, scopeLoc)
	p.visitArgs(fn.Args)
	p.pushScopeForVisitPass(ast.ScopeFunctionBody, fn.Body.Loc)
	fn.Body.Stmts = p.visitStmtsAndPrependTempRefs(fn.Body.Stmts)
	p.popScope()
	p.popScope()

	p.tryBodyCount = oldTryBodyCount
	p.isThisCaptured = oldIsThisCaptured
}

func (p *parser) scanForImportsAndExports(stmts []ast.Stmt, isBundling bool) []ast.Stmt {
	stmtsEnd := 0

	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *ast.SImport:
			// TypeScript always trims unused imports. This is important for
			// correctness since some imports might be fake (only in the type
			// system and used for type-only imports).
			if p.mangleSyntax || p.ts.Parse {
				foundImports := false
				isUnusedInTypeScript := true

				// Remove the default name if it's unused
				if s.DefaultName != nil {
					foundImports = true
					symbol := p.symbols[s.DefaultName.Ref.InnerIndex]

					// TypeScript has a separate definition of unused
					if p.ts.Parse && p.tsUseCounts[s.DefaultName.Ref.InnerIndex] != 0 {
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
					if p.ts.Parse && p.tsUseCounts[s.NamespaceRef.InnerIndex] != 0 {
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
						if p.ts.Parse && p.tsUseCounts[item.Name.Ref.InnerIndex] != 0 {
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
				if p.ts.Parse && foundImports && isUnusedInTypeScript {
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
						for name, item := range importItems {
							*items = append(*items, ast.ClauseItem{name, item.Loc, item})
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
						Alias:        "default",
						AliasLoc:     s.DefaultName.Loc,
						ImportPath:   s.Path,
						NamespaceRef: s.NamespaceRef,
					}
				}

				if s.StarNameLoc != nil {
					p.namedImports[s.NamespaceRef] = ast.NamedImport{
						Alias:        "*",
						AliasLoc:     *s.StarNameLoc,
						ImportPath:   s.Path,
						NamespaceRef: ast.InvalidRef,
					}
				}

				if s.Items != nil {
					for _, item := range *s.Items {
						p.namedImports[item.Name.Ref] = ast.NamedImport{
							Alias:        item.Alias,
							AliasLoc:     item.AliasLoc,
							ImportPath:   s.Path,
							NamespaceRef: s.NamespaceRef,
						}
					}
				}
			}

			p.importPaths = append(p.importPaths, ast.ImportPath{
				Path: s.Path,

				// This is true for import statements without imports like "import 'foo'"
				DoesNotUseExports: s.DefaultName == nil && s.StarNameLoc == nil && s.Items == nil,
			})

		case *ast.SExportStar:
			// Only track import paths if we want dependencies
			if isBundling {
				p.importPaths = append(p.importPaths, ast.ImportPath{Path: s.Path})

				if s.Item != nil {
					// "export * as ns from 'path'"
					p.namedImports[s.Item.Name.Ref] = ast.NamedImport{
						Alias:        "*",
						AliasLoc:     s.Item.Name.Loc,
						ImportPath:   s.Path,
						NamespaceRef: ast.InvalidRef,
					}
				} else {
					// "export * from 'path'"
					p.exportStars = append(p.exportStars, s.Path)
				}
			}

		case *ast.SExportFrom:
			// Only track import paths if we want dependencies
			if isBundling {
				p.importPaths = append(p.importPaths, ast.ImportPath{Path: s.Path})

				for _, item := range s.Items {
					// Note that the imported alias is not item.Alias, which is the
					// exported alias. This is somewhat confusing because each
					// SExportFrom statement is basically SImport + SExportClause in one.
					p.namedImports[item.Name.Ref] = ast.NamedImport{
						Alias:        p.symbols[item.Name.Ref.InnerIndex].Name,
						AliasLoc:     item.Name.Loc,
						ImportPath:   s.Path,
						NamespaceRef: s.NamespaceRef,
						IsExported:   true,
					}
				}
			}

		case *ast.SExportClause:
			// Strip exports of non-local symbols in TypeScript, since those likely
			// correspond to type-only exports
			if p.ts.Parse {
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
	p.useCountEstimates = make(map[ast.Ref]uint32)
	p.declaredSymbols = nil
	p.importPaths = nil
	part := ast.Part{
		Stmts:             p.visitStmtsAndPrependTempRefs(stmts),
		UseCountEstimates: p.useCountEstimates,
	}
	if len(part.Stmts) > 0 {
		part.CanBeRemovedIfUnused = p.stmtsCanBeRemovedIfUnused(part.Stmts)
		part.DeclaredSymbols = p.declaredSymbols
		part.ImportPaths = p.importPaths
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
	case *ast.ENull, *ast.EUndefined, *ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.EString, *ast.EThis, *ast.ERegExp, *ast.EFunction, *ast.EArrow:
		return true

	case *ast.EDot:
		return e.CanBeRemovedIfUnused

	case *ast.EClass:
		return p.classCanBeRemovedIfUnused(e.Class)

	case *ast.EIdentifier:
		symbol := p.symbols[e.Ref.InnerIndex]
		if symbol.Kind != ast.SymbolUnbound {
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
			if property.Kind != ast.PropertySpread && !p.exprCanBeRemovedIfUnused(property.Key) {
				return false
			}
			if property.Value != nil && !p.exprCanBeRemovedIfUnused(*property.Value) {
				return false
			}
		}
		return true
	}

	// Assume all other expression types have side effects and cannot be removed
	return false
}

type LanguageTarget int8

const (
	// These are arranged such that ESNext is the default zero value and such
	// that earlier releases are less than later releases
	ES2015 = -6
	ES2016 = -5
	ES2017 = -4
	ES2018 = -3
	ES2019 = -2
	ES2020 = -1
	ESNext = 0
)

var targetTable = map[LanguageTarget]string{
	ES2015: "ES2015",
	ES2016: "ES2016",
	ES2017: "ES2017",
	ES2018: "ES2018",
	ES2019: "ES2019",
	ES2020: "ES2020",
	ESNext: "ESNext",
}

type JSXOptions struct {
	Parse    bool
	Factory  []string
	Fragment []string
}

type TypeScriptOptions struct {
	Parse bool
}

type ParseOptions struct {
	// true: imports are scanned and bundled along with the file
	// false: imports are left alone and the file is passed through as-is
	IsBundling bool

	Defines      *ProcessedDefines
	MangleSyntax bool
	TS           TypeScriptOptions
	JSX          JSXOptions
	Target       LanguageTarget
}

func newParser(log logging.Log, source logging.Source, lexer lexer.Lexer, options ParseOptions) *parser {
	if options.Defines == nil {
		defaultDefines := ProcessDefines(nil)
		options.Defines = &defaultDefines
	}

	p := &parser{
		log:               log,
		source:            source,
		lexer:             lexer,
		allowIn:           true,
		target:            options.Target,
		ts:                options.TS,
		jsx:               options.JSX,
		mangleSyntax:      options.MangleSyntax,
		isBundling:        options.IsBundling,
		processedDefines:  *options.Defines,
		currentFnOpts:     fnOpts{isOutsideFn: true},
		useCountEstimates: make(map[ast.Ref]uint32),
		runtimeImports:    make(map[string]ast.Ref),

		// These are for TypeScript
		emittedNamespaceVars:      make(map[ast.Ref]bool),
		isExportedInsideNamespace: make(map[ast.Ref]ast.Ref),
		knownEnumValues:           make(map[ast.Ref]map[string]float64),

		// These are for handling ES6 imports and exports
		importItemsForNamespace: make(map[ast.Ref]map[string]ast.LocRef),
		isImportItem:            make(map[ast.Ref]bool),
		namedImports:            make(map[ast.Ref]ast.NamedImport),
		namedExports:            make(map[string]ast.Ref),
	}

	p.findSymbolHelper = func(name string) ast.Ref { return p.findSymbol(name).ref }
	p.pushScopeForParsePass(ast.ScopeEntry, ast.Loc{locModuleScope})

	// The bundler pre-declares these symbols
	p.exportsRef = p.newSymbol(ast.SymbolHoisted, "exports")
	p.requireRef = p.newSymbol(ast.SymbolHoisted, "require")
	p.moduleRef = p.newSymbol(ast.SymbolHoisted, "module")

	// Only declare these symbols if we're bundling
	if options.IsBundling {
		p.currentScope.Members["exports"] = p.exportsRef
		p.currentScope.Members["require"] = p.requireRef
		p.currentScope.Members["module"] = p.moduleRef
	}

	return p
}

func Parse(log logging.Log, source logging.Source, options ParseOptions) (result ast.AST, ok bool) {
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

	p := newParser(log, source, lexer.NewLexer(log, source), options)

	// Consume a leading hashbang comment
	hashbang := ""
	if p.lexer.Token == lexer.THashbang {
		hashbang = p.lexer.Identifier
		p.lexer.Next()
	}

	// Parse the file in the first pass, but do not bind symbols
	stmts := p.parseStmtsUpTo(lexer.TEndOfFile, parseStmtOpts{isModuleScope: true})
	p.prepareForVisitPass()

	// Bind symbols in a second pass over the AST. I started off doing this in a
	// single pass, but it turns out it's pretty much impossible to do this
	// correctly while handling arrow functions because of the grammar
	// ambiguities.
	parts := []ast.Part{}
	if !p.isBundling {
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
					parts = p.appendPart(parts, []ast.Stmt{ast.Stmt{stmt.Loc, &clone}})
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

	// Insert an import statement for any runtime imports we generated
	if len(p.runtimeImports) > 0 {
		// Sort the imports for determinism
		keys := make([]string, 0, len(p.runtimeImports))
		for key, _ := range p.runtimeImports {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		namespaceRef := p.newSymbol(ast.SymbolOther, "runtime")
		p.moduleScope.Generated = append(p.moduleScope.Generated, namespaceRef)
		declaredSymbols := make([]ast.DeclaredSymbol, len(keys))
		clauseItems := make([]ast.ClauseItem, len(keys))
		runtimePath := ast.Path{
			UseSourceIndex: true,
			SourceIndex:    ast.RuntimeSourceIndex,
		}

		// Create per-import information
		for i, key := range keys {
			ref := p.runtimeImports[key]
			declaredSymbols[i] = ast.DeclaredSymbol{Ref: ref, IsTopLevel: true}
			clauseItems[i] = ast.ClauseItem{Alias: key, Name: ast.LocRef{Ref: ref}}
			p.namedImports[ref] = ast.NamedImport{
				Alias:        key,
				ImportPath:   runtimePath,
				NamespaceRef: namespaceRef,
			}
		}

		// Append a single import to the end of the file (ES6 imports are hoisted
		// so we don't need to worry about where the import statement goes)
		parts = append(parts, ast.Part{
			DeclaredSymbols: declaredSymbols,
			ImportPaths: []ast.ImportPath{ast.ImportPath{
				Kind: ast.ImportStmt,
				Path: runtimePath,
			}},
			Stmts: []ast.Stmt{ast.Stmt{ast.Loc{}, &ast.SImport{
				NamespaceRef: namespaceRef,
				Items:        &clauseItems,
				Path:         runtimePath,
			}}},
		})
	}

	// Handle import paths after the whole file has been visited because we need
	// symbol usage counts to be able to remove unused type-only imports in
	// TypeScript code.
	partsEnd := 0
	for _, part := range parts {
		p.importPaths = []ast.ImportPath{}
		part.Stmts = p.scanForImportsAndExports(part.Stmts, options.IsBundling)
		part.ImportPaths = append(part.ImportPaths, p.importPaths...)
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
			for ref, _ := range part.UseCountEstimates {
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

	result = p.toAST(source, parts, hashbang)
	result.WasTypeScript = options.TS.Parse
	return
}

func ModuleExportsAST(log logging.Log, source logging.Source, options ParseOptions, expr ast.Expr) ast.AST {
	// Don't create a new lexer using lexer.NewLexer() here since that will
	// actually attempt to parse the first token, which might cause a syntax
	// error.
	p := newParser(log, source, lexer.Lexer{}, options)
	p.prepareForVisitPass()

	// Make a symbol map that contains our file's symbols
	symbols := ast.SymbolMap{make([][]ast.Symbol, source.Index+1)}
	symbols.Outer[source.Index] = p.symbols

	// "module.exports = [expr]"
	stmt := ast.Stmt{expr.Loc, &ast.SExpr{ast.Expr{expr.Loc, &ast.EBinary{
		ast.BinOpAssign,
		ast.Expr{expr.Loc, &ast.EDot{
			Target:  ast.Expr{expr.Loc, &ast.EIdentifier{p.moduleRef}},
			Name:    "exports",
			NameLoc: expr.Loc,
		}},
		expr,
	}}}}

	// Mark that we used the "module" variable
	p.symbols[p.moduleRef.InnerIndex].UseCountEstimate++

	return p.toAST(source, []ast.Part{ast.Part{Stmts: []ast.Stmt{stmt}}}, "")
}

func (p *parser) prepareForVisitPass() {
	p.pushScopeForVisitPass(ast.ScopeEntry, ast.Loc{locModuleScope})
	p.moduleScope = p.currentScope
}

func (p *parser) toAST(source logging.Source, parts []ast.Part, hashbang string) ast.AST {
	// Make a wrapper symbol in case we need to be wrapped in a closure
	wrapperRef := p.newSymbol(ast.SymbolOther, "require_"+
		ast.GenerateNonUniqueNameFromPath(p.source.AbsolutePath))

	// Make a symbol map that contains our file's symbols
	symbols := ast.NewSymbolMap(int(source.Index) + 1)
	symbols.Outer[source.Index] = p.symbols

	return ast.AST{
		Parts:                 parts,
		ModuleScope:           p.moduleScope,
		Symbols:               symbols,
		ExportsRef:            p.exportsRef,
		ModuleRef:             p.moduleRef,
		WrapperRef:            wrapperRef,
		Hashbang:              hashbang,
		NamedImports:          p.namedImports,
		NamedExports:          p.namedExports,
		TopLevelSymbolToParts: p.topLevelSymbolToParts,
		ExportStars:           p.exportStars,

		// CommonJS features
		HasTopLevelReturn: p.hasTopLevelReturn,
		UsesExportsRef:    p.symbols[p.exportsRef.InnerIndex].UseCountEstimate > 0,
		UsesModuleRef:     p.symbols[p.moduleRef.InnerIndex].UseCountEstimate > 0,

		// ES6 features
		HasES6Imports: p.hasES6ImportSyntax,
		HasES6Exports: p.hasES6ExportSyntax,
	}
}
