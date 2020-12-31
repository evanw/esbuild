package snap_printer

import (
	"bytes"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/snap_renamer"
	"sort"
)

type replacementLoc struct {
	loc     int
	replace *snap_renamer.Replacement
}

func sortedReplacements(namedReferences *map[js_ast.Ref]*snap_renamer.NamedReference) []replacementLoc {
	var replacementLocs []replacementLoc
	for _, reference := range *namedReferences {
		if reference.Replace != nil {
			for _, loc := range reference.UnreplacedLocs {
				replacementLocs = append(replacementLocs, replacementLoc{
					loc:     loc,
					replace: reference.Replace})
			}
		}
	}

	sort.Slice(replacementLocs, func(i, j int) bool {
		return replacementLocs[i].loc < replacementLocs[j].loc
	})
	return replacementLocs
}

func (p *printer) fixNamedBeforeReplaceds() {
	deltaLoc := 0
	sortedLocs := sortedReplacements(&p.renamer.NamedReferences)
	for _, replacement := range sortedLocs {
		deltaLoc += p.replaceAt(
			replacement.loc + +deltaLoc,
			replacement.replace.Original,
			replacement.replace.Replaced)
	}
}

func (p *printer) replaceAt(loc int, original string, replacement string) int {
	initialLen := len(p.js)

	originalBytes := []byte(original)
	replacementBytes := []byte(replacement)

	afterLoc := p.js[loc:initialLen]

	afterIdx := bytes.Index(afterLoc, originalBytes)

	replaceAt := loc + afterIdx
	continueAt := replaceAt + len(originalBytes)

	lenDiff := len(replacement) - len(original)

	js := make([]byte, initialLen+lenDiff)
	for i := 0; i < replaceAt; i++ {
		js[i] = p.js[i]
	}

	idx := replaceAt
	for i := 0; i < len(replacementBytes); i++ {
		js[idx] = replacementBytes[i]
		idx++
	}
	for i := continueAt; i < initialLen; i++ {
		js[idx] = p.js[i]
		idx++
	}

	p.js = js

	finalLen := len(p.js)
	return finalLen - initialLen
}
