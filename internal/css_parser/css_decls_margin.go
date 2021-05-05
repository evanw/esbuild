package css_parser

import (
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

const (
	marginTop = iota
	marginRight
	marginBottom
	marginLeft
)

type marginSide struct {
	token     css_ast.Token
	index     uint32
	important bool
	single    bool
}

type marginTracker struct {
	sides [4]marginSide
}

func (margin *marginTracker) updateSide(rules []css_ast.R, side int, new marginSide) {
	if old := margin.sides[side]; old.token.Kind != css_lexer.TEndOfFile && (!new.single || old.single) {
		rules[old.index] = nil
	}
	margin.sides[side] = new
}

func (margin *marginTracker) mangleSides(rules []css_ast.R, decl *css_ast.RDeclaration, index int) {
	if n := len(decl.Value); n >= 1 && n <= 4 {
		for side, t := range decl.Value {
			t.TurnLengthIntoNumberIfZero()
			margin.updateSide(rules, side, marginSide{token: t, index: uint32(index), important: decl.Important})
		}
		if n == 1 {
			margin.sides[1] = margin.sides[0]
		}
		if n <= 2 {
			margin.sides[2] = margin.sides[0]
		}
		if n <= 3 {
			margin.sides[3] = margin.sides[1]
		}
		margin.compactRules(rules, decl.KeyRange)
	} else {
		margin.sides = [4]marginSide{}
	}
}

func (margin *marginTracker) mangleSide(rules []css_ast.R, decl *css_ast.RDeclaration, index int, side int) {
	if tokens := decl.Value; len(tokens) == 1 {
		t := tokens[0]
		if t.TurnLengthIntoNumberIfZero() {
			tokens[0] = t
		}
		margin.updateSide(rules, side, marginSide{token: t, index: uint32(index), important: decl.Important, single: true})
		margin.compactRules(rules, decl.KeyRange)
	} else {
		margin.sides = [4]marginSide{}
	}
}

func (margin *marginTracker) compactRules(rules []css_ast.R, keyRange logger.Range) {
	// All tokens must be present
	if eof := css_lexer.TEndOfFile; margin.sides[0].token.Kind == eof || margin.sides[1].token.Kind == eof ||
		margin.sides[2].token.Kind == eof || margin.sides[3].token.Kind == eof {
		return
	}

	// All declarations must have the same "!important" state
	if i := margin.sides[0].important; i != margin.sides[1].important ||
		i != margin.sides[2].important || i != margin.sides[3].important {
		return
	}

	// Generate the most minimal representation
	tokens := []css_ast.Token{
		margin.sides[0].token,
		margin.sides[1].token,
		margin.sides[2].token,
		margin.sides[3].token,
	}
	if tokens[3].EqualsIgnoringWhitespace(tokens[1]) {
		if tokens[2].EqualsIgnoringWhitespace(tokens[0]) {
			if tokens[1].EqualsIgnoringWhitespace(tokens[0]) {
				tokens = tokens[:1]
			} else {
				tokens = tokens[:2]
			}
		} else {
			tokens = tokens[:3]
		}

		// Copy the whitespace after flag from the last token
		if last := &tokens[len(tokens)-1]; (last.Whitespace^margin.sides[3].token.Whitespace)&css_ast.WhitespaceAfter != 0 {
			last.Whitespace ^= css_ast.WhitespaceAfter
		}
	}

	// Remove all of the existing declarations
	rules[margin.sides[0].index] = nil
	rules[margin.sides[1].index] = nil
	rules[margin.sides[2].index] = nil
	rules[margin.sides[3].index] = nil

	// Insert the combined declaration where the last rule was
	rules[margin.sides[3].index] = &css_ast.RDeclaration{
		Key:       css_ast.DMargin,
		KeyText:   "margin",
		Value:     tokens,
		KeyRange:  keyRange,
		Important: margin.sides[0].important,
	}
}
