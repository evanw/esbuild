package snap_printer

import (
	"github.com/evanw/esbuild/internal/js_ast"
	"regexp"
)

// Tracks `let` statements that need to be inserted at the top level scope and
// the top of the file.
// This is the simplest way to ensure that the replacement functions are declared
// before they are used and accessible where needed.
// The fact that they're not declared at the exact same scope as the original identifier
// should not matter esp. since their names are unique and thus won't be shadowed.
// Example:
// ```
// let a
// a = require('a')
// ```
// becomes
// ```
// let __get_a__;
// let a;
// __get_a__ = function() {
// 	 return a = a || require("a")
// };
// ```

type TopLevelVar struct {
	name     string
	idx      int
	inserted bool
}

func (p *printer) trackTopLevelVar(name string) {
	p.topLevelVars = append(p.topLevelVars, TopLevelVar{idx: len(p.js), name: name})
}

// __commonJS["./node_modules/upath/build/code/upath.js"] = function(exports2, module2, __filename, __dirname, require) {
var wrapperRx = regexp.MustCompile(`^ *__commonJS\[".+"] = function\(.+?\) {(\r\n|\r|\n)`)

// "use strict"
var useStrictRx = regexp.MustCompile(` *["']use strict["'];? *(\r\n|\r|\n)`)

func rxEndLocs(p *printer, rx *regexp.Regexp, multi bool) []int {
	if multi {
		idxs := rx.FindAllIndex(p.js, 1000)
		if len(idxs) == 0 {
			return nil
		}
		locs := make([]int, len(idxs))
		for i, el := range idxs {
			locs[i] = el[1]
		}
		return locs
	} else {
		loc := rx.FindIndex(p.js)
		if loc == nil {
			return nil
		} else {
			return []int{loc[1]}
		}
	}
}

func prepend(p *printer, idx *int, s string) {
	data := []byte(s)

	if idx == nil {
		p.js = append(data, p.js...)
	} else {
		jsLen := len(p.js)
		dataLen := len(data)
		completeJs := make([]byte, jsLen+dataLen)
		// Copy the wrapper open code that we matched
		for i := 0; i < *idx; i++ {
			completeJs[i] = p.js[i]
		}
		// Insert our declaration code
		for i := 0; i < dataLen; i++ {
			completeJs[i+*idx] = data[i]
		}
		// Copy the module body and wrapper close code after our declarations
		for i := *idx; i < jsLen; i++ {
			completeJs[i+dataLen] = p.js[i]
		}
		p.js = completeJs
	}
}

func (p *printer) prependTopLevelDecls() {
	if len(p.topLevelVars) == 0 {
		return
	}

	// We need to ensure that we add our declarations inside the wrapper function when we're dealing
	// with a bundle and the module code is wrapped.
	// If the file contains one or more 'use strict' declarations,
	// we need to insert our declaration after the closest 'use strict'
	// (searching upwards) from the original position.

	ends := rxEndLocs(p, useStrictRx, true)
	if ends == nil {
		ends = rxEndLocs(p, wrapperRx, false)
	}

	// In case we cannot find the insertion points add all declarations at
	// the top of the file.
	if ends == nil {
		decl := "let "
		for i, v := range p.topLevelVars {
			if i > 0 {
				decl += ", "
			}
			decl += v.name
		}
		decl += ";\n"
		prepend(p, nil, decl)
		return
	}

	// Otherwise group declarations by where they need to be inserted
	grouped := make(map[int][]string)
	endsLen := len(ends)
	for i, bottom := range ends {
		var top int
		if i+1 >= endsLen {
			top = len(p.js)
		} else {
			top = ends[i+1]
		}

		for _, tlv := range p.topLevelVars {
			if !tlv.inserted && bottom < tlv.idx && tlv.idx < top {
				group, ok := grouped[bottom]
				if !ok {
					grouped[bottom] = []string{tlv.name}
				} else {
					grouped[bottom] = append(group, tlv.name)
				}
			}
		}
	}

	offset := 0

	// TODO: map iteration order is not guaranteed but here we make the assumption that it is
	// https://stackoverflow.com/a/9621526/97443
	for idx, names := range grouped {
		decl := "let "
		for i, name := range names {
			if i > 0 {
				decl += ", "
			}
			decl += name
		}
		decl += ";\n"
		loc := idx + offset
		prepend(p, &loc, decl)
		offset = offset + len(decl)
	}
}

//
// Rewrite globals
//

// globals derived from electron-link blueprint declarations
// See: https://github.com/atom/electron-link/blob/abeb97d8633c06ac6a762ac427b272adebd32c4f/src/blueprint.js#L6
// Also related to: internal/resolver/resolver.go :1246 (BuiltInNodeModules)
var snapGlobals = []string{"process", "document", "global", "window", "console"}

func (p *printer) rewriteGlobals() {
	for outerIdx, outer := range p.symbols.Outer {
		for innerIdx, ref := range outer {
			// Globals aren't declared anywhere and thus are unbound
			if ref.Kind == js_ast.SymbolUnbound {
				for _, global := range snapGlobals {
					if ref.OriginalName == global {
						name := functionCallForGlobal(global)
						p.symbols.Outer[outerIdx][innerIdx].OriginalName = name
						continue
					}
				}
			}
		}
	}
}

func (p *printer) restoreGlobals() {
	for outerIdx, outer := range p.symbols.Outer {
		for innerIdx, ref := range outer {
			if ref.Kind == js_ast.SymbolUnbound {
				for _, global := range snapGlobals {
					if ref.OriginalName == functionCallForGlobal(global) {
						name := global
						p.symbols.Outer[outerIdx][innerIdx].OriginalName = name
						continue
					}
				}
			}
		}
	}
}
