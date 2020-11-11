package snap_renamer

import (
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/renamer"
)

type replacement struct {
	original string
	replaced string
}

type SnapRenamer struct {
	symbols             js_ast.SymbolMap
	deferredIdentifiers map[js_ast.Ref]replacement
	wrappedRenamer      *renamer.Renamer
}

func NewSnapRenamer(symbols js_ast.SymbolMap) SnapRenamer {
	return SnapRenamer{
		symbols:             symbols,
		deferredIdentifiers: make(map[js_ast.Ref]replacement),
	}
}

// The linking process prepares a NumberRenamer which includes `names` and a symbol map
// mostly related to the code wrapping each module.
// In order to correctly determine symbol names we store a reference here and forward
// symbol resolves to it @see `NameForSymbol`.
func WrapRenamer(r *renamer.Renamer, symbols js_ast.SymbolMap) SnapRenamer {
	return SnapRenamer{
		symbols:             symbols,
		deferredIdentifiers: make(map[js_ast.Ref]replacement),
		wrappedRenamer:      r,
	}
}

func (r *SnapRenamer) resolveRefFromSymbols(ref js_ast.Ref) js_ast.Ref {
	return js_ast.FollowSymbols(r.symbols, ref)
}

func (r *SnapRenamer) NameForSymbol(ref js_ast.Ref) string {
	return r.SnapNameForSymbol(ref, true)
}

func (r *SnapRenamer) SnapNameForSymbol(
	ref js_ast.Ref,
	allowReplaceWithDeferr bool) string {
	ref = r.resolveRefFromSymbols(ref)
	if allowReplaceWithDeferr {
		deferredIdentifier, ok := r.deferredIdentifiers[ref]
		if ok {
			return deferredIdentifier.replaced
		}
	}
	if r.wrappedRenamer != nil {
		return (*r.wrappedRenamer).NameForSymbol(ref)
	}
	name := r.symbols.Get(ref).OriginalName
	return name
}

// Stores a replacement string for accesses to the given ref that is used when
// @see NameForSymbol is called later.
// The replacement is a function call, i.e. `__get_a__()` which will be printed
// in place of the original var, i.e. `a`.
func (r *SnapRenamer) Replace(ref js_ast.Ref, replaceWith string) {
	ref = r.resolveRefFromSymbols(ref)
	original := r.NameForSymbol(ref)
	r.deferredIdentifiers[ref] = replacement{
		original: original,
		replaced: replaceWith,
	}
}

// Returns `true` if a replacement was registered for the given ref
func (r *SnapRenamer) HasBeenReplaced(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	_, ok := r.deferredIdentifiers[ref]
	return ok
}

// Returns the original id of the ref whose id has been replaced before.
// This function panics if no replacement is found for this ref.
func (r *SnapRenamer) GetOriginalId(ref js_ast.Ref) string {
	ref = r.resolveRefFromSymbols(ref)
	replacement, ok := r.deferredIdentifiers[ref]
	if !ok {
		panic("Should only ask for original ids for the ones that were replaced")
	}
	return replacement.original
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

func (r *SnapRenamer) IsUnboundNonRequire(ref js_ast.Ref) bool {
	ref = r.resolveRefFromSymbols(ref)
	symbol := r.symbols.Get(ref)
	if symbol.Kind == js_ast.SymbolUnbound {
		return symbol.OriginalName != "require"
	} else {
		return false
	}
}
