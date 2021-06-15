package css_parser

import (
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
	token  css_ast.Token
	index  uint32
	single bool
}

type boxTracker struct {
	sides     [4]boxSide
	important bool
}

func (box *boxTracker) updateSide(rules []css_ast.R, side int, new boxSide) {
	if old := box.sides[side]; old.token.Kind != css_lexer.TEndOfFile && (!new.single || old.single) {
		rules[old.index] = nil
	}
	box.sides[side] = new
}

func (box *boxTracker) mangleSides(rules []css_ast.R, decl *css_ast.RDeclaration, index int, removeWhitespace bool) {
	// Reset if we see a change in the "!important" flag
	if box.important != decl.Important {
		box.sides = [4]boxSide{}
		box.important = decl.Important
	}

	if n := len(decl.Value); n >= 1 && n <= 4 {
		isMargin := decl.Key == css_ast.DMargin
		for side, t := range expandTokenQuad(decl.Value) {
			t.TurnLengthIntoNumberIfZero()
			box.updateSide(rules, side, boxSide{token: t, index: uint32(index)})
		}
		box.compactRules(rules, decl.KeyRange, removeWhitespace, isMargin)
	} else {
		box.sides = [4]boxSide{}
	}
}

func (box *boxTracker) mangleSide(rules []css_ast.R, decl *css_ast.RDeclaration, index int, removeWhitespace bool, side int) {
	// Reset if we see a change in the "!important" flag
	if box.important != decl.Important {
		box.sides = [4]boxSide{}
		box.important = decl.Important
	}

	if tokens := decl.Value; len(tokens) == 1 {
		isMargin := false
		switch decl.Key {
		case css_ast.DMarginTop, css_ast.DMarginRight, css_ast.DMarginBottom, css_ast.DMarginLeft:
			isMargin = true
		}
		t := tokens[0]
		if t.TurnLengthIntoNumberIfZero() {
			tokens[0] = t
		}
		box.updateSide(rules, side, boxSide{token: t, index: uint32(index), single: true})
		box.compactRules(rules, decl.KeyRange, removeWhitespace, isMargin)
	} else {
		box.sides = [4]boxSide{}
	}
}

func (box *boxTracker) compactRules(rules []css_ast.R, keyRange logger.Range, removeWhitespace bool, isMargin bool) {
	// All tokens must be present
	if eof := css_lexer.TEndOfFile; box.sides[0].token.Kind == eof || box.sides[1].token.Kind == eof ||
		box.sides[2].token.Kind == eof || box.sides[3].token.Kind == eof {
		return
	}

	// Generate the most minimal representation
	tokens := compactTokenQuad(
		box.sides[0].token,
		box.sides[1].token,
		box.sides[2].token,
		box.sides[3].token,
		removeWhitespace,
	)

	// Remove all of the existing declarations
	rules[box.sides[0].index] = nil
	rules[box.sides[1].index] = nil
	rules[box.sides[2].index] = nil
	rules[box.sides[3].index] = nil

	// Insert the combined declaration where the last rule was
	var key css_ast.D
	var keyText string
	if isMargin {
		key = css_ast.DMargin
		keyText = "margin"
	} else {
		key = css_ast.DPadding
		keyText = "padding"
	}
	rules[box.sides[3].index] = &css_ast.RDeclaration{
		Key:       key,
		KeyText:   keyText,
		Value:     tokens,
		KeyRange:  keyRange,
		Important: box.important,
	}
}
