package snap_renamer

import (
	"fmt"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/renamer"
)

type Replacement struct {
	Original string
	Replaced string
}

type NamedReference struct {
	UnreplacedLocs []int
	Replace        *Replacement
}

type SnapRenamer struct {
	symbols             js_ast.SymbolMap
	deferredIdentifiers map[js_ast.Ref]Replacement
	wrappedRenamer      *renamer.Renamer
	NamedReferences     map[js_ast.Ref]*NamedReference
	CurrentPrinterIndex func() int
}

type nameForSymbolOpts struct {
	allowReplaceWithDeferr bool
	isRewriting            bool
}

var DefaultNameForSymbolOpts = nameForSymbolOpts{
	allowReplaceWithDeferr: true,
	isRewriting:            false,
}

var NoDeferNameForSymbolOpts = nameForSymbolOpts{
	allowReplaceWithDeferr: false,
	isRewriting:            false,
}

var RewritingNameForSymbolOpts = nameForSymbolOpts{
	allowReplaceWithDeferr: true,
	isRewriting:            true,
}

func NewSnapRenamer(symbols js_ast.SymbolMap) SnapRenamer {
	return SnapRenamer{
		symbols:             symbols,
		deferredIdentifiers: make(map[js_ast.Ref]Replacement),
		NamedReferences:     make(map[js_ast.Ref]*NamedReference),
	}
}

// The linking process prepares a NumberRenamer which includes `names` and a symbol map
// mostly related to the code wrapping each module.
// In order to correctly determine symbol names we store a reference here and forward
// symbol resolves to it @see `NameForSymbol`.
func WrapRenamer(r *renamer.Renamer, symbols js_ast.SymbolMap) SnapRenamer {
	return SnapRenamer{
		symbols:             symbols,
		deferredIdentifiers: make(map[js_ast.Ref]Replacement),
		wrappedRenamer:      r,
		NamedReferences:     make(map[js_ast.Ref]*NamedReference),
	}
}

func (r *SnapRenamer) resolveRefFromSymbols(ref js_ast.Ref) js_ast.Ref {
	return js_ast.FollowSymbols(r.symbols, ref)
}

func (r *SnapRenamer) NameForSymbol(ref js_ast.Ref) string {
	return r.SnapNameForSymbol(ref, &DefaultNameForSymbolOpts)
}

func (r *SnapRenamer) SnapNameForSymbol(
	ref js_ast.Ref, opts *nameForSymbolOpts) string {

	ref = r.resolveRefFromSymbols(ref)

	if !opts.isRewriting && opts.allowReplaceWithDeferr && r.canCaptureNameLocs() && !r.HasBeenReplaced(ref) {
		res, ok := r.NamedReferences[ref]
		if !ok {
			res = &NamedReference{
				UnreplacedLocs: []int{},
				Replace:        nil,
			}
			r.NamedReferences[ref] = res
		}
		res.UnreplacedLocs = append(res.UnreplacedLocs, r.CurrentPrinterIndex())
	}

	if opts.allowReplaceWithDeferr {
		deferredIdentifier, ok := r.deferredIdentifiers[ref]
		if ok {
			return deferredIdentifier.Replaced
		}
	}
	if r.wrappedRenamer != nil {
		return (*r.wrappedRenamer).NameForSymbol(ref)
	}
	name := r.symbols.Get(ref).OriginalName
	return name
}

// Stores a Replacement string for accesses to the given ref that is used when
// @see NameForSymbol is called later.
// The Replacement is a function call, i.e. `__get_a__()` which will be printed
// in place of the Original var, i.e. `a`.
func (r *SnapRenamer) Replace(ref js_ast.Ref, replaceWith string) {
	ref = r.resolveRefFromSymbols(ref)

	// Prevent replacing the Replacement which results in double wrapping
	if r.HasBeenReplaced(ref) {
		return
	}
	res, hasBeenNamed := r.NamedReferences[ref]

	replace := Replacement{
		Original: r.SnapNameForSymbol(ref, &RewritingNameForSymbolOpts),
		Replaced: replaceWith,
	}

	if hasBeenNamed {
		res.Replace = &replace
		fmt.Printf("Named: %v %v \n", *res.Replace, res.UnreplacedLocs)
	}
	r.deferredIdentifiers[ref] = replace
}

// Returns `true` if a Replacement was registered for the given ref
func (r *SnapRenamer) HasBeenReplaced(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	_, ok := r.deferredIdentifiers[ref]
	return ok
}

func (r *SnapRenamer) IsLegalGlobal(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	return symbol.OriginalName == "Object"
}

// Returns the Original id of the ref whose id has been Replaced before.
// This function panics if no Replacement is found for this ref.
func (r *SnapRenamer) GetOriginalId(ref js_ast.Ref) string {
	ref = r.resolveRefFromSymbols(ref)
	replacement, ok := r.deferredIdentifiers[ref]
	if !ok {
		panic("Should only ask for Original ids for the ones that were Replaced")
	}
	return replacement.Original
}

func (r *SnapRenamer) IsUnbound(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	if symbol.Kind == js_ast.SymbolUnbound {
		return true
	} else {
		return false
	}
}

// When printing runtime code the renamer isn't initialized to collect named locs
func (r *SnapRenamer) canCaptureNameLocs() bool {
	return r.NamedReferences != nil && r.CurrentPrinterIndex != nil
}

func (r *SnapRenamer) IsUnwrappable(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	if symbol.Kind == js_ast.SymbolUnbound {
		return true
	}
	return r.isExportSymbol(symbol)
}

func (r *SnapRenamer) isExportSymbol(symbol *js_ast.Symbol) bool {
	matchesKind := symbol.Kind == js_ast.SymbolHoisted ||
		symbol.Kind == js_ast.SymbolUnbound
	return matchesKind && symbol.OriginalName == "exports"
}

func (r *SnapRenamer) IsExport(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	return r.isExportSymbol(symbol)
}

func (r *SnapRenamer) isModuleSymbol(symbol *js_ast.Symbol) bool {
	matchesKind := symbol.Kind == js_ast.SymbolHoisted ||
		symbol.Kind == js_ast.SymbolUnbound
	return matchesKind && symbol.OriginalName == "module"
}

func (r *SnapRenamer) IsModule(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	return r.isModuleSymbol(symbol)
}

// TODO(thlorenz): Include more from
// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects
var VALID_GLOBALS = []string{
	"require", "Object",
}

func (r *SnapRenamer) GlobalNeedsDefer(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	if symbol.Kind == js_ast.SymbolUnbound {
		for _, v := range VALID_GLOBALS {
			if v == symbol.OriginalName {
				return false
			}
		}
		return true
	} else {
		return false
	}
}
