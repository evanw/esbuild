package renamer

import (
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
)

func ComputeReservedNames(moduleScopes []*js_ast.Scope, symbols ast.SymbolMap) map[string]uint32 {
	names := make(map[string]uint32)

	// All keywords and strict mode reserved words are reserved names
	for k := range js_lexer.Keywords {
		names[k] = 1
	}
	for k := range js_lexer.StrictModeReservedWords {
		names[k] = 1
	}

	// All unbound symbols must be reserved names
	for _, scope := range moduleScopes {
		computeReservedNamesForScope(scope, symbols, names)
	}

	return names
}

func computeReservedNamesForScope(scope *js_ast.Scope, symbols ast.SymbolMap, names map[string]uint32) {
	for _, member := range scope.Members {
		symbol := symbols.Get(member.Ref)
		if symbol.Kind == ast.SymbolUnbound || symbol.Flags.Has(ast.MustNotBeRenamed) {
			names[symbol.OriginalName] = 1
		}
	}
	for _, ref := range scope.Generated {
		symbol := symbols.Get(ref)
		if symbol.Kind == ast.SymbolUnbound || symbol.Flags.Has(ast.MustNotBeRenamed) {
			names[symbol.OriginalName] = 1
		}
	}

	// If there's a direct "eval" somewhere inside the current scope, continue
	// traversing down the scope tree until we find it to get all reserved names
	if scope.ContainsDirectEval {
		for _, child := range scope.Children {
			if child.ContainsDirectEval {
				computeReservedNamesForScope(child, symbols, names)
			}
		}
	}
}

type Renamer interface {
	NameForSymbol(ref ast.Ref) string
}

////////////////////////////////////////////////////////////////////////////////
// noOpRenamer

type noOpRenamer struct {
	symbols ast.SymbolMap
}

func NewNoOpRenamer(symbols ast.SymbolMap) Renamer {
	return &noOpRenamer{
		symbols: symbols,
	}
}

func (r *noOpRenamer) NameForSymbol(ref ast.Ref) string {
	ref = ast.FollowSymbols(r.symbols, ref)
	return r.symbols.Get(ref).OriginalName
}

////////////////////////////////////////////////////////////////////////////////
// MinifyRenamer

type symbolSlot struct {
	name               string
	count              uint32
	needsCapitalForJSX uint32 // This is really a bool but needs to be atomic
}

type MinifyRenamer struct {
	reservedNames        map[string]uint32
	slots                [4][]symbolSlot
	topLevelSymbolToSlot map[ast.Ref]uint32
	symbols              ast.SymbolMap
}

func NewMinifyRenamer(symbols ast.SymbolMap, firstTopLevelSlots ast.SlotCounts, reservedNames map[string]uint32) *MinifyRenamer {
	return &MinifyRenamer{
		symbols:       symbols,
		reservedNames: reservedNames,
		slots: [4][]symbolSlot{
			make([]symbolSlot, firstTopLevelSlots[0]),
			make([]symbolSlot, firstTopLevelSlots[1]),
			make([]symbolSlot, firstTopLevelSlots[2]),
			make([]symbolSlot, firstTopLevelSlots[3]),
		},
		topLevelSymbolToSlot: make(map[ast.Ref]uint32),
	}
}

func (r *MinifyRenamer) NameForSymbol(ref ast.Ref) string {
	// Follow links to get to the underlying symbol
	ref = ast.FollowSymbols(r.symbols, ref)
	symbol := r.symbols.Get(ref)

	// Skip this symbol if the name is pinned
	ns := symbol.SlotNamespace()
	if ns == ast.SlotMustNotBeRenamed {
		return symbol.OriginalName
	}

	// Check if it's a nested scope symbol
	i := symbol.NestedScopeSlot

	// If it's not (i.e. it's in a top-level scope), look up the slot
	if !i.IsValid() {
		index, ok := r.topLevelSymbolToSlot[ref]
		if !ok {
			// If we get here, then we're printing a symbol that never had any
			// recorded uses. This is odd but can happen in certain scenarios.
			// For example, code in a branch with dead control flow won't mark
			// any uses but may still be printed. In that case it doesn't matter
			// what name we use since it's dead code.
			return symbol.OriginalName
		}
		i = ast.MakeIndex32(index)
	}

	return r.slots[ns][i.GetIndex()].name
}

// The InnerIndex should be stable because the parser for a single file is
// single-threaded and deterministically assigns out InnerIndex values
// sequentially. But the SourceIndex should be unstable because the main thread
// assigns out source index values sequentially to newly-discovered dependencies
// in a multi-threaded producer/consumer relationship. So instead we use the
// index of the source in the DFS order over all entry points for stability.
type StableSymbolCount struct {
	StableSourceIndex uint32
	Ref               ast.Ref
	Count             uint32
}

// This type is just so we can use Go's native sort function
type StableSymbolCountArray []StableSymbolCount

func (a StableSymbolCountArray) Len() int          { return len(a) }
func (a StableSymbolCountArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a StableSymbolCountArray) Less(i int, j int) bool {
	ai, aj := a[i], a[j]
	if ai.Count > aj.Count {
		return true
	}
	if ai.Count < aj.Count {
		return false
	}
	if ai.StableSourceIndex < aj.StableSourceIndex {
		return true
	}
	if ai.StableSourceIndex > aj.StableSourceIndex {
		return false
	}
	return ai.Ref.InnerIndex < aj.Ref.InnerIndex
}

func (r *MinifyRenamer) AccumulateSymbolUseCounts(
	topLevelSymbols *StableSymbolCountArray,
	symbolUses map[ast.Ref]js_ast.SymbolUse,
	stableSourceIndices []uint32,
) {
	// NOTE: This function is run in parallel. Make sure to avoid data races.

	for ref, use := range symbolUses {
		r.AccumulateSymbolCount(topLevelSymbols, ref, use.CountEstimate, stableSourceIndices)
	}
}

func (r *MinifyRenamer) AccumulateSymbolCount(
	topLevelSymbols *StableSymbolCountArray,
	ref ast.Ref,
	count uint32,
	stableSourceIndices []uint32,
) {
	// NOTE: This function is run in parallel. Make sure to avoid data races.

	// Follow links to get to the underlying symbol
	ref = ast.FollowSymbols(r.symbols, ref)
	symbol := r.symbols.Get(ref)
	for symbol.NamespaceAlias != nil {
		ref = ast.FollowSymbols(r.symbols, symbol.NamespaceAlias.NamespaceRef)
		symbol = r.symbols.Get(ref)
	}

	// Skip this symbol if the name is pinned
	ns := symbol.SlotNamespace()
	if ns == ast.SlotMustNotBeRenamed {
		return
	}

	// Check if it's a nested scope symbol
	if i := symbol.NestedScopeSlot; i.IsValid() {
		// If it is, accumulate the count using a parallel-safe atomic increment
		slot := &r.slots[ns][i.GetIndex()]
		atomic.AddUint32(&slot.count, count)
		if symbol.Flags.Has(ast.MustStartWithCapitalLetterForJSX) {
			atomic.StoreUint32(&slot.needsCapitalForJSX, 1)
		}
		return
	}

	// If it's a top-level symbol, defer it to later since we have
	// to allocate slots for these in serial instead of in parallel
	*topLevelSymbols = append(*topLevelSymbols, StableSymbolCount{
		StableSourceIndex: stableSourceIndices[ref.SourceIndex],
		Ref:               ref,
		Count:             count,
	})
}

// The parallel part of the symbol count accumulation algorithm above processes
// nested symbols and generates an array of top-level symbols to process later.
// After the parallel part has finished, that array of top-level symbols is passed
// to this function which processes them in serial.
func (r *MinifyRenamer) AllocateTopLevelSymbolSlots(topLevelSymbols StableSymbolCountArray) {
	for _, stable := range topLevelSymbols {
		symbol := r.symbols.Get(stable.Ref)
		slots := &r.slots[symbol.SlotNamespace()]
		if i, ok := r.topLevelSymbolToSlot[stable.Ref]; ok {
			slot := &(*slots)[i]
			slot.count += stable.Count
			if symbol.Flags.Has(ast.MustStartWithCapitalLetterForJSX) {
				slot.needsCapitalForJSX = 1
			}
		} else {
			needsCapitalForJSX := uint32(0)
			if symbol.Flags.Has(ast.MustStartWithCapitalLetterForJSX) {
				needsCapitalForJSX = 1
			}
			i = uint32(len(*slots))
			*slots = append(*slots, symbolSlot{
				count:              stable.Count,
				needsCapitalForJSX: needsCapitalForJSX,
			})
			r.topLevelSymbolToSlot[stable.Ref] = i
		}
	}
}

func (r *MinifyRenamer) AssignNamesByFrequency(minifier *ast.NameMinifier) {
	for ns, slots := range r.slots {
		// Sort symbols by count
		sorted := make(slotAndCountArray, len(slots))
		for i, item := range slots {
			sorted[i] = slotAndCount{slot: uint32(i), count: item.count}
		}
		sort.Sort(sorted)

		// Assign names to symbols
		nextName := 0
		for _, data := range sorted {
			slot := &slots[data.slot]
			name := minifier.NumberToMinifiedName(nextName)
			nextName++

			// Make sure we never generate a reserved name. We only have to worry
			// about collisions with reserved identifiers for normal symbols, and we
			// only have to worry about collisions with keywords for labels. We do
			// not have to worry about either for private names because they start
			// with a "#" character.
			switch ast.SlotNamespace(ns) {
			case ast.SlotDefault:
				for r.reservedNames[name] != 0 {
					name = minifier.NumberToMinifiedName(nextName)
					nextName++
				}

				// Make sure names of symbols used in JSX elements start with a capital letter
				if slot.needsCapitalForJSX != 0 {
					for name[0] >= 'a' && name[0] <= 'z' {
						name = minifier.NumberToMinifiedName(nextName)
						nextName++
					}
				}

			case ast.SlotLabel:
				for js_lexer.Keywords[name] != 0 {
					name = minifier.NumberToMinifiedName(nextName)
					nextName++
				}
			}

			// Private names must be prefixed with "#"
			if ast.SlotNamespace(ns) == ast.SlotPrivateName {
				name = "#" + name
			}

			slot.name = name
		}
	}
}

// Returns the number of nested slots
func AssignNestedScopeSlots(moduleScope *js_ast.Scope, symbols []ast.Symbol) (slotCounts ast.SlotCounts) {
	// Temporarily set the nested scope slots of top-level symbols to valid so
	// they aren't renamed in nested scopes. This prevents us from accidentally
	// assigning nested scope slots to variables declared using "var" in a nested
	// scope that are actually hoisted up to the module scope to become a top-
	// level symbol.
	validSlot := ast.MakeIndex32(1)
	for _, member := range moduleScope.Members {
		symbols[member.Ref.InnerIndex].NestedScopeSlot = validSlot
	}
	for _, ref := range moduleScope.Generated {
		symbols[ref.InnerIndex].NestedScopeSlot = validSlot
	}

	// Assign nested scope slots independently for each nested scope
	for _, child := range moduleScope.Children {
		slotCounts.UnionMax(assignNestedScopeSlotsHelper(child, symbols, ast.SlotCounts{}))
	}

	// Then set the nested scope slots of top-level symbols back to zero. Top-
	// level symbols are not supposed to have nested scope slots.
	for _, member := range moduleScope.Members {
		symbols[member.Ref.InnerIndex].NestedScopeSlot = ast.Index32{}
	}
	for _, ref := range moduleScope.Generated {
		symbols[ref.InnerIndex].NestedScopeSlot = ast.Index32{}
	}
	return
}

func assignNestedScopeSlotsHelper(scope *js_ast.Scope, symbols []ast.Symbol, slot ast.SlotCounts) ast.SlotCounts {
	// Sort member map keys for determinism
	sortedMembers := make([]int, 0, len(scope.Members))
	for _, member := range scope.Members {
		sortedMembers = append(sortedMembers, int(member.Ref.InnerIndex))
	}
	sort.Ints(sortedMembers)

	// Assign slots for this scope's symbols. Only do this if the slot is
	// not already assigned. Nested scopes have copies of symbols from parent
	// scopes and we want to use the slot from the parent scope, not child scopes.
	for _, innerIndex := range sortedMembers {
		symbol := &symbols[innerIndex]
		if ns := symbol.SlotNamespace(); ns != ast.SlotMustNotBeRenamed && !symbol.NestedScopeSlot.IsValid() {
			symbol.NestedScopeSlot = ast.MakeIndex32(slot[ns])
			slot[ns]++
		}
	}
	for _, ref := range scope.Generated {
		symbol := &symbols[ref.InnerIndex]
		if ns := symbol.SlotNamespace(); ns != ast.SlotMustNotBeRenamed && !symbol.NestedScopeSlot.IsValid() {
			symbol.NestedScopeSlot = ast.MakeIndex32(slot[ns])
			slot[ns]++
		}
	}

	// Labels are always declared in a nested scope, so we don't need to check.
	if scope.Label.Ref != ast.InvalidRef {
		symbol := &symbols[scope.Label.Ref.InnerIndex]
		symbol.NestedScopeSlot = ast.MakeIndex32(slot[ast.SlotLabel])
		slot[ast.SlotLabel]++
	}

	// Assign slots for the symbols of child scopes
	slotCounts := slot
	for _, child := range scope.Children {
		slotCounts.UnionMax(assignNestedScopeSlotsHelper(child, symbols, slot))
	}
	return slotCounts
}

type slotAndCount struct {
	slot  uint32
	count uint32
}

// This type is just so we can use Go's native sort function
type slotAndCountArray []slotAndCount

func (a slotAndCountArray) Len() int          { return len(a) }
func (a slotAndCountArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }
func (a slotAndCountArray) Less(i int, j int) bool {
	ai, aj := a[i], a[j]
	return ai.count > aj.count || (ai.count == aj.count && ai.slot < aj.slot)
}

////////////////////////////////////////////////////////////////////////////////
// NumberRenamer

type NumberRenamer struct {
	symbols ast.SymbolMap
	root    numberScope
	names   [][]string
}

func NewNumberRenamer(symbols ast.SymbolMap, reservedNames map[string]uint32) *NumberRenamer {
	return &NumberRenamer{
		symbols: symbols,
		names:   make([][]string, len(symbols.SymbolsForSource)),
		root:    numberScope{nameCounts: reservedNames},
	}
}

func (r *NumberRenamer) NameForSymbol(ref ast.Ref) string {
	ref = ast.FollowSymbols(r.symbols, ref)
	if inner := r.names[ref.SourceIndex]; inner != nil {
		if name := inner[ref.InnerIndex]; name != "" {
			return name
		}
	}
	return r.symbols.Get(ref).OriginalName
}

func (r *NumberRenamer) AddTopLevelSymbol(ref ast.Ref) {
	r.assignName(&r.root, ref)
}

func (r *NumberRenamer) assignName(scope *numberScope, ref ast.Ref) {
	ref = ast.FollowSymbols(r.symbols, ref)

	// Don't rename the same symbol more than once
	inner := r.names[ref.SourceIndex]
	if inner != nil && inner[ref.InnerIndex] != "" {
		return
	}

	// Don't rename unbound symbols, symbols marked as reserved names, labels, or private names
	symbol := r.symbols.Get(ref)
	ns := symbol.SlotNamespace()
	if ns != ast.SlotDefault && ns != ast.SlotPrivateName {
		return
	}

	// Make sure names of symbols used in JSX elements start with a capital letter
	originalName := symbol.OriginalName
	if symbol.Flags.Has(ast.MustStartWithCapitalLetterForJSX) {
		if first := rune(originalName[0]); first >= 'a' && first <= 'z' {
			originalName = fmt.Sprintf("%c%s", first+('A'-'a'), originalName[1:])
		}
	}

	// Compute a new name
	name := scope.findUnusedName(originalName, ns)

	// Store the new name
	if inner == nil {
		// Note: This should not be a data race even though this method is run from
		// multiple threads. The parallel part only looks at symbols defined in
		// nested scopes, and those can only ever be accessed from within the file.
		// References to those symbols should never spread across files.
		//
		// While we could avoid the data race by densely preallocating the entire
		// "names" array ahead of time, that will waste a lot more memory for
		// builds that make heavy use of code splitting and have many chunks. Doing
		// things lazily like this means we use less memory but still stay safe.
		inner = make([]string, len(r.symbols.SymbolsForSource[ref.SourceIndex]))
		r.names[ref.SourceIndex] = inner
	}
	inner[ref.InnerIndex] = name
}

func (r *NumberRenamer) assignNamesInScope(scope *js_ast.Scope, sourceIndex uint32, parent *numberScope, sorted *[]int) *numberScope {
	s := &numberScope{parent: parent, nameCounts: make(map[string]uint32)}

	if len(scope.Members) > 0 {
		// Sort member map keys for determinism, reusing a shared memory buffer
		*sorted = (*sorted)[:0]
		for _, member := range scope.Members {
			*sorted = append(*sorted, int(member.Ref.InnerIndex))
		}
		sort.Ints(*sorted)

		// Rename all user-defined symbols in this scope
		for _, innerIndex := range *sorted {
			r.assignName(s, ast.Ref{SourceIndex: sourceIndex, InnerIndex: uint32(innerIndex)})
		}
	}

	// Also rename all generated symbols in this scope
	for _, ref := range scope.Generated {
		r.assignName(s, ref)
	}

	return s
}

func (r *NumberRenamer) assignNamesRecursive(scope *js_ast.Scope, sourceIndex uint32, parent *numberScope, sorted *[]int) {
	// For performance in extreme cases (e.g. 10,000 nested scopes), traversing
	// through singly-nested scopes uses iteration instead of recursion
	for {
		if len(scope.Members) > 0 || len(scope.Generated) > 0 {
			// For performance in extreme cases (e.g. 10,000 nested scopes), only
			// allocate a scope when it's necessary. I'm not quite sure why allocating
			// one scope per level is so much overhead. It's not that many objects.
			// Or at least there are already that many objects for the AST that we're
			// traversing, so I don't know why 80% of the time in these extreme cases
			// is taken by this function (if we don't avoid this allocation).
			parent = r.assignNamesInScope(scope, sourceIndex, parent, sorted)
		}
		if children := scope.Children; len(children) == 1 {
			scope = children[0]
		} else {
			break
		}
	}

	// Symbols in child scopes may also have to be renamed to avoid conflicts
	for _, child := range scope.Children {
		r.assignNamesRecursive(child, sourceIndex, parent, sorted)
	}
}

func (r *NumberRenamer) AssignNamesByScope(nestedScopes map[uint32][]*js_ast.Scope) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(nestedScopes))

	// Rename nested scopes from separate files in parallel
	for sourceIndex, scopes := range nestedScopes {
		go func(sourceIndex uint32, scopes []*js_ast.Scope) {
			var sorted []int
			for _, scope := range scopes {
				r.assignNamesRecursive(scope, sourceIndex, &r.root, &sorted)
			}
			waitGroup.Done()
		}(sourceIndex, scopes)
	}

	waitGroup.Wait()
}

type numberScope struct {
	parent *numberScope

	// This is used as a set of used names in this scope. This also maps the name
	// to the number of times the name has experienced a collision. When a name
	// collides with an already-used name, we need to rename it. This is done by
	// incrementing a number at the end until the name is unused. We save the
	// count here so that subsequent collisions can start counting from where the
	// previous collision ended instead of having to start counting from 1.
	nameCounts map[string]uint32
}

type nameUse uint8

const (
	nameUnused nameUse = iota
	nameUsed
	nameUsedInSameScope
)

func (s *numberScope) findNameUse(name string) nameUse {
	original := s
	for {
		if _, ok := s.nameCounts[name]; ok {
			if s == original {
				return nameUsedInSameScope
			}
			return nameUsed
		}
		s = s.parent
		if s == nil {
			return nameUnused
		}
	}
}

func (s *numberScope) findUnusedName(name string, ns ast.SlotNamespace) string {
	// We may not have a valid identifier if this is an internally-constructed name
	if ns == ast.SlotPrivateName {
		if id := name[1:]; !js_ast.IsIdentifier(id) {
			name = js_ast.ForceValidIdentifier("#", id)
		}
	} else {
		if !js_ast.IsIdentifier(name) {
			name = js_ast.ForceValidIdentifier("", name)
		}
	}

	if use := s.findNameUse(name); use != nameUnused {
		// If the name is already in use, generate a new name by appending a number
		tries := uint32(1)
		if use == nameUsedInSameScope {
			// To avoid O(n^2) behavior, the number must start off being the number
			// that we used last time there was a collision with this name. Otherwise
			// if there are many collisions with the same name, each name collision
			// would have to increment the counter past all previous name collisions
			// which is a O(n^2) time algorithm. Only do this if this symbol comes
			// from the same scope as the previous one since sibling scopes can reuse
			// the same name without problems.
			tries = s.nameCounts[name]
		}
		prefix := name

		// Keep incrementing the number until the name is unused
		for {
			tries++
			name = prefix + strconv.Itoa(int(tries))

			// Make sure this new name is unused
			if s.findNameUse(name) == nameUnused {
				// Store the count so we can start here next time instead of starting
				// from 1. This means we avoid O(n^2) behavior.
				if use == nameUsedInSameScope {
					s.nameCounts[prefix] = tries
				}
				break
			}
		}
	}

	// Each name starts off with a count of 1 so that the first collision with
	// "name" is called "name2"
	s.nameCounts[name] = 1
	return name
}

////////////////////////////////////////////////////////////////////////////////
// ExportRenamer

type ExportRenamer struct {
	used  map[string]uint32
	count int
}

func (r *ExportRenamer) NextRenamedName(name string) string {
	if r.used == nil {
		r.used = make(map[string]uint32)
	}
	if tries, ok := r.used[name]; ok {
		prefix := name
		for {
			tries++
			name = prefix + strconv.Itoa(int(tries))
			if _, ok := r.used[name]; !ok {
				break
			}
		}
		r.used[name] = tries
	} else {
		r.used[name] = 1
	}
	return name
}

func (r *ExportRenamer) NextMinifiedName() string {
	name := ast.DefaultNameMinifierJS.NumberToMinifiedName(r.count)
	r.count++
	return name
}
