package bundler

import (
	"esbuild/ast"
	"esbuild/lexer"
	"sort"
	"strconv"
)

func numberToMinifiedName(i int) string {
	j := i % 54
	name := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$"[j : j+1]
	i = i / 54

	for i > 0 {
		i--
		j := i % 64
		name += "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$0123456789"[j : j+1]
		i = i / 64
	}

	return name
}

func reservedNames(moduleScopes []*ast.Scope, symbols *ast.SymbolMap) map[string]bool {
	names := make(map[string]bool)

	// All keywords are reserved names
	for k, _ := range lexer.Keywords() {
		names[k] = true
	}

	// All unbound symbols must be reserved names
	for _, scope := range moduleScopes {
		for _, ref := range scope.Members {
			symbol := symbols.Get(ref)
			if symbol.Kind == ast.SymbolUnbound {
				names[symbol.Name] = true
			}
		}
	}

	return names
}

func sortedSymbolsInScope(scope *ast.Scope) uint64Array {
	// Sort for determinism
	sorted := uint64Array(make([]uint64, 0, len(scope.Members)+len(scope.Generated)))
	for _, ref := range scope.Members {
		sorted = append(sorted, (uint64(ref.OuterIndex)<<32)|uint64(ref.InnerIndex))
	}
	for _, ref := range scope.Generated {
		sorted = append(sorted, (uint64(ref.OuterIndex)<<32)|uint64(ref.InnerIndex))
	}
	sort.Sort(sorted)
	return sorted
}

////////////////////////////////////////////////////////////////////////////////
// renameAllSymbols() implementation

type renamer struct {
	parent *renamer
	names  map[string]bool
}

func (r *renamer) isNameUsed(name string) bool {
	for {
		if _, ok := r.names[name]; ok {
			return true
		}
		r = r.parent
		if r == nil {
			return false
		}
	}
}

func (r *renamer) findUnusedName(prefix string) string {
	name := prefix
	tries := 1
	for {
		if !r.isNameUsed(name) {
			r.names[name] = true
			return name
		}
		tries++
		name = prefix + strconv.Itoa(tries)
	}
}

func renameAllSymbols(moduleScopes []*ast.Scope, symbols *ast.SymbolMap) {
	r := &renamer{nil, reservedNames(moduleScopes, symbols)}
	alreadyRenamed := make(map[ast.Ref]bool)

	// Rename top-level symbols across all files all at once since after
	// bundling, they will all be in the same scope
	for _, scope := range moduleScopes {
		r.renameSymbolsInScope(scope, symbols, alreadyRenamed)
	}

	// Symbols in child scopes may also have to be renamed to avoid conflicts
	for _, scope := range moduleScopes {
		for _, child := range scope.Children {
			r.renameAllSymbolsRecursive(child, symbols, alreadyRenamed)
		}
	}
}

func (r *renamer) renameSymbolsInScope(scope *ast.Scope, symbols *ast.SymbolMap, alreadyRenamed map[ast.Ref]bool) {
	sorted := sortedSymbolsInScope(scope)

	// Rename all symbols in this scope
	for _, i := range sorted {
		ref := ast.Ref{uint32(i >> 32), uint32(i)}
		ref = ast.FollowSymbols(symbols, ref)

		// Don't rename the same symbol more than once
		if alreadyRenamed[ref] {
			continue
		}
		alreadyRenamed[ref] = true

		symbol := symbols.Get(ref)

		// Don't rename unbound symbols
		if symbol.Kind == ast.SymbolUnbound {
			continue
		}

		symbol.Name = r.findUnusedName(symbol.Name)
		symbols.Set(ref, symbol)
	}
}

func (parent *renamer) renameAllSymbolsRecursive(scope *ast.Scope, symbols *ast.SymbolMap, alreadyRenamed map[ast.Ref]bool) {
	r := &renamer{parent, make(map[string]bool)}
	r.renameSymbolsInScope(scope, symbols, alreadyRenamed)

	// Symbols in child scopes may also have to be renamed to avoid conflicts
	for _, child := range scope.Children {
		r.renameAllSymbolsRecursive(child, symbols, alreadyRenamed)
	}
}

////////////////////////////////////////////////////////////////////////////////
// minifyAllSymbols() implementation

func minifyAllSymbols(moduleScopes []*ast.Scope, symbols *ast.SymbolMap) {
	g := minifyGroup{[]uint32{}, make(map[ast.Ref]uint32)}
	var next uint32 = 0

	// Allocate a slot for every symbol in each top-level scope. These slots must
	// not overlap between files because the bundler may smoosh everything
	// together into a single scope.
	for _, scope := range moduleScopes {
		next = g.countSymbolsInScope(scope, symbols, next)
	}

	// Allocate a slot for every symbol in each nested scope. Since it's
	// impossible for symbols from nested scopes to conflict, symbols from
	// different nested scopes can reuse the same slots (and therefore get the
	// same minified names).
	//
	// One good heuristic is to merge slots from different nested scopes using
	// sequential assignment. Then top-level function statements will always have
	// the same argument names, which is better for gzip compression.
	for _, scope := range moduleScopes {
		for _, child := range scope.Children {
			// Deliberately don't update "next" here. Sibling scopes can't collide and
			// so can reuse slots.
			g.countSymbolsRecursive(child, symbols, next, 0)
		}
	}

	// Sort slot indices descending by the count for that slot
	sorted := slotArray(make([]slot, len(g.slotToCount)))
	for index, count := range g.slotToCount {
		sorted[index] = slot{count, uint32(index)}
	}
	sort.Sort(sorted)

	reservedNames := reservedNames(moduleScopes, symbols)
	names := make([]string, len(sorted))
	nextName := 0

	// Assign names sequentially in order so the most frequent symbols get the
	// shortest names
	for _, slot := range sorted {
		name := numberToMinifiedName(nextName)
		nextName++

		// Make sure we never generate a reserved name
		for reservedNames[name] {
			name = numberToMinifiedName(nextName)
			nextName++
		}

		names[slot.index] = name
	}

	// Copy the names to the appropriate symbols
	for ref, i := range g.symbolToSlot {
		symbol := symbols.Get(ref)
		symbol.Name = names[i]
		symbols.Set(ref, symbol)
	}
}

type minifyGroup struct {
	slotToCount  []uint32
	symbolToSlot map[ast.Ref]uint32
}

func (g *minifyGroup) countSymbol(slot uint32, ref ast.Ref, count uint32) bool {
	// Don't double-count symbols that have already been counted
	if _, ok := g.symbolToSlot[ref]; ok {
		return false
	}

	// Optionally extend the slot array
	if slot == uint32(len(g.slotToCount)) {
		g.slotToCount = append(g.slotToCount, 0)
	}

	// Count this symbol in this slot
	g.slotToCount[slot] += count
	g.symbolToSlot[ref] = slot
	return true
}

func (g *minifyGroup) countSymbolsInScope(scope *ast.Scope, symbols *ast.SymbolMap, next uint32) uint32 {
	sorted := sortedSymbolsInScope(scope)

	for _, i := range sorted {
		ref := ast.Ref{uint32(i >> 32), uint32(i)}
		ref = ast.FollowSymbols(symbols, ref)

		symbol := symbols.Get(ref)

		// Only count bound symbols
		if symbol.Kind == ast.SymbolUnbound {
			continue
		}

		// Add 1 to the count to also include the declaration
		if g.countSymbol(next, ref, symbol.UseCountEstimate+1) {
			next += 1
		}
	}

	return next
}

func (g *minifyGroup) countSymbolsRecursive(scope *ast.Scope, symbols *ast.SymbolMap, next uint32, labelCount uint32) uint32 {
	next = g.countSymbolsInScope(scope, symbols, next)

	// Labels are in a separate namespace from symbols
	if scope.Kind == ast.ScopeLabel {
		symbol := symbols.Get(scope.ScopeRef)
		g.countSymbol(labelCount, scope.ScopeRef, symbol.UseCountEstimate+1)
		labelCount += 1
	}

	for _, child := range scope.Children {
		// Deliberately don't update "next" here. Sibling scopes can't collide and
		// so can reuse slots.
		g.countSymbolsRecursive(child, symbols, next, labelCount)
	}
	return next
}

type slot struct {
	count uint32
	index uint32
}

// These types are just so we can use Go's native sort function
type uint64Array []uint64
type slotArray []slot

func (a uint64Array) Len() int               { return len(a) }
func (a uint64Array) Swap(i int, j int)      { a[i], a[j] = a[j], a[i] }
func (a uint64Array) Less(i int, j int) bool { return a[i] < a[j] }

func (a slotArray) Len() int          { return len(a) }
func (a slotArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }
func (a slotArray) Less(i int, j int) bool {
	ai, aj := a[i], a[j]
	return ai.count > aj.count || (ai.count == aj.count && ai.index < aj.index)
}
