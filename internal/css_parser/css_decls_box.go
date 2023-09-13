package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

const (
	boxTop = iota
	boxRight
	boxBottom
	boxLeft
)

type boxSide struct {
	token         css_ast.Token
	unitSafety    unitSafetyTracker
	ruleIndex     uint32 // The index of the originating rule in the rules array
	wasSingleRule bool   // True if the originating rule was just for this side
}

type boxTracker struct {
	keyText   string
	sides     [4]boxSide
	allowAuto bool // If true, allow the "auto" keyword
	important bool // True if all active rules were flagged as "!important"
	key       css_ast.D
}

type unitSafetyStatus uint8

const (
	unitSafe         unitSafetyStatus = iota // "margin: 0 1px 2cm 3%;"
	unitUnsafeSingle                         // "margin: 0 1vw 2vw 3vw;"
	unitUnsafeMixed                          // "margin: 0 1vw 2vh 3ch;"
)

// We can only compact rules together if they have the same unit safety level.
// We want to avoid a situation where the browser treats some of the original
// rules as valid and others as invalid.
//
//	Safe:
//	  top: 1px; left: 0; bottom: 1px; right: 0;
//	  top: 1Q; left: 2Q; bottom: 3Q; right: 4Q;
//
//	Unsafe:
//	  top: 1vh; left: 2vw; bottom: 3vh; right: 4vw;
//	  top: 1Q; left: 2Q; bottom: 3Q; right: 0;
//	  inset: 1Q 0 0 0; top: 0;
type unitSafetyTracker struct {
	unit   string
	status unitSafetyStatus
}

func (a unitSafetyTracker) isSafeWith(b unitSafetyTracker) bool {
	return a.status == b.status && a.status != unitUnsafeMixed && (a.status != unitUnsafeSingle || a.unit == b.unit)
}

func (t *unitSafetyTracker) includeUnitOf(token css_ast.Token) {
	switch token.Kind {
	case css_lexer.TNumber:
		if token.Text == "0" {
			return
		}

	case css_lexer.TPercentage:
		return

	case css_lexer.TDimension:
		if token.DimensionUnitIsSafeLength() {
			return
		} else if unit := token.DimensionUnit(); t.status == unitSafe {
			t.status = unitUnsafeSingle
			t.unit = unit
			return
		} else if t.status == unitUnsafeSingle && t.unit == unit {
			return
		}
	}

	t.status = unitUnsafeMixed
}

func (box *boxTracker) updateSide(rules []css_ast.Rule, side int, new boxSide) {
	if old := box.sides[side]; old.token.Kind != css_lexer.TEndOfFile &&
		(!new.wasSingleRule || old.wasSingleRule) &&
		old.unitSafety.status == unitSafe && new.unitSafety.status == unitSafe {
		rules[old.ruleIndex] = css_ast.Rule{}
	}
	box.sides[side] = new
}

func (box *boxTracker) mangleSides(rules []css_ast.Rule, decl *css_ast.RDeclaration, minifyWhitespace bool) {
	// Reset if we see a change in the "!important" flag
	if box.important != decl.Important {
		box.sides = [4]boxSide{}
		box.important = decl.Important
	}

	allowedIdent := ""
	if box.allowAuto {
		allowedIdent = "auto"
	}
	if quad, ok := expandTokenQuad(decl.Value, allowedIdent); ok {
		// Use a single tracker for the whole rule
		unitSafety := unitSafetyTracker{}
		for _, t := range quad {
			if !box.allowAuto || t.Kind.IsNumeric() {
				unitSafety.includeUnitOf(t)
			}
		}
		for side, t := range quad {
			if unitSafety.status == unitSafe {
				t.TurnLengthIntoNumberIfZero()
			}
			box.updateSide(rules, side, boxSide{
				token:      t,
				ruleIndex:  uint32(len(rules) - 1),
				unitSafety: unitSafety,
			})
		}
		box.compactRules(rules, decl.KeyRange, minifyWhitespace)
	} else {
		box.sides = [4]boxSide{}
	}
}

func (box *boxTracker) mangleSide(rules []css_ast.Rule, decl *css_ast.RDeclaration, minifyWhitespace bool, side int) {
	// Reset if we see a change in the "!important" flag
	if box.important != decl.Important {
		box.sides = [4]boxSide{}
		box.important = decl.Important
	}

	if tokens := decl.Value; len(tokens) == 1 {
		if t := tokens[0]; t.Kind.IsNumeric() || (t.Kind == css_lexer.TIdent && box.allowAuto && strings.EqualFold(t.Text, "auto")) {
			unitSafety := unitSafetyTracker{}
			if !box.allowAuto || t.Kind.IsNumeric() {
				unitSafety.includeUnitOf(t)
			}
			if unitSafety.status == unitSafe && t.TurnLengthIntoNumberIfZero() {
				tokens[0] = t
			}
			box.updateSide(rules, side, boxSide{
				token:         t,
				ruleIndex:     uint32(len(rules) - 1),
				wasSingleRule: true,
				unitSafety:    unitSafety,
			})
			box.compactRules(rules, decl.KeyRange, minifyWhitespace)
			return
		}
	}

	box.sides = [4]boxSide{}
}

func (box *boxTracker) compactRules(rules []css_ast.Rule, keyRange logger.Range, minifyWhitespace bool) {
	// Don't compact if the shorthand form is unsupported
	if box.key == css_ast.DUnknown {
		return
	}

	// All tokens must be present
	if eof := css_lexer.TEndOfFile; box.sides[0].token.Kind == eof || box.sides[1].token.Kind == eof ||
		box.sides[2].token.Kind == eof || box.sides[3].token.Kind == eof {
		return
	}

	// All tokens must have the same unit
	for _, side := range box.sides[1:] {
		if !side.unitSafety.isSafeWith(box.sides[0].unitSafety) {
			return
		}
	}

	// Generate the most minimal representation
	tokens := compactTokenQuad(
		box.sides[0].token,
		box.sides[1].token,
		box.sides[2].token,
		box.sides[3].token,
		minifyWhitespace,
	)

	// Remove all of the existing declarations
	var minLoc logger.Loc
	for i, side := range box.sides {
		if loc := rules[side.ruleIndex].Loc; i == 0 || loc.Start < minLoc.Start {
			minLoc = loc
		}
		rules[side.ruleIndex] = css_ast.Rule{}
	}

	// Insert the combined declaration where the last rule was
	rules[box.sides[3].ruleIndex] = css_ast.Rule{Loc: minLoc, Data: &css_ast.RDeclaration{
		Key:       box.key,
		KeyText:   box.keyText,
		Value:     tokens,
		KeyRange:  keyRange,
		Important: box.important,
	}}
}
