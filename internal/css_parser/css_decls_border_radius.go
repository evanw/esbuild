package css_parser

import (
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

const (
	borderRadiusTopLeft = iota
	borderRadiusTopRight
	borderRadiusBottomRight
	borderRadiusBottomLeft
)

type borderRadiusCorner struct {
	firstToken    css_ast.Token
	secondToken   css_ast.Token
	unitSafety    unitSafetyTracker
	ruleIndex     uint32 // The index of the originating rule in the rules array
	wasSingleRule bool   // True if the originating rule was just for this side
}

type borderRadiusTracker struct {
	corners   [4]borderRadiusCorner
	important bool // True if all active rules were flagged as "!important"
}

func (borderRadius *borderRadiusTracker) updateCorner(rules []css_ast.Rule, corner int, new borderRadiusCorner) {
	if old := borderRadius.corners[corner]; old.firstToken.Kind != css_lexer.TEndOfFile &&
		(!new.wasSingleRule || old.wasSingleRule) &&
		old.unitSafety.status == unitSafe && new.unitSafety.status == unitSafe {
		rules[old.ruleIndex] = css_ast.Rule{}
	}
	borderRadius.corners[corner] = new
}

func (borderRadius *borderRadiusTracker) mangleCorners(rules []css_ast.Rule, decl *css_ast.RDeclaration, minifyWhitespace bool) {
	// Reset if we see a change in the "!important" flag
	if borderRadius.important != decl.Important {
		borderRadius.corners = [4]borderRadiusCorner{}
		borderRadius.important = decl.Important
	}

	tokens := decl.Value
	beforeSplit := len(tokens)
	afterSplit := len(tokens)

	// Search for the single slash if present
	for i, t := range tokens {
		if t.Kind == css_lexer.TDelimSlash {
			if beforeSplit == len(tokens) {
				beforeSplit = i
				afterSplit = i + 1
			} else {
				// Multiple slashes are an error
				borderRadius.corners = [4]borderRadiusCorner{}
				return
			}
		}
	}

	// Use a single tracker for the whole rule
	unitSafety := unitSafetyTracker{}
	for _, t := range tokens[:beforeSplit] {
		unitSafety.includeUnitOf(t)
	}
	for _, t := range tokens[afterSplit:] {
		unitSafety.includeUnitOf(t)
	}

	firstRadii, firstRadiiOk := expandTokenQuad(tokens[:beforeSplit], "")
	lastRadii, lastRadiiOk := expandTokenQuad(tokens[afterSplit:], "")

	// Stop now if the pattern wasn't matched
	if !firstRadiiOk || (beforeSplit < afterSplit && !lastRadiiOk) {
		borderRadius.corners = [4]borderRadiusCorner{}
		return
	}

	// Handle the first radii
	for corner, t := range firstRadii {
		if unitSafety.status == unitSafe {
			t.TurnLengthIntoNumberIfZero()
		}
		borderRadius.updateCorner(rules, corner, borderRadiusCorner{
			firstToken:  t,
			secondToken: t,
			unitSafety:  unitSafety,
			ruleIndex:   uint32(len(rules) - 1),
		})
	}

	// Handle the last radii
	if lastRadiiOk {
		for corner, t := range lastRadii {
			if unitSafety.status == unitSafe {
				t.TurnLengthIntoNumberIfZero()
			}
			borderRadius.corners[corner].secondToken = t
		}
	}

	// Success
	borderRadius.compactRules(rules, decl.KeyRange, minifyWhitespace)
}

func (borderRadius *borderRadiusTracker) mangleCorner(rules []css_ast.Rule, decl *css_ast.RDeclaration, minifyWhitespace bool, corner int) {
	// Reset if we see a change in the "!important" flag
	if borderRadius.important != decl.Important {
		borderRadius.corners = [4]borderRadiusCorner{}
		borderRadius.important = decl.Important
	}

	if tokens := decl.Value; (len(tokens) == 1 && tokens[0].Kind.IsNumeric()) ||
		(len(tokens) == 2 && tokens[0].Kind.IsNumeric() && tokens[1].Kind.IsNumeric()) {
		firstToken := tokens[0]
		secondToken := firstToken
		if len(tokens) == 2 {
			secondToken = tokens[1]
		}

		// Check to see if these units are safe to use in every browser
		unitSafety := unitSafetyTracker{}
		unitSafety.includeUnitOf(firstToken)
		unitSafety.includeUnitOf(secondToken)

		// Only collapse "0unit" into "0" if the unit is safe
		if unitSafety.status == unitSafe && firstToken.TurnLengthIntoNumberIfZero() {
			tokens[0] = firstToken
		}
		if len(tokens) == 2 {
			if unitSafety.status == unitSafe && secondToken.TurnLengthIntoNumberIfZero() {
				tokens[1] = secondToken
			}

			// If both tokens are equal, merge them into one
			if firstToken.EqualIgnoringWhitespace(secondToken) {
				tokens[0].Whitespace &= ^css_ast.WhitespaceAfter
				decl.Value = tokens[:1]
			}
		}

		borderRadius.updateCorner(rules, corner, borderRadiusCorner{
			firstToken:    firstToken,
			secondToken:   secondToken,
			unitSafety:    unitSafety,
			ruleIndex:     uint32(len(rules) - 1),
			wasSingleRule: true,
		})
		borderRadius.compactRules(rules, decl.KeyRange, minifyWhitespace)
	} else {
		borderRadius.corners = [4]borderRadiusCorner{}
	}
}

func (borderRadius *borderRadiusTracker) compactRules(rules []css_ast.Rule, keyRange logger.Range, minifyWhitespace bool) {
	// All tokens must be present
	if eof := css_lexer.TEndOfFile; borderRadius.corners[0].firstToken.Kind == eof || borderRadius.corners[1].firstToken.Kind == eof ||
		borderRadius.corners[2].firstToken.Kind == eof || borderRadius.corners[3].firstToken.Kind == eof {
		return
	}

	// All tokens must have the same unit
	for _, side := range borderRadius.corners[1:] {
		if !side.unitSafety.isSafeWith(borderRadius.corners[0].unitSafety) {
			return
		}
	}

	// Generate the most minimal representation
	tokens := compactTokenQuad(
		borderRadius.corners[0].firstToken,
		borderRadius.corners[1].firstToken,
		borderRadius.corners[2].firstToken,
		borderRadius.corners[3].firstToken,
		minifyWhitespace,
	)
	secondTokens := compactTokenQuad(
		borderRadius.corners[0].secondToken,
		borderRadius.corners[1].secondToken,
		borderRadius.corners[2].secondToken,
		borderRadius.corners[3].secondToken,
		minifyWhitespace,
	)
	if !css_ast.TokensEqualIgnoringWhitespace(tokens, secondTokens) {
		var whitespace css_ast.WhitespaceFlags
		if !minifyWhitespace {
			whitespace = css_ast.WhitespaceBefore | css_ast.WhitespaceAfter
		}
		tokens = append(tokens, css_ast.Token{
			Loc:        tokens[len(tokens)-1].Loc,
			Kind:       css_lexer.TDelimSlash,
			Text:       "/",
			Whitespace: whitespace,
		})
		tokens = append(tokens, secondTokens...)
	}

	// Remove all of the existing declarations
	var minLoc logger.Loc
	for i, corner := range borderRadius.corners {
		if loc := rules[corner.ruleIndex].Loc; i == 0 || loc.Start < minLoc.Start {
			minLoc = loc
		}
		rules[corner.ruleIndex] = css_ast.Rule{}
	}

	// Insert the combined declaration where the last rule was
	rules[borderRadius.corners[3].ruleIndex] = css_ast.Rule{Loc: minLoc, Data: &css_ast.RDeclaration{
		Key:       css_ast.DBorderRadius,
		KeyText:   "border-radius",
		Value:     tokens,
		KeyRange:  keyRange,
		Important: borderRadius.important,
	}}
}
