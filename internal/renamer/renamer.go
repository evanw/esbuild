package renamer

import (
	"sort"
	"strconv"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
)

func ComputeReservedNames(moduleScopes []*js_ast.Scope, symbols js_ast.SymbolMap) map[string]uint32 {
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
		for _, member := range scope.Members {
			symbol := symbols.Get(member.Ref)
			if symbol.Kind == js_ast.SymbolUnbound || symbol.MustNotBeRenamed {
				names[symbol.OriginalName] = 1
			}
		}
		for _, ref := range scope.Generated {
			symbol := symbols.Get(ref)
			if symbol.Kind == js_ast.SymbolUnbound || symbol.MustNotBeRenamed {
				names[symbol.OriginalName] = 1
			}
		}
	}

	return names
}

type Renamer interface {
	NameForSymbol(ref js_ast.Ref) string
}

////////////////////////////////////////////////////////////////////////////////
// noOpRenamer

type noOpRenamer struct {
	symbols js_ast.SymbolMap
}

func NewNoOpRenamer(symbols js_ast.SymbolMap) Renamer {
	return &noOpRenamer{
		symbols: symbols,
	}
}

func (r *noOpRenamer) NameForSymbol(ref js_ast.Ref) string {
	ref = js_ast.FollowSymbols(r.symbols, ref)
	return r.symbols.Get(ref).OriginalName
}

////////////////////////////////////////////////////////////////////////////////
// MinifyRenamer

type symbolSlot struct {
	name  string
	count uint32
}

type MinifyRenamer struct {
	symbols       js_ast.SymbolMap
	sortedBuffer  StableRefArray
	reservedNames map[string]uint32
	slots         [3][]symbolSlot
	symbolToSlot  map[js_ast.Ref]ast.Index32
}

func NewMinifyRenamer(symbols js_ast.SymbolMap, firstTopLevelSlots js_ast.SlotCounts, reservedNames map[string]uint32) *MinifyRenamer {
	return &MinifyRenamer{
		symbols:       symbols,
		reservedNames: reservedNames,
		slots: [3][]symbolSlot{
			make([]symbolSlot, firstTopLevelSlots[0]),
			make([]symbolSlot, firstTopLevelSlots[1]),
			make([]symbolSlot, firstTopLevelSlots[2]),
		},
		symbolToSlot: make(map[js_ast.Ref]ast.Index32),
	}
}

func (r *MinifyRenamer) NameForSymbol(ref js_ast.Ref) string {
	// Follow links to get to the underlying symbol
	ref = js_ast.FollowSymbols(r.symbols, ref)
	symbol := r.symbols.Get(ref)

	// Skip this symbol if the name is pinned
	ns := symbol.SlotNamespace()
	if ns == js_ast.SlotMustNotBeRenamed {
		return symbol.OriginalName
	}

	// Check if it's a nested scope symbol
	i := symbol.NestedScopeSlot

	// If it's not (i.e. it's in a top-level scope), look up the slot
	if !i.IsValid() {
		var ok bool
		i, ok = r.symbolToSlot[ref]
		if !ok {
			// If we get here, then we're printing a symbol that never had any
			// recorded uses. This is odd but can happen in certain scenarios.
			// For example, code in a branch with dead control flow won't mark
			// any uses but may still be printed. In that case it doesn't matter
			// what name we use since it's dead code.
			return symbol.OriginalName
		}
	}

	return r.slots[ns][i.GetIndex()].name
}

func (r *MinifyRenamer) AccumulateSymbolUseCounts(symbolUses map[js_ast.Ref]js_ast.SymbolUse, stableSourceIndices []uint32) {
	// Sort symbol uses for determinism, reusing a shared memory buffer
	r.sortedBuffer = r.sortedBuffer[:0]
	for ref := range symbolUses {
		r.sortedBuffer = append(r.sortedBuffer, StableRef{
			StableSourceIndex: stableSourceIndices[ref.SourceIndex],
			Ref:               ref,
		})
	}
	sort.Sort(r.sortedBuffer)

	// Accumulate symbol use counts
	for _, stable := range r.sortedBuffer {
		r.AccumulateSymbolCount(stable.Ref, symbolUses[stable.Ref].CountEstimate)
	}
}

func (r *MinifyRenamer) AccumulateSymbolCount(ref js_ast.Ref, count uint32) {
	// Follow links to get to the underlying symbol
	ref = js_ast.FollowSymbols(r.symbols, ref)
	symbol := r.symbols.Get(ref)
	for symbol.NamespaceAlias != nil {
		ref = js_ast.FollowSymbols(r.symbols, symbol.NamespaceAlias.NamespaceRef)
		symbol = r.symbols.Get(ref)
	}

	// Skip this symbol if the name is pinned
	ns := symbol.SlotNamespace()
	if ns == js_ast.SlotMustNotBeRenamed {
		return
	}

	// Check if it's a nested scope symbol
	slots := &r.slots[ns]
	i := symbol.NestedScopeSlot

	// If it's not (i.e. it's in a top-level scope), allocate a slot for it
	if !i.IsValid() {
		var ok bool
		i, ok = r.symbolToSlot[ref]
		if !ok {
			i = ast.MakeIndex32(uint32(len(*slots)))
			*slots = append(*slots, symbolSlot{})
			r.symbolToSlot[ref] = i
		}
	}

	(*slots)[i.GetIndex()].count += count
}

func (r *MinifyRenamer) AssignNamesByFrequency(minifier *js_ast.NameMinifier) {
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
			name := minifier.NumberToMinifiedName(nextName)
			nextName++

			// Make sure we never generate a reserved name. We only have to worry
			// about collisions with reserved identifiers for normal symbols, and we
			// only have to worry about collisions with keywords for labels. We do
			// not have to worry about either for private names because they start
			// with a "#" character.
			switch js_ast.SlotNamespace(ns) {
			case js_ast.SlotDefault:
				for r.reservedNames[name] != 0 {
					name = minifier.NumberToMinifiedName(nextName)
					nextName++
				}

			case js_ast.SlotLabel:
				for js_lexer.Keywords[name] != 0 {
					name = minifier.NumberToMinifiedName(nextName)
					nextName++
				}
			}

			// Private names must be prefixed with "#"
			if js_ast.SlotNamespace(ns) == js_ast.SlotPrivateName {
				name = "#" + name
			}

			slots[data.slot].name = name
		}
	}
}

// Returns the number of nested slots
func AssignNestedScopeSlots(moduleScope *js_ast.Scope, symbols []js_ast.Symbol) (slotCounts js_ast.SlotCounts) {
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
		slotCounts.UnionMax(assignNestedScopeSlotsHelper(child, symbols, js_ast.SlotCounts{}))
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

func assignNestedScopeSlotsHelper(scope *js_ast.Scope, symbols []js_ast.Symbol, slot js_ast.SlotCounts) js_ast.SlotCounts {
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
		if ns := symbol.SlotNamespace(); ns != js_ast.SlotMustNotBeRenamed && !symbol.NestedScopeSlot.IsValid() {
			symbol.NestedScopeSlot = ast.MakeIndex32(slot[ns])
			slot[ns]++
		}
	}
	for _, ref := range scope.Generated {
		symbol := &symbols[ref.InnerIndex]
		if ns := symbol.SlotNamespace(); ns != js_ast.SlotMustNotBeRenamed && !symbol.NestedScopeSlot.IsValid() {
			symbol.NestedScopeSlot = ast.MakeIndex32(slot[ns])
			slot[ns]++
		}
	}

	// Labels are always declared in a nested scope, so we don't need to check.
	if scope.LabelRef != js_ast.InvalidRef {
		symbol := &symbols[scope.LabelRef.InnerIndex]
		symbol.NestedScopeSlot = ast.MakeIndex32(slot[js_ast.SlotLabel])
		slot[js_ast.SlotLabel]++
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

// The sort order here is arbitrary but needs to be consistent between builds.
// The InnerIndex should be stable because the parser for a single file is
// single-threaded and deterministically assigns out InnerIndex values
// sequentially. But the SourceIndex should be unstable because the main thread
// assigns out source index values sequentially to newly-discovered dependencies
// in a multi-threaded producer/consumer relationship. So instead we use the
// index of the source in the DFS order over all entry points for stability.
type StableRef struct {
	StableSourceIndex uint32
	Ref               js_ast.Ref
}

// This type is just so we can use Go's native sort function
type StableRefArray []StableRef

func (a StableRefArray) Len() int          { return len(a) }
func (a StableRefArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }
func (a StableRefArray) Less(i int, j int) bool {
	ai, aj := a[i], a[j]
	return ai.StableSourceIndex < aj.StableSourceIndex || (ai.StableSourceIndex == aj.StableSourceIndex && ai.Ref.InnerIndex < aj.Ref.InnerIndex)
}

////////////////////////////////////////////////////////////////////////////////
// NumberRenamer

type NumberRenamer struct {
	symbols js_ast.SymbolMap
	names   [][]string
	root    numberScope
}

func NewNumberRenamer(symbols js_ast.SymbolMap, reservedNames map[string]uint32) *NumberRenamer {
	return &NumberRenamer{
		symbols: symbols,
		names:   make([][]string, len(symbols.SymbolsForSource)),
		root:    numberScope{nameCounts: reservedNames},
	}
}

func (r *NumberRenamer) NameForSymbol(ref js_ast.Ref) string {
	ref = js_ast.FollowSymbols(r.symbols, ref)
	if inner := r.names[ref.SourceIndex]; inner != nil {
		if name := inner[ref.InnerIndex]; name != "" {
			return name
		}
	}
	return r.symbols.Get(ref).OriginalName
}

func (r *NumberRenamer) AddTopLevelSymbol(ref js_ast.Ref) {
	r.assignName(&r.root, ref)
}

func (r *NumberRenamer) assignName(scope *numberScope, ref js_ast.Ref) {
	ref = js_ast.FollowSymbols(r.symbols, ref)

	// Don't rename the same symbol more than once
	inner := r.names[ref.SourceIndex]
	if inner != nil && inner[ref.InnerIndex] != "" {
		return
	}

	// Don't rename unbound symbols, symbols marked as reserved names, labels, or private names
	symbol := r.symbols.Get(ref)
	if symbol.SlotNamespace() != js_ast.SlotDefault {
		return
	}

	// Compute a new name
	name := scope.findUnusedName(symbol.OriginalName)

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

func (r *NumberRenamer) assignNamesRecursive(scope *js_ast.Scope, sourceIndex uint32, parent *numberScope, sorted *[]int) {
	s := &numberScope{parent: parent, nameCounts: make(map[string]uint32)}

	// Sort member map keys for determinism, reusing a shared memory buffer
	*sorted = (*sorted)[:0]
	for _, member := range scope.Members {
		*sorted = append(*sorted, int(member.Ref.InnerIndex))
	}
	sort.Ints(*sorted)

	// Rename all symbols in this scope
	for _, innerIndex := range *sorted {
		r.assignName(s, js_ast.Ref{SourceIndex: sourceIndex, InnerIndex: uint32(innerIndex)})
	}
	for _, ref := range scope.Generated {
		r.assignName(s, ref)
	}

	// Symbols in child scopes may also have to be renamed to avoid conflicts
	for _, child := range scope.Children {
		r.assignNamesRecursive(child, sourceIndex, s, sorted)
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

func (s *numberScope) findUnusedName(name string) string {
	name = js_lexer.ForceValidIdentifier(name)

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
	count int
	used  map[string]uint32
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
	name := js_ast.DefaultNameMinifier.NumberToMinifiedName(r.count)
	r.count++
	return name
}
