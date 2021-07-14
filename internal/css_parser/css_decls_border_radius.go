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
	firstToken  css_ast.Token
	secondToken css_ast.Token
	index       uint32
	single      bool
}

type borderRadiusTracker struct {
	corners   [4]borderRadiusCorner
	important bool
}

func (borderRadius *borderRadiusTracker) updateCorner(rules []css_ast.R, corner int, new borderRadiusCorner) {
	if old := borderRadius.corners[corner]; old.firstToken.Kind != css_lexer.TEndOfFile && (!new.single || old.single) {
		rules[old.index] = nil
	}
	borderRadius.corners[corner] = new
}

func (borderRadius *borderRadiusTracker) mangleCorners(rules []css_ast.R, decl *css_ast.RDeclaration, index int, removeWhitespace bool) {
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

	firstRadii, firstRadiiOk := expandTokenQuad(tokens[:beforeSplit])
	lastRadii, lastRadiiOk := expandTokenQuad(tokens[afterSplit:])

	// Stop now if the pattern wasn't matched
	if !firstRadiiOk || (beforeSplit < afterSplit && !lastRadiiOk) {
		borderRadius.corners = [4]borderRadiusCorner{}
		return
	}

	// Handle the first radii
	for corner, t := range firstRadii {
		t.TurnLengthIntoNumberIfZero()
		borderRadius.updateCorner(rules, corner, borderRadiusCorner{
			firstToken:  t,
			secondToken: t,
			index:       uint32(index),
		})
	}

	// Handle the last radii
	if lastRadiiOk {
		for corner, t := range lastRadii {
			t.TurnLengthIntoNumberIfZero()
			borderRadius.corners[corner].secondToken = t
		}
	}

	// Success
	borderRadius.compactRules(rules, decl.KeyRange, removeWhitespace)
}

func (borderRadius *borderRadiusTracker) mangleCorner(rules []css_ast.R, decl *css_ast.RDeclaration, index int, removeWhitespace bool, corner int) {
	// Reset if we see a change in the "!important" flag
	if borderRadius.important != decl.Important {
		borderRadius.corners = [4]borderRadiusCorner{}
		borderRadius.important = decl.Important
	}

	if tokens := decl.Value; (len(tokens) == 1 && tokens[0].Kind.IsNumericOrIdent()) ||
		(len(tokens) == 2 && tokens[0].Kind.IsNumericOrIdent() && tokens[1].Kind.IsNumericOrIdent()) {
		firstToken := tokens[0]
		if firstToken.TurnLengthIntoNumberIfZero() {
			tokens[0] = firstToken
		}
		secondToken := firstToken
		if len(tokens) == 2 {
			secondToken = tokens[1]
			if secondToken.TurnLengthIntoNumberIfZero() {
				tokens[1] = secondToken
			}
			if firstToken.EqualIgnoringWhitespace(secondToken) {
				tokens[0].Whitespace &= ^css_ast.WhitespaceAfter
				decl.Value = tokens[:1]
			}
		}
		borderRadius.updateCorner(rules, corner, borderRadiusCorner{
			firstToken:  firstToken,
			secondToken: secondToken,
			index:       uint32(index),
			single:      true,
		})
		borderRadius.compactRules(rules, decl.KeyRange, removeWhitespace)
	} else {
		borderRadius.corners = [4]borderRadiusCorner{}
	}
}

func (borderRadius *borderRadiusTracker) compactRules(rules []css_ast.R, keyRange logger.Range, removeWhitespace bool) {
	// All tokens must be present
	if eof := css_lexer.TEndOfFile; borderRadius.corners[0].firstToken.Kind == eof || borderRadius.corners[1].firstToken.Kind == eof ||
		borderRadius.corners[2].firstToken.Kind == eof || borderRadius.corners[3].firstToken.Kind == eof {
		return
	}

	// Generate the most minimal representation
	tokens := compactTokenQuad(
		borderRadius.corners[0].firstToken,
		borderRadius.corners[1].firstToken,
		borderRadius.corners[2].firstToken,
		borderRadius.corners[3].firstToken,
		removeWhitespace,
	)
	secondTokens := compactTokenQuad(
		borderRadius.corners[0].secondToken,
		borderRadius.corners[1].secondToken,
		borderRadius.corners[2].secondToken,
		borderRadius.corners[3].secondToken,
		removeWhitespace,
	)
	if !css_ast.TokensEqualIgnoringWhitespace(tokens, secondTokens) {
		var whitespace css_ast.WhitespaceFlags
		if !removeWhitespace {
			whitespace = css_ast.WhitespaceBefore | css_ast.WhitespaceAfter
		}
		tokens = append(tokens, css_ast.Token{
			Kind:       css_lexer.TDelimSlash,
			Text:       "/",
			Whitespace: whitespace,
		})
		tokens = append(tokens, secondTokens...)
	}

	// Remove all of the existing declarations
	rules[borderRadius.corners[0].index] = nil
	rules[borderRadius.corners[1].index] = nil
	rules[borderRadius.corners[2].index] = nil
	rules[borderRadius.corners[3].index] = nil

	// Insert the combined declaration where the last rule was
	rules[borderRadius.corners[3].index] = &css_ast.RDeclaration{
		Key:       css_ast.DBorderRadius,
		KeyText:   "border-radius",
		Value:     tokens,
		KeyRange:  keyRange,
		Important: borderRadius.important,
	}
}
