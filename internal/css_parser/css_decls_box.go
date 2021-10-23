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

func (box *boxTracker) updateSide(rules []css_ast.Rule, side int, new boxSide) {
	if old := box.sides[side]; old.token.Kind != css_lexer.TEndOfFile && (!new.single || old.single) {
		rules[old.index] = css_ast.Rule{}
	}
	box.sides[side] = new
}

func (box *boxTracker) mangleSides(rules []css_ast.Rule, decl *css_ast.RDeclaration, index int, removeWhitespace bool) {
	// Reset if we see a change in the "!important" flag
	if box.important != decl.Important {
		box.sides = [4]boxSide{}
		box.important = decl.Important
	}

	allowedIdent := ""
	isMargin := decl.Key == css_ast.DMargin
	if isMargin {
		allowedIdent = "auto"
	}
	if quad, ok := expandTokenQuad(decl.Value, allowedIdent); ok {
		for side, t := range quad {
			t.TurnLengthIntoNumberIfZero()
			box.updateSide(rules, side, boxSide{token: t, index: uint32(index)})
		}
		box.compactRules(rules, decl.KeyRange, removeWhitespace, isMargin)
	} else {
		box.sides = [4]boxSide{}
	}
}

func (box *boxTracker) mangleSide(rules []css_ast.Rule, decl *css_ast.RDeclaration, index int, removeWhitespace bool, side int) {
	// Reset if we see a change in the "!important" flag
	if box.important != decl.Important {
		box.sides = [4]boxSide{}
		box.important = decl.Important
	}

	isMargin := false
	switch decl.Key {
	case css_ast.DMarginTop, css_ast.DMarginRight, css_ast.DMarginBottom, css_ast.DMarginLeft:
		isMargin = true
	}

	if tokens := decl.Value; len(tokens) == 1 {
		if t := tokens[0]; t.Kind.IsNumeric() || (t.Kind == css_lexer.TIdent && isMargin && t.Text == "auto") {
			if t.TurnLengthIntoNumberIfZero() {
				tokens[0] = t
			}
			box.updateSide(rules, side, boxSide{token: t, index: uint32(index), single: true})
			box.compactRules(rules, decl.KeyRange, removeWhitespace, isMargin)
			return
		}
	}

	box.sides = [4]boxSide{}
}

func (box *boxTracker) compactRules(rules []css_ast.Rule, keyRange logger.Range, removeWhitespace bool, isMargin bool) {
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
	rules[box.sides[0].index] = css_ast.Rule{}
	rules[box.sides[1].index] = css_ast.Rule{}
	rules[box.sides[2].index] = css_ast.Rule{}
	rules[box.sides[3].index] = css_ast.Rule{}

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
	rules[box.sides[3].index].Data = &css_ast.RDeclaration{
		Key:       key,
		KeyText:   keyText,
		Value:     tokens,
		KeyRange:  keyRange,
		Important: box.important,
	}
}
